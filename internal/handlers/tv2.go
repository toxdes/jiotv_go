package handlers

import (
	"encoding/json"
	"html/template"
	"strings"

	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	"github.com/jiotv-go/jiotv_go/v3/internal/plugins"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
	"github.com/jiotv-go/jiotv_go/v3/pkg/utils"

	"github.com/gofiber/fiber/v2"
)

// TV2Handler serves a traditional TV-like fullscreen channel experience
// at /tv2. Uses a direct <video> element instead of an iframe so that
// user gestures (keypresses) on the page propagate to video.play().
func TV2Handler(c *fiber.Ctx) error {
	channels, err := television.Channels()
	if err != nil {
		return ErrorMessageHandler(c, err)
	}

	if len(config.Cfg.Plugins) > 0 {
		pluginChannels := plugins.GetChannels()
		channels.Result = append(channels.Result, pluginChannels...)
	}

	hostURL := c.Protocol() + "://" + c.Hostname()

	type tv2Channel struct {
		television.Channel
		PlayerURL string `json:"player_url,omitempty"`
		StreamURL string `json:"stream_url"`
	}

	displayChannels := channels.Result
	if len(config.Cfg.DefaultCategories) > 0 || len(config.Cfg.DefaultLanguages) > 0 {
		displayChannels = television.FilterChannelsByDefaults(displayChannels, config.Cfg.DefaultCategories, config.Cfg.DefaultLanguages)
	}

	chs := make([]tv2Channel, len(displayChannels))
	for i, ch := range displayChannels {
		var logoURL string
		if strings.HasPrefix(ch.LogoURL, "http://") || strings.HasPrefix(ch.LogoURL, "https://") {
			logoURL = ch.LogoURL
		} else {
			logoURL = hostURL + "/jtvimage/" + ch.LogoURL
		}

		var streamURL string
		var playerURL string
		if ch.IsCustom && ch.PluginID != "" {
			streamURL = "/" + ch.PluginID + "/" + ch.ID
			playerURL = "/" + ch.PluginID + "/player/" + ch.ID + "?q=auto"
		} else {
			streamURL = utils.BuildHLSPlayURL("auto", ch.ID)
		}

		chs[i] = tv2Channel{
			Channel:   ch,
			PlayerURL: playerURL,
			StreamURL: streamURL,
		}
		chs[i].LogoURL = logoURL
	}

	jsonBytes, err := json.Marshal(chs)
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
