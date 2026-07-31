package handlers

import (
	"encoding/json"
	"html/template"
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
// only needs these three fields to render the guide and start playback.
type tv2Channel struct {
	ID        string `json:"channel_id"`
	Name      string `json:"channel_name"`
	PlayerURL string `json:"player_url"`
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

	if len(config.Cfg.Plugins) > 0 {
		pluginChannels := plugins.GetChannels()
		channels.Result = append(channels.Result, pluginChannels...)
	}

	displayChannels := channels.Result
	if len(config.Cfg.DefaultCategories) > 0 || len(config.Cfg.DefaultLanguages) > 0 {
		displayChannels = television.FilterChannelsByDefaults(displayChannels, config.Cfg.DefaultCategories, config.Cfg.DefaultLanguages)
	}

	chs := make([]tv2Channel, len(displayChannels))
	for i, ch := range displayChannels {
		var playerURL string
		if ch.IsCustom && ch.PluginID != "" {
			playerURL = "/" + ch.PluginID + "/player/" + ch.ID + "?q=high"
		} else {
			// /mpd handles DRM with Shaka and falls back to the HLS player when
			// the channel has no usable DRM MPD.
			playerURL = "/mpd/" + ch.ID + "?q=high"
		}

		chs[i] = tv2Channel{
			ID:        ch.ID,
			Name:      ch.Name,
			PlayerURL: playerURL,
		}
	}
	putTV2CachedChannels(chs)
	return renderTV2(c, chs)
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
