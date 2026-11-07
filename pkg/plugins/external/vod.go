package external

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

var errVODNoMatch = errors.New("no confident, unambiguous catalogue match for the scheduled programme")

type Programme struct {
	Title      string `json:"showname"`
	Category   string `json:"showCategory"`
	StartEpoch int64  `json:"startEpoch"`
	EndEpoch   int64  `json:"endEpoch"`
}

type ProgrammeResolver func(string, string) ([]Programme, error)

type vodSession struct {
	Origin      string `json:"origin"`
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	GuestToken  string `json:"guest_token"`
	ProfileID   string `json:"profile_id"`
	UserType    string `json:"user_type"`
	Country     string `json:"country"`
	AppVersion  string `json:"app_version"`
	ExpiresAt   int64  `json:"expires_at"`
}

type vodItem struct {
	Duration      int      `json:"duration"`
	AgeRating     string   `json:"ageRating"`
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle"`
	AssetType     int      `json:"assetType"`
	Languages     []string `json:"languages"`
	ReleaseDate   string   `json:"releaseDate"`
	EpisodeNumber int      `json:"episodeNumber"`
	Order         int
	TotalItems    int
	TVShow        *vodItem `json:"tvShow"`
}

type vodPlayback struct {
	Padding        bool
	Fallback       bool
	ID             string
	ChannelID      string
	Programme      Programme
	Content        vodItem
	ManifestURL    string
	LicenseURL     string
	LicenseHeaders http.Header
	IsDRM          bool
}

func (p *Plugin) ConfigureVOD(sessionURL string, schedule ProgrammeResolver, stateDir ...string) {
	if p.cfg.VOD == nil || p.cfg.VOD.ChannelPrefix == "" || p.cfg.VOD.Catalogue.Request.URL == "" || p.cfg.VOD.Playback.Request.URL == "" || schedule == nil {
		return
	}
	p.vodSessionURL = sessionURL
	if len(stateDir) > 0 {
		p.vodStateDir = stateDir[0]
	}
	p.vodDays = expirable.NewLRU[string, []vodScheduledItem](128, nil, 48*time.Hour)
	p.vodSchedule = schedule
	p.vodCatalogue = expirable.NewLRU[string, []vodItem](256, nil, 15*time.Minute)
	p.vodSelections = expirable.NewLRU[string, *vodPlayback](128, nil, 5*time.Minute)
	p.vodStreams = expirable.NewLRU[string, *vodPlayback](256, nil, 4*time.Hour)
}

func (p *Plugin) vodEnabled(id string) bool {
	if p.vodSchedule == nil || id == "" || strings.Trim(id, "0123456789") == "" || !strings.HasPrefix(id, p.cfg.VOD.ChannelPrefix) {
		return false
	}
	for _, channel := range p.cfg.Channels {
		if channel.ID == id {
			return true
		}
	}
	return false
}

func (p *Plugin) loadVODSession() (*vodSession, error) {
	if p.vodSessionURL == "" {
		return nil, fmt.Errorf("experimental playback requires plugin_vod_session_url")
	}
	endpoint, err := url.Parse(p.vodSessionURL)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("experimental playback requires a valid HTTP(S) session URL")
	}
	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cannot construct experimental session request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache, no-store")
	client := *p.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch experimental playback session")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("experimental session endpoint returned HTTP %d", response.StatusCode)
	}
	const maxSessionBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxSessionBytes+1))
	if err != nil || len(data) > maxSessionBytes {
		return nil, fmt.Errorf("cannot read experimental playback session JSON")
	}
	var session vodSession
	if json.Unmarshal(data, &session) != nil {
		return nil, fmt.Errorf("experimental playback session is incomplete")
	}
	if strings.TrimRight(session.Origin, "/") != strings.TrimRight(p.GetOrigin(), "/") {
		return nil, fmt.Errorf("experimental playback session belongs to a different plugin origin")
	}
	if session.ExpiresAt != 0 && time.Now().Unix() >= session.ExpiresAt {
		return nil, fmt.Errorf("experimental playback session expired; export a fresh signed-in session")
	}
	variables := p.vodVariables(&session)
	for _, name := range p.cfg.VOD.RequiredSession {
		if value := vodPath(variables, "session."+name); value == nil || value == "" {
			return nil, fmt.Errorf("experimental playback session is incomplete")
		}
	}
	return &session, nil
}

func vodSameProvider(origin, target string) bool {
	a, e1 := url.Parse(origin)
	b, e2 := url.Parse(target)
	if e1 != nil || e2 != nil || a.Scheme != "https" || b.Scheme != "https" {
		return false
	}
	base := strings.TrimPrefix(strings.ToLower(a.Hostname()), "www.")
	host := strings.ToLower(b.Hostname())
	return base != "" && (host == base || strings.HasSuffix(host, "."+base))
}

func (p *Plugin) vodJSON(session *vodSession, spec VODRequest, variables map[string]any) (any, error) {
	req, err := p.vodBuildRequest(session, spec, variables)
	if err != nil {
		return nil, err
	}
	client := *p.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider API request failed")
	}
	defer resp.Body.Close()
	var data any
	if json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&data) != nil {
		return nil, fmt.Errorf("provider API returned a non-JSON response (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK || vodPath(data, spec.ErrorPath) != nil {
		return nil, fmt.Errorf("provider request rejected (HTTP %d)", resp.StatusCode)
	}
	return data, nil
}

func (p *Plugin) vodSearch(session *vodSession, title, language string) ([]vodItem, error) {
	key := session.Country + "|" + language + "|" + normaliseVODTitle(title)
	if cached, ok := p.vodCatalogue.Get(key); ok {
		return cached, nil
	}
	variables := p.vodVariables(session)
	variables["query"] = title
	variables["language"] = language
	response, err := p.vodJSON(session, p.cfg.VOD.Catalogue.Request, variables)
	if err != nil {
		return nil, err
	}
	items := vodReadItems(response, p.cfg.VOD.Catalogue)
	if len(items) == 0 {
		return nil, fmt.Errorf("no catalogue results for the scheduled programme")
	}
	p.vodCatalogue.Add(key, items)
	return items, nil
}

func vodReadItems(response any, spec VODCatalogue) []vodItem {
	fields := spec.Fields
	var items []vodItem
	for _, object := range vodPathValues(response, spec.ItemsPath) {
		item := vodItem{Duration: vodInt(vodPath(object, fields["duration"])), AgeRating: vodString(vodPath(object, fields["age_rating"])), ID: vodString(vodPath(object, fields["id"])), Title: vodString(vodPath(object, fields["title"])), OriginalTitle: vodString(vodPath(object, fields["original_title"])), AssetType: vodInt(vodPath(object, fields["asset_type"])), ReleaseDate: vodString(vodPath(object, fields["release_date"])), EpisodeNumber: vodInt(vodPath(object, fields["episode_number"])), Order: vodInt(vodPath(object, fields["order"])), TotalItems: vodInt(vodPath(object, fields["total_items"]))}
		if languages, ok := vodPath(object, fields["languages"]).([]any); ok {
			for _, language := range languages {
				item.Languages = append(item.Languages, vodString(language))
			}
		}
		if id := vodString(vodPath(object, fields["parent_id"])); id != "" {
			item.TVShow = &vodItem{ID: id, Title: vodString(vodPath(object, fields["parent_title"]))}
		}
		if item.ID != "" {
			items = append(items, item)
		}
	}
	return items
}

func normaliseVODTitle(title string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ")
}

func vodSimilarity(a, b string) float64 {
	a, b = normaliseVODTitle(a), normaliseVODTitle(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	x, y := []rune(a), []rune(b)
	previous := make([]int, len(y)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, left := range x {
		row := make([]int, len(y)+1)
		row[0] = i + 1
		for j, right := range y {
			cost := 0
			if left != right {
				cost = 1
			}
			row[j+1] = min(row[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = row
	}
	return 1 - float64(previous[len(y)])/float64(max(len(x), len(y)))
}

func (p *Plugin) matchVODItems(programme Programme, items []vodItem, language string) ([]vodItem, float64, error) {
	movieType, showType, episodeType := p.cfg.VOD.AssetTypes["movie"], p.cfg.VOD.AssetTypes["show"], p.cfg.VOD.AssetTypes["episode"]
	bestScore, runnerUp := 0.0, 0.0
	var best vodItem
	seen := make(map[string]bool)
	movie := strings.Contains(strings.ToLower(programme.Category), "movie") || strings.Contains(strings.ToLower(programme.Category), "film")
	for _, item := range items {
		if seen[item.ID] || (item.AssetType != movieType && item.AssetType != showType) {
			continue
		}
		seen[item.ID] = true
		if movie && item.AssetType != movieType {
			continue
		}
		if language != "" && len(item.Languages) != 0 {
			found := false
			for _, value := range item.Languages {
				if value == language {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		score := max(vodSimilarity(programme.Title, item.Title), vodSimilarity(programme.Title, item.OriginalTitle))
		if score > bestScore {
			runnerUp = bestScore
			bestScore = score
			best = item
		} else if score > runnerUp {
			runnerUp = score
		}
	}
	if bestScore < .8 || bestScore-runnerUp < .05 {
		return nil, bestScore, errVODNoMatch
	}
	if best.AssetType == movieType {
		return []vodItem{best}, bestScore, nil
	}
	var episodes []vodItem
	seen = make(map[string]bool)
	for _, item := range items {
		if item.AssetType == episodeType && item.TVShow != nil && item.TVShow.ID == best.ID && !seen[item.ID] && (item.ReleaseDate == "" || item.ReleaseDate[:min(10, len(item.ReleaseDate))] <= time.Now().Format("2006-01-02")) {
			seen[item.ID] = true
			episodes = append(episodes, item)
		}
	}
	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].ReleaseDate == episodes[j].ReleaseDate {
			return episodes[i].EpisodeNumber > episodes[j].EpisodeNumber
		}
		return episodes[i].ReleaseDate > episodes[j].ReleaseDate
	})
	if len(episodes) == 0 {
		return []vodItem{best}, bestScore, nil
	}
	return episodes[:min(5, len(episodes))], bestScore, nil
}

func (p *Plugin) vodResolve(id string, after int64) (*vodPlayback, error) {
	session, err := p.loadVODSession()
	if err != nil {
		return nil, err
	}
	if p.HasVODSchedule(id) {
		return p.vodResolveScheduled(session, id, time.Now())
	}
	name, language, epgID := "", "", id
	for _, channel := range p.cfg.Channels {
		if channel.ID == id {
			name = channel.Name
			language = p.cfg.VOD.LanguageCodes[strconv.Itoa(channel.Language)]
			if channel.EPGID != "" {
				epgID = channel.EPGID
			}
			break
		}
	}
	guide, err := p.vodSchedule(epgID, name)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(guide, func(i, j int) bool { return guide[i].StartEpoch < guide[j].StartEpoch })
	threshold := max(time.Now().UnixMilli(), after)
	var programme Programme
	for _, entry := range guide {
		if entry.EndEpoch > threshold && (after != 0 || entry.StartEpoch <= threshold) {
			programme = entry
			break
		}
	}
	if programme.Title == "" {
		return nil, fmt.Errorf("no current or next programme is available in the guide")
	}
	fingerprint := sha256.Sum256([]byte(session.Token))
	key := fmt.Sprintf("%s|%d|%x", id, programme.StartEpoch, fingerprint[:8])
	if cached, ok := p.vodSelections.Get(key); ok {
		return cached, nil
	}
	result, err, _ := p.vodGroup.Do(key, func() (any, error) {
		if cached, ok := p.vodSelections.Get(key); ok {
			return cached, nil
		}
		candidates, fallback, err := p.vodCandidates(session, id, programme, language)
		if fallback {
			language = p.cfg.VOD.Fallbacks[id].Language
		}
		if err != nil {
			return nil, err
		}
		var lastError error
		for _, item := range candidates {
			playback, err := p.vodContentPlayback(session, item, language)
			if err != nil {
				lastError = err
				continue
			}
			playback.ID = uuid.NewString()
			playback.ChannelID = id
			playback.Programme = programme
			playback.Fallback = fallback
			p.vodStreams.Add(playback.ID, playback)
			p.vodSelections.Add(key, playback)
			return playback, nil
		}
		return nil, lastError
	})
	if err != nil {
		return nil, err
	}
	return result.(*vodPlayback), nil
}

func (p *Plugin) vodContentPlayback(session *vodSession, item vodItem, language string) (*vodPlayback, error) {
	variables := p.vodVariables(session)
	variables["content"] = map[string]any{"id": item.ID, "title": item.Title}
	variables["language"] = language
	response, err := p.vodJSON(session, p.cfg.VOD.Playback.Request, variables)
	if err != nil {
		return nil, err
	}
	fields := p.cfg.VOD.Playback.Fields
	if duration := vodInt(vodPath(response, fields["duration"])); duration > 0 {
		item.Duration = duration
	}
	if vodBool(vodPath(response, fields["trailer"])) {
		return nil, fmt.Errorf("provider returned a trailer instead of full playback")
	}
	drm := vodBool(vodPath(response, fields["drm"]))
	manifest := vodString(vodPath(response, fields["hls_manifest"]))
	if drm {
		manifest = vodString(vodPath(response, fields["dash_manifest"]))
	}
	if manifest == "" {
		manifest = vodString(vodPath(response, fields["manifest"]))
	}
	media, err := url.Parse(manifest)
	if err != nil || media.Scheme != "https" || media.Host == "" {
		return nil, fmt.Errorf("provider did not return a playable HTTPS manifest")
	}
	playback := &vodPlayback{Content: item, ManifestURL: manifest, IsDRM: drm}
	if drm {
		variables["response"] = response
		request, err := p.vodBuildRequest(session, p.cfg.VOD.License, variables)
		if err != nil {
			return nil, fmt.Errorf("cannot construct provider license authorization")
		}
		playback.LicenseURL = request.URL.String()
		playback.LicenseHeaders = request.Header.Clone()
	}
	return playback, nil
}

func vodAfter(value string) int64 { after, _ := strconv.ParseInt(value, 10, 64); return after }

func (p *Plugin) vodCandidates(session *vodSession, id string, programme Programme, language string, seeds ...uint64) ([]vodItem, bool, error) {
	items, err := p.vodSearch(session, programme.Title, language)
	if err != nil {
		return nil, false, err
	}
	candidates, _, err := p.matchVODItems(programme, items, language)
	fallback := false
	if errors.Is(err, errVODNoMatch) {
		if rule, ok := p.cfg.VOD.Fallbacks[id]; ok {
			candidates, err = p.vodFallbackItems(session, rule, seeds...)
			language = rule.Language
			fallback = true
		}
	}
	if err != nil {
		return nil, false, err
	}
	matched := candidates[0]
	if matched.AssetType == p.cfg.VOD.AssetTypes["episode"] && matched.TVShow != nil {
		matched = *matched.TVShow
	}
	if p.cfg.VOD.Series != nil && matched.ID != "" && (candidates[0].AssetType == p.cfg.VOD.AssetTypes["show"] || candidates[0].AssetType == p.cfg.VOD.AssetTypes["episode"]) {
		candidates, err = p.vodSeriesEpisodes(session, matched, language, fallback, seeds...)
		if err != nil {
			return nil, false, err
		}
	} else if candidates[0].AssetType == p.cfg.VOD.AssetTypes["show"] {
		matched := candidates[0]
		items, err = p.vodSearch(session, matched.Title, language)
		if err != nil {
			return nil, false, err
		}
		exact := programme
		exact.Title = matched.Title
		candidates, _, err = p.matchVODItems(exact, items, language)
		if err != nil {
			return nil, false, err
		}
		if candidates[0].AssetType == p.cfg.VOD.AssetTypes["show"] {
			return nil, false, fmt.Errorf("matched show has no released episodes in the catalogue results")
		}
	}

	return candidates, fallback, nil
}
