package handlers

import (
	"encoding/json"
	"html/template"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	"github.com/jiotv-go/jiotv_go/v3/internal/plugins"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"

	"github.com/gofiber/fiber/v2"
)

const tv2ChannelsCacheTTL = time.Minute

// tv2Channel is deliberately smaller than television.Channel: the TV2 client
// only needs these fields to render the guide and start playback.
type tv2Channel struct {
	ID           string `json:"channel_id"`
	Name         string `json:"channel_name"`
	PlayerURL    string `json:"player_url"`
	EPGChannelID string `json:"epg_channel_id,omitempty"`
}

type tv2ChannelCacheEntry struct {
	channels  []tv2Channel
	expiresAt time.Time
}

var tv2ChannelsCache = struct {
	sync.RWMutex
	entry tv2ChannelCacheEntry
}{}

// TV2Handler serves a traditional TV-like fullscreen channel experience
// at /tv2. Playback runs in the existing DRM-aware player iframe while the
// TV2 page retains ownership of its fullscreen UI and remote controls.
func TV2Handler(c *fiber.Ctx) error {
	if cached := getTV2CachedChannels(); cached != nil {
		return renderTV2(c, cached)
	}

	channels, err := television.Channels()
	if err != nil {
		return ErrorMessageHandler(c, err)
	}
	jioChannels := append([]television.Channel(nil), channels.Result...)

	if len(config.Cfg.Plugins) > 0 {
		pluginChannels := plugins.GetChannels()
		channels.Result = append(channels.Result, pluginChannels...)
		if len(pluginChannels) > 0 {
			// Warm the plugin guide source while the TV2 page is rendering so the
			// first channel preview normally does not wait on the external feed.
			prewarmTV2PluginEPG(pluginChannels[0].ID)
		}
	}

	displayChannels := channels.Result
	if len(config.Cfg.DefaultCategories) > 0 || len(config.Cfg.DefaultLanguages) > 0 {
		displayChannels = television.FilterChannelsByDefaults(displayChannels, config.Cfg.DefaultCategories, config.Cfg.DefaultLanguages)
	}

	chs := make([]tv2Channel, len(displayChannels))
	for i, ch := range displayChannels {
		var playerURL string
		var epgChannelID string
		// Match /tv/play's routing: resolve plugin ownership through the plugin
		// manager instead of relying on the channel's embedded metadata.
		if pluginID, ok := plugins.GetChannelPluginID(ch.ID); ok {
			// af=1 enables the HLS player's DRM-equivalent autoplay policy:
			// attempt unmuted playback, then retry muted if the browser requires it.
			playerURL = "/" + pluginID + "/player/" + ch.ID + "?q=high&af=1"
			if !isTV2NumericChannelID(ch.ID) {
				epgChannelID = findTV2JioEPGChannelID(ch.Name, jioChannels)
			}
		} else {
			// /mpd handles DRM with Shaka and falls back to the HLS player when
			// the channel has no usable DRM MPD.
			// af=1 asks the HLS fallback to follow the DRM player's autoplay
			// policy: try unmuted first, then retry muted if required.
			playerURL = "/mpd/" + ch.ID + "?q=high&af=1"
		}

		chs[i] = tv2Channel{
			ID:           ch.ID,
			Name:         ch.Name,
			PlayerURL:    playerURL,
			EPGChannelID: epgChannelID,
		}
	}
	putTV2CachedChannels(chs)
	return renderTV2(c, chs)
}

func isTV2NumericChannelID(channelID string) bool {
	_, err := strconv.Atoi(channelID)
	return err == nil
}

func findTV2JioEPGChannelID(channelName string, jioChannels []television.Channel) string {
	bestID := ""
	bestScore := 0
	for _, channel := range jioChannels {
		if !isTV2NumericChannelID(channel.ID) {
			continue
		}
		score := tv2EPGNameMatchScore(channelName, channel.Name)
		if score > bestScore {
			bestID = channel.ID
			bestScore = score
		}
	}
	return bestID
}

func getTV2CachedChannels() []tv2Channel {
	tv2ChannelsCache.RLock()
	entry := tv2ChannelsCache.entry
	tv2ChannelsCache.RUnlock()
	if entry.channels == nil || time.Now().After(entry.expiresAt) {
		return nil
	}
	return append([]tv2Channel(nil), entry.channels...)
}

func putTV2CachedChannels(channels []tv2Channel) {
	tv2ChannelsCache.Lock()
	tv2ChannelsCache.entry = tv2ChannelCacheEntry{
		channels:  append([]tv2Channel(nil), channels...),
		expiresAt: time.Now().Add(tv2ChannelsCacheTTL),
	}
	tv2ChannelsCache.Unlock()
}

func renderTV2(c *fiber.Ctx, channels []tv2Channel) error {
	jsonBytes, err := json.Marshal(channels)
	if err != nil {
		return ErrorMessageHandler(c, err)
	}

	// Escape </script> in JSON to prevent premature script tag closure
	jsonStr := strings.ReplaceAll(string(jsonBytes), "</script>", "<\\/script>")

	internalUtils.SetCacheHeader(c, 0)

	return c.Render("views/tv2_index", fiber.Map{
		"Title":              Title,
		"ChannelsJSON":       template.JS(jsonStr),
		"FavoriteChannelIDs": config.Cfg.FavoriteChannelIDs,
	})
}
