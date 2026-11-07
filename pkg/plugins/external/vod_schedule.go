package external

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
)

type vodScheduledItem struct {
	Programme Programme `json:"programme"`
	Content   vodItem   `json:"content"`
	Fallback  bool      `json:"fallback"`
	Padding   bool      `json:"padding,omitempty"`
}

func (p *Plugin) HasVODSchedule(id string) bool {
	if !p.vodEnabled(id) {
		return false
	}
	spec, ok := p.cfg.VOD.Schedules[id]
	return ok && (spec.Mode == "guide" || spec.Mode == "movies")
}

func (p *Plugin) vodChannel(id string) (string, string, string) {
	for _, ch := range p.cfg.Channels {
		if ch.ID == id {
			epg := ch.EPGID
			if epg == "" {
				epg = id
			}
			return ch.Name, p.cfg.VOD.LanguageCodes[strconv.Itoa(ch.Language)], epg
		}
	}
	return "", "", id
}

func (p *Plugin) vodDay(id string, now time.Time) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(p.cfg.VOD.Schedules[id].Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid scheduled channel timezone")
	}
	local := now.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1), nil
}

func (p *Plugin) vodPlanKey(id string, day time.Time) string {
	data, _ := json.Marshal(p.cfg.VOD)
	sum := sha256.Sum256(append([]byte(p.Name+"|"+p.GetOrigin()+"|"+id+"|"+day.Format("2006-01-02")+"|"), data...))
	return fmt.Sprintf("%x", sum[:])
}

func vodScheduleSeed(key string) uint64 {
	sum := sha256.Sum256([]byte(key))
	return binary.LittleEndian.Uint64(sum[:8])
}

func (p *Plugin) vodStoredPlan(key string) ([]vodScheduledItem, bool) {
	if entries, ok := p.vodDays.Get(key); ok {
		return append([]vodScheduledItem(nil), entries...), true
	}
	if p.vodStateDir == "" {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(p.vodStateDir, key+".json"))
	if err != nil || len(data) > 2<<20 {
		return nil, false
	}
	var entries []vodScheduledItem
	if json.Unmarshal(data, &entries) != nil || len(entries) == 0 {
		return nil, false
	}
	for _, entry := range entries {
		if entry.Content.ID == "" || entry.Content.Duration <= 0 || entry.Programme.EndEpoch <= entry.Programme.StartEpoch {
			return nil, false
		}
	}
	p.vodDays.Add(key, entries)
	return append([]vodScheduledItem(nil), entries...), true
}

func (p *Plugin) vodStorePlan(key string, entries []vodScheduledItem) error {
	if p.vodStateDir != "" {
		if err := os.MkdirAll(p.vodStateDir, 0700); err != nil {
			return fmt.Errorf("cannot create scheduled channel state directory")
		}
		data, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(p.vodStateDir, ".schedule-")
		if err != nil {
			return fmt.Errorf("cannot save scheduled channel state")
		}
		name := file.Name()
		defer os.Remove(name)
		if _, err = file.Write(data); err != nil {
			file.Close()
			return fmt.Errorf("cannot write scheduled channel state")
		}
		if err = file.Close(); err != nil {
			return err
		}
		if err = os.Rename(name, filepath.Join(p.vodStateDir, key+".json")); err != nil {
			return fmt.Errorf("cannot replace scheduled channel state")
		}
	}
	p.vodDays.Add(key, append([]vodScheduledItem(nil), entries...))
	return nil
}

func (p *Plugin) vodSourceGuide(id string) ([]Programme, error) {
	name, _, epg := p.vodChannel(id)
	guide, err := p.vodSchedule(epg, name)
	if err != nil {
		return nil, err
	}
	guide = append([]Programme(nil), guide...)
	sort.SliceStable(guide, func(i, j int) bool { return guide[i].StartEpoch < guide[j].StartEpoch })
	return guide, nil
}

func (p *Plugin) vodStreamKey(session *vodSession, id string, start int64) string {
	fingerprint := sha256.Sum256([]byte(session.Token))
	return fmt.Sprintf("%s|%d|%x", id, start, fingerprint[:8])
}

func (p *Plugin) vodCacheScheduledStream(session *vodSession, id string, entry vodScheduledItem, stream *vodPlayback) {
	stream.ID = uuid.NewString()
	stream.ChannelID = id
	stream.Programme = entry.Programme
	stream.Fallback = entry.Fallback
	stream.Content = entry.Content
	stream.Padding = entry.Padding
	p.vodStreams.Add(stream.ID, stream)
	p.vodSelections.Add(p.vodStreamKey(session, id, entry.Programme.StartEpoch), stream)
}

func (p *Plugin) vodGuideSlot(session *vodSession, id string, programme Programme) (vodScheduledItem, error) {
	day, _, err := p.vodDay(id, time.UnixMilli(programme.StartEpoch))
	if err != nil {
		return vodScheduledItem{}, err
	}
	key := p.vodPlanKey(id, day) + "-" + strconv.FormatInt(programme.StartEpoch, 10)
	if entries, ok := p.vodStoredPlan(key); ok {
		return entries[0], nil
	}
	result, err, _ := p.vodGroup.Do("slot:"+key, func() (any, error) {
		if entries, ok := p.vodStoredPlan(key); ok {
			return entries[0], nil
		}
		_, language, _ := p.vodChannel(id)
		titleKey := p.vodPlanKey(id, day) + "-title-" + fmt.Sprintf("%x", sha256.Sum256([]byte(programme.Title+programme.Category)))
		var candidates []vodItem
		fallback := false
		if saved, ok := p.vodStoredPlan(titleKey); ok {
			candidates = []vodItem{saved[0].Content}
		} else {
			candidates, fallback, err = p.vodCandidates(session, id, programme, language, vodScheduleSeed(key))
			if err != nil {
				return nil, err
			}
		}
		var last error
		for _, item := range candidates {
			stream, streamErr := p.vodContentPlayback(session, item, language)
			if streamErr != nil {
				last = streamErr
				continue
			}
			item = stream.Content
			if item.Duration <= 0 {
				last = fmt.Errorf("provider did not supply a usable content duration")
				continue
			}
			display := programme
			if item.TVShow != nil && item.TVShow.Title != "" {
				display.Title = item.TVShow.Title
			} else {
				display.Title = item.Title
			}
			entry := vodScheduledItem{Programme: display, Content: item, Fallback: fallback}
			if err := p.vodStorePlan(key, []vodScheduledItem{entry}); err != nil {
				return nil, err
			}
			if !fallback {
				if err := p.vodStorePlan(titleKey, []vodScheduledItem{entry}); err != nil {
					return nil, err
				}
			}
			p.vodCacheScheduledStream(session, id, entry, stream)
			return entry, nil
		}
		if last == nil {
			last = fmt.Errorf("no playable content is available for this scheduled slot")
		}
		return nil, last
	})
	if err != nil {
		return vodScheduledItem{}, err
	}
	return result.(vodScheduledItem), nil
}

func (p *Plugin) vodMovieDay(session *vodSession, id string, now time.Time) ([]vodScheduledItem, error) {
	start, end, err := p.vodDay(id, now)
	if err != nil {
		return nil, err
	}
	key := p.vodPlanKey(id, start)
	if entries, ok := p.vodStoredPlan(key); ok {
		return entries, nil
	}
	result, err, _ := p.vodGroup.Do("movies:"+key, func() (any, error) {
		if entries, ok := p.vodStoredPlan(key); ok {
			return entries, nil
		}
		rule, ok := p.cfg.VOD.Fallbacks[id]
		if !ok {
			return nil, fmt.Errorf("scheduled movie channel requires a movie pool")
		}
		seed := vodScheduleSeed(key)
		items, err := p.vodFallbackItems(session, rule, seed)
		if err != nil {
			return nil, err
		}
		var movies []vodItem
		for _, item := range items {
			if item.AssetType != p.cfg.VOD.AssetTypes["movie"] {
				continue
			}
			stream, err := p.vodContentPlayback(session, item, rule.Language)
			if err == nil && stream.Content.Duration > 0 {
				movies = append(movies, stream.Content)
			}
		}
		if len(movies) == 0 {
			return nil, fmt.Errorf("no playable movies with valid durations in the configured pool")
		}
		rng := vodRandom(seed)
		cursor := start.UnixMilli()
		limit := end.UnixMilli()
		var entries []vodScheduledItem
		for len(entries) < 200 {
			var choices []vodItem
			for _, movie := range movies {
				if cursor+int64(movie.Duration)*1000 <= limit {
					choices = append(choices, movie)
				}
			}
			if len(choices) == 0 {
				break
			}
			if len(choices) > 1 && len(entries) > 0 {
				previous := entries[len(entries)-1].Content.ID
				filtered := choices[:0]
				for _, movie := range choices {
					if movie.ID != previous {
						filtered = append(filtered, movie)
					}
				}
				if len(filtered) > 0 {
					choices = filtered
				}
			}
			movie := choices[rng.IntN(len(choices))]
			next := cursor + int64(movie.Duration)*1000
			entries = append(entries, vodScheduledItem{Programme: Programme{Title: movie.Title, Category: "Movie", StartEpoch: cursor, EndEpoch: next}, Content: movie, Fallback: true})
			cursor = next
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("movie durations do not fit the scheduled day")
		}
		padding := limit - cursor
		if padding > int64(6*time.Hour/time.Millisecond) {
			return nil, fmt.Errorf("movie pool cannot fill the daytime schedule")
		}
		for i := range entries {
			entries[i].Programme.StartEpoch += padding
			entries[i].Programme.EndEpoch += padding
		}
		if padding > 0 {
			movie := entries[len(entries)-1].Content
			fill := vodScheduledItem{Programme: Programme{Title: movie.Title + " (repeat)", Category: "Movie", StartEpoch: start.UnixMilli(), EndEpoch: start.UnixMilli() + padding}, Content: movie, Fallback: true, Padding: true}
			entries = append([]vodScheduledItem{fill}, entries...)
		}
		if err := p.vodStorePlan(key, entries); err != nil {
			return nil, err
		}
		return entries, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]vodScheduledItem), nil
}

func (p *Plugin) vodCurrentScheduled(session *vodSession, id string, now time.Time) (vodScheduledItem, error) {
	if p.cfg.VOD.Schedules[id].Mode == "movies" {
		entries, err := p.vodMovieDay(session, id, now)
		if err != nil {
			return vodScheduledItem{}, err
		}
		for _, entry := range entries {
			if entry.Programme.StartEpoch <= now.UnixMilli() && entry.Programme.EndEpoch > now.UnixMilli() {
				return entry, nil
			}
		}
	} else {
		guide, err := p.vodSourceGuide(id)
		if err != nil {
			return vodScheduledItem{}, err
		}
		for _, programme := range guide {
			if programme.StartEpoch <= now.UnixMilli() && programme.EndEpoch > now.UnixMilli() {
				return p.vodGuideSlot(session, id, programme)
			}
		}
	}
	return vodScheduledItem{}, fmt.Errorf("no current programme in the generated schedule")
}

func (p *Plugin) vodResolveScheduled(session *vodSession, id string, now time.Time) (*vodPlayback, error) {
	entry, err := p.vodCurrentScheduled(session, id, now)
	if err != nil {
		return nil, err
	}
	key := p.vodStreamKey(session, id, entry.Programme.StartEpoch)
	if cached, ok := p.vodSelections.Get(key); ok {
		return cached, nil
	}
	result, err, _ := p.vodGroup.Do("scheduled-play:"+key, func() (any, error) {
		if cached, ok := p.vodSelections.Get(key); ok {
			return cached, nil
		}
		_, language, _ := p.vodChannel(id)
		stream, err := p.vodContentPlayback(session, entry.Content, language)
		if err != nil {
			return nil, err
		}
		p.vodCacheScheduledStream(session, id, entry, stream)
		return stream, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*vodPlayback), nil
}

func (p *Plugin) VODGuide(id string) ([]Programme, error) {
	session, err := p.loadVODSession()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if p.cfg.VOD.Schedules[id].Mode == "movies" {
		entries, err := p.vodMovieDay(session, id, now)
		if err != nil {
			return nil, err
		}
		guide := make([]Programme, len(entries))
		for i, entry := range entries {
			guide[i] = entry.Programme
		}
		return guide, nil
	}
	guide, err := p.vodSourceGuide(id)
	if err != nil {
		return nil, err
	}
	current, err := p.vodCurrentScheduled(session, id, now)
	if err != nil {
		return nil, err
	}
	for i, programme := range guide {
		if programme.StartEpoch == current.Programme.StartEpoch {
			guide[i] = current.Programme
			continue
		}
		day, _, err := p.vodDay(id, time.UnixMilli(programme.StartEpoch))
		if err != nil {
			return nil, err
		}
		key := p.vodPlanKey(id, day) + "-" + strconv.FormatInt(programme.StartEpoch, 10)
		if saved, ok := p.vodStoredPlan(key); ok {
			guide[i] = saved[0].Programme
		}
	}
	day, _, _ := p.vodDay(id, now)
	warmKey := "warm:" + p.vodPlanKey(id, day)
	if _, loaded := p.vodDays.Get(warmKey); !loaded {
		p.vodDays.Add(warmKey, nil)
		go func() {
			for _, programme := range guide {
				if programme.StartEpoch > now.UnixMilli() {
					_, _ = p.vodGuideSlot(session, id, programme)
				}
			}
		}()
	}
	return guide, nil
}
