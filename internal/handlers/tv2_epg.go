package handlers

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	"github.com/jiotv-go/jiotv_go/v3/internal/plugins"
)

const (
	defaultTV2EPGURL           = "https://avkb.short.gy/epg.xml.gz"
	defaultTV2EPGAlternateURL  = "https://mitthu786.github.io/tvepg/tataplay/epg.xml.gz"
	tv2EPGCacheTTL             = 30 * time.Minute
	tv2EPGRequestTimeout       = 30 * time.Second
	tv2EPGProgrammesPerChannel = 24
)

type tv2EPGProgramme struct {
	StartEpoch int64  `json:"startEpoch"`
	EndEpoch   int64  `json:"endEpoch"`
	ShowName   string `json:"showname"`
}

type tv2XMLTVProgramme struct {
	Channel string `xml:"channel,attr"`
	Start   string `xml:"start,attr"`
	Stop    string `xml:"stop,attr"`
	Title   string `xml:"title"`
}

type tv2XMLTVChannel struct {
	ID          string `xml:"id,attr"`
	DisplayName string `xml:"display-name"`
}

type tv2ExternalEPGIndex struct {
	programmes              map[string][]tv2EPGProgramme
	channelIDsByName        map[string][]string
	channelIDsByDisplayName map[string][]string
	channelNamesByID        map[string]string
}

type tv2EPGCacheEntry struct {
	index     tv2ExternalEPGIndex
	expiresAt time.Time
}

var tv2EPGCache = struct {
	sync.Mutex
	entries map[string]tv2EPGCacheEntry
}{
	entries: make(map[string]tv2EPGCacheEntry),
}

var tv2EPGPrewarmOnce sync.Once

func prewarmTV2PluginEPG(channelID string) {
	tv2EPGPrewarmOnce.Do(func() {
		go func(sourceURL string) {
			_, _ = getTV2ExternalEPG(sourceURL)
		}(tv2JioEPGMappingSource())
	})
}

// TV2EPGHandler returns XMLTV programme data for channels that are absent from
// Jio's per-channel EPG API, including external plugin channel IDs.
func TV2EPGHandler(c *fiber.Ctx) error {
	_, isPluginChannel := plugins.GetChannelPluginID(c.Params("channelID"))
	if isPluginChannel {
		if index, err := getTV2ExternalEPG(tv2JioEPGMappingSource()); err == nil {
			if jioChannelID := findTV2JioEPGChannelIDInXMLTV(index, c.Query("name")); jioChannelID != "" {
				return c.Redirect("/epg/"+jioChannelID+"/0", fiber.StatusTemporaryRedirect)
			}
		}
	}

	var sourceErr error
	anySourceAvailable := false
	for _, sourceURL := range tv2EPGSources(c.Params("channelID")) {
		index, err := getTV2ExternalEPG(sourceURL)
		if err != nil {
			sourceErr = err
			continue
		}
		anySourceAvailable = true
		if channelProgrammes := findTV2ProgrammeGuide(index, c.Params("channelID"), c.Query("name"), isPluginChannel, time.Now().UnixMilli()); len(channelProgrammes) != 0 {
			return c.JSON(fiber.Map{"epg": channelProgrammes})
		}
	}

	if !anySourceAvailable && sourceErr != nil {
		return fiber.NewError(fiber.StatusBadGateway, "TV2 EPG unavailable: "+sourceErr.Error())
	}
	return fiber.NewError(fiber.StatusNotFound, "No current programme guide for this channel")
}

func tv2JioEPGMappingSource() string {
	if config.Cfg.TV2EPGURL != "" {
		return config.Cfg.TV2EPGURL
	}
	return defaultTV2EPGURL
}

func tv2EPGSources(channelID string) []string {
	primaryURL := config.Cfg.TV2EPGURL
	if primaryURL == "" {
		primaryURL = defaultTV2EPGURL
	}

	alternateURL := config.Cfg.TV2EPGAlternateURL
	if alternateURL == "" && config.Cfg.TV2EPGURL == "" {
		alternateURL = defaultTV2EPGAlternateURL
	}
	_, isPluginChannel := plugins.GetChannelPluginID(channelID)
	return prioritiseTV2EPGSources(primaryURL, alternateURL, isPluginChannel)
}

func prioritiseTV2EPGSources(primaryURL, alternateURL string, isPluginChannel bool) []string {
	if alternateURL == "" || alternateURL == primaryURL {
		return []string{primaryURL}
	}
	if isPluginChannel {
		return []string{alternateURL, primaryURL}
	}
	return []string{primaryURL, alternateURL}
}

func getTV2ExternalEPG(sourceURL string) (tv2ExternalEPGIndex, error) {
	tv2EPGCache.Lock()
	defer tv2EPGCache.Unlock()

	if cached, ok := tv2EPGCache.entries[sourceURL]; ok && time.Now().Before(cached.expiresAt) {
		return cached.index, nil
	}

	client := &http.Client{Timeout: tv2EPGRequestTimeout}
	response, err := client.Get(sourceURL)
	if err != nil {
		return cachedTV2EPGOrError(sourceURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return cachedTV2EPGOrError(sourceURL, fmt.Errorf("source returned HTTP %d", response.StatusCode))
	}

	gzipReader, err := gzip.NewReader(response.Body)
	if err != nil {
		return cachedTV2EPGOrError(sourceURL, fmt.Errorf("opening XMLTV gzip: %w", err))
	}
	defer gzipReader.Close()

	index, err := parseTV2XMLTV(gzipReader, time.Now().UnixMilli())
	if err != nil {
		return cachedTV2EPGOrError(sourceURL, fmt.Errorf("parsing XMLTV: %w", err))
	}

	tv2EPGCache.entries[sourceURL] = tv2EPGCacheEntry{index: index, expiresAt: time.Now().Add(tv2EPGCacheTTL)}
	return index, nil
}

func cachedTV2EPGOrError(sourceURL string, err error) (tv2ExternalEPGIndex, error) {
	if cached, ok := tv2EPGCache.entries[sourceURL]; ok && len(cached.index.programmes) != 0 {
		return cached.index, nil
	}
	return tv2ExternalEPGIndex{}, err
}

func findTV2ProgrammeGuide(index tv2ExternalEPGIndex, channelID, channelName string, allowNameMatch bool, nowEpoch int64) []tv2EPGProgramme {
	candidateIDs := []string{channelID}
	if allowNameMatch {
		for _, sourceID := range tv2EPGChannelCandidates(index, channelName) {
			if sourceID != channelID {
				candidateIDs = append(candidateIDs, sourceID)
			}
		}
	}
	for _, sourceID := range candidateIDs {
		programmes := index.programmes[sourceID]
		for _, programme := range programmes {
			if programme.EndEpoch > nowEpoch {
				return programmes
			}
		}
	}
	return nil
}

func findTV2JioEPGChannelIDInXMLTV(index tv2ExternalEPGIndex, channelName string) string {
	for _, channelID := range tv2EPGChannelCandidates(index, channelName) {
		if _, err := strconv.Atoi(channelID); err == nil {
			return channelID
		}
	}
	return ""
}

func tv2EPGChannelCandidates(index tv2ExternalEPGIndex, channelName string) []string {
	seen := make(map[string]struct{})
	candidates := make([]string, 0)
	appendCandidates := func(ids []string) {
		for _, channelID := range ids {
			if _, exists := seen[channelID]; exists {
				continue
			}
			seen[channelID] = struct{}{}
			candidates = append(candidates, channelID)
		}
	}

	// Preserve HD/SD/etc. first. Only then broaden the match by removing
	// quality labels from both names.
	appendCandidates(index.channelIDsByDisplayName[normaliseTV2EPGDisplayName(channelName)])
	appendCandidates(index.channelIDsByName[normaliseTV2EPGName(channelName)])
	return candidates
}

func parseTV2XMLTV(reader io.Reader, minimumEndEpoch int64) (tv2ExternalEPGIndex, error) {
	decoder := xml.NewDecoder(reader)
	index := tv2ExternalEPGIndex{
		programmes:              make(map[string][]tv2EPGProgramme),
		channelIDsByName:        make(map[string][]string),
		channelIDsByDisplayName: make(map[string][]string),
		channelNamesByID:        make(map[string]string),
	}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return tv2ExternalEPGIndex{}, err
		}

		startElement, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if startElement.Name.Local == "channel" {
			var channel tv2XMLTVChannel
			if err := decoder.DecodeElement(&channel, &startElement); err != nil {
				return tv2ExternalEPGIndex{}, err
			}
			if name := normaliseTV2EPGName(channel.DisplayName); channel.ID != "" && name != "" {
				index.channelIDsByName[name] = append(index.channelIDsByName[name], channel.ID)
				index.channelIDsByDisplayName[normaliseTV2EPGDisplayName(channel.DisplayName)] = append(index.channelIDsByDisplayName[normaliseTV2EPGDisplayName(channel.DisplayName)], channel.ID)
				index.channelNamesByID[channel.ID] = channel.DisplayName
			}
			continue
		}
		if startElement.Name.Local != "programme" {
			continue
		}

		var programme tv2XMLTVProgramme
		if err := decoder.DecodeElement(&programme, &startElement); err != nil {
			return tv2ExternalEPGIndex{}, err
		}
		start, err := time.Parse("20060102150405 -0700", strings.TrimSpace(programme.Start))
		if err != nil {
			continue
		}
		stop, err := time.Parse("20060102150405 -0700", strings.TrimSpace(programme.Stop))
		if err != nil || programme.Channel == "" || strings.TrimSpace(programme.Title) == "" || stop.UnixMilli() <= minimumEndEpoch {
			continue
		}

		addTV2EPGProgramme(index.programmes, programme.Channel, tv2EPGProgramme{
			StartEpoch: start.UnixMilli(),
			EndEpoch:   stop.UnixMilli(),
			ShowName:   strings.TrimSpace(programme.Title),
		})
	}

	for channelID := range index.programmes {
		sort.Slice(index.programmes[channelID], func(i, j int) bool {
			return index.programmes[channelID][i].StartEpoch < index.programmes[channelID][j].StartEpoch
		})
	}

	return index, nil
}

func addTV2EPGProgramme(programmesByChannel map[string][]tv2EPGProgramme, channelID string, programme tv2EPGProgramme) {
	programmes := programmesByChannel[channelID]
	if len(programmes) < tv2EPGProgrammesPerChannel {
		programmesByChannel[channelID] = append(programmes, programme)
		return
	}

	latestIndex := 0
	for i := 1; i < len(programmes); i++ {
		if programmes[i].StartEpoch > programmes[latestIndex].StartEpoch {
			latestIndex = i
		}
	}
	if programme.StartEpoch < programmes[latestIndex].StartEpoch {
		programmes[latestIndex] = programme
	}
}

func normaliseTV2EPGName(name string) string {
	words := normaliseTV2EPGWords(name)
	filtered := words[:0]
	for _, word := range words {
		switch word {
		case "hd", "sd", "uhd", "fhd", "4k":
			continue
		}
		filtered = append(filtered, word)
	}
	return strings.Join(filtered, "")
}

func normaliseTV2EPGDisplayName(name string) string {
	return strings.Join(normaliseTV2EPGWords(name), "")
}

func normaliseTV2EPGWords(name string) []string {
	name = strings.ReplaceAll(strings.ToLower(name), "&", " and ")
	return strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func tv2EPGNameMatchScore(requestName, candidateName string) int {
	if candidateName == "" {
		return 0
	}
	if normaliseTV2EPGDisplayName(requestName) == normaliseTV2EPGDisplayName(candidateName) {
		return 2
	}
	if normaliseTV2EPGName(requestName) == normaliseTV2EPGName(candidateName) {
		return 1
	}
	return 0
}
