package handlers

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	"github.com/jiotv-go/jiotv_go/v3/internal/plugins"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
	"github.com/jiotv-go/jiotv_go/v3/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// TVIndexHandler serves the TV-optimized channel grid for WebOS / Smart TV clients.
func TVIndexHandler(c *fiber.Ctx) error {
	channels, err := television.Channels()
	if err != nil {
		return ErrorMessageHandler(c, err)
	}

	if len(config.Cfg.Plugins) > 0 {
		pluginChannels := plugins.GetChannels()
		channels.Result = append(channels.Result, pluginChannels...)
	}

	quality := c.Query("q")
	if quality == "" {
		quality = "auto"
	}

	hostURL := c.Protocol() + "://" + c.Hostname()

	// Apply filters first, then build the enriched channel list
	language := c.Query("language")
	category := c.Query("category")

	displayChannels := channels.Result
	if language != "" || category != "" {
		// Keep /tv in sync with the main index route: each filter may contain
		// a comma-separated list and an omitted filter must remain unspecified.
		displayChannels = television.FilterChannelsByDefaults(
			displayChannels,
			parseTVFilter(category),
			parseTVFilter(language),
		)
	} else if len(config.Cfg.DefaultCategories) > 0 || len(config.Cfg.DefaultLanguages) > 0 {
		displayChannels = television.FilterChannelsByDefaults(displayChannels, config.Cfg.DefaultCategories, config.Cfg.DefaultLanguages)
	}

	// Enrich with logo URLs and player URLs
	type tvChannel struct {
		television.Channel
		PlayerURL string `json:"player_url"`
		StreamURL string `json:"stream_url"`
		UsePlayer bool   `json:"use_player"`
	}
	tvChannels := make([]tvChannel, len(displayChannels))
	for i, ch := range displayChannels {
		var logoURL string
		if strings.HasPrefix(ch.LogoURL, "http://") || strings.HasPrefix(ch.LogoURL, "https://") {
			logoURL = ch.LogoURL
		} else {
			logoURL = hostURL + "/jtvimage/" + ch.LogoURL
		}
		playURL, streamURL, usePlayer := tvPlaybackURLs(ch, quality)
		tvChannels[i] = tvChannel{
			Channel:   ch,
			PlayerURL: playURL,
			StreamURL: streamURL,
			UsePlayer: usePlayer,
		}
		tvChannels[i].LogoURL = logoURL
	}

	// Never cache the TV page — stale data breaks fav filter
	internalUtils.SetCacheHeader(c, 0)

	return c.Render("views/tv_index", fiber.Map{
		"Title":              Title,
		"Channels":           tvChannels,
		"Categories":         television.CategoryMap,
		"Languages":          television.LanguageMap,
		"FavoriteChannelIDs": config.Cfg.FavoriteChannelIDs,
		"Quality":            quality,
		"Qualities": map[string]string{
			"auto":   "Auto",
			"high":   "High",
			"medium": "Medium",
			"low":    "Low",
		},
	})
}

func parseTVFilter(value string) []int {
	if value == "" {
		return nil
	}

	values := make([]int, 0)
	for _, item := range strings.Split(value, ",") {
		parsed, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil {
			values = append(values, parsed)
		}
	}
	return values
}

func tvPlaybackURLs(ch television.Channel, quality string) (playerURL, streamURL string, usePlayer bool) {
	if ch.IsCustom && ch.PluginID != "" {
		return "/" + ch.PluginID + "/player/" + ch.ID + "?q=" + url.QueryEscape(quality), "/" + ch.PluginID + "/" + ch.ID, false
	}

	streamURL = utils.BuildHLSPlayURL(quality, ch.ID)
	if EnableDRM && !ch.IsCustom && (ch.KeyURL != "" || utils.ContainsString(ch.ID, drmList)) {
		// A plain video element cannot negotiate Widevine. Send known DRM
		// channels through the upstream web-player contract instead.
		return "/mpd/" + ch.ID + "?q=" + url.QueryEscape(quality), streamURL, true
	}

	return "/player/" + ch.ID + "?q=" + url.QueryEscape(quality), streamURL, false
}

// TVPlayHandler serves the TV-optimized player page with fullscreen iframe.
func TVPlayHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	quality := c.Query("q")
	channelName := c.Query("name")

	if quality == "" {
		quality = "auto"
	}

	if channelName == "" {
		decoded, err := url.QueryUnescape(channelName)
		if err == nil {
			channelName = decoded
		}
	}
	channelName, _ = url.QueryUnescape(channelName)

	if channelName == "" {
		channelName = id
	}

	if err := EnsureFreshTokens(); err != nil {
		utils.Log.Printf("Failed to ensure fresh tokens for TV player: %v", err)
	}

	var player_url string
	if pluginID, ok := plugins.GetChannelPluginID(id); ok {
		player_url = "/" + pluginID + "/player/" + id + "?q=" + quality
	} else if EnableDRM {
		if utils.ContainsString(id, drmList) {
			player_url = "/mpd/" + id + "?q=" + quality
		} else if isCustomChannel(id) {
			player_url = "/player/" + id + "?q=" + quality
		} else {
			player_url = "/mpd/" + id + "?q=" + quality
		}
	} else {
		player_url = "/player/" + id + "?q=" + quality
	}

	internalUtils.SetCacheHeader(c, 3600)
	return c.Render("views/tv_play", fiber.Map{
		"Title":       Title,
		"player_url":  player_url,
		"ChannelID":   id,
		"ChannelName": channelName,
	})
}
