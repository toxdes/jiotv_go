package handlers

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/jiotv-go/jiotv_go/v3/internal/config"
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

	quality := c.Query("q")
	if quality == "" {
		quality = "auto"
	}

	hostURL := c.Protocol() + "://" + c.Hostname()
	for i, channel := range channels.Result {
		if strings.HasPrefix(channel.LogoURL, "http://") || strings.HasPrefix(channel.LogoURL, "https://") {
			channels.Result[i].LogoURL = channel.LogoURL
		} else {
			channels.Result[i].LogoURL = hostURL + "/jtvimage/" + channel.LogoURL
		}
	}

	language := c.Query("language")
	category := c.Query("category")

	tvContext := fiber.Map{
		"Title":      Title,
		"Channels":   nil,
		"Categories": television.CategoryMap,
		"Languages":  television.LanguageMap,
		"Qualities": map[string]string{
			"auto":   "Auto",
			"high":   "High",
			"medium": "Medium",
			"low":    "Low",
		},
	}

	if language != "" || category != "" {
		language_int, err := strconv.Atoi(language)
		if err != nil {
			return ErrorMessageHandler(c, err)
		}
		category_int, err := strconv.Atoi(category)
		if err != nil {
			return ErrorMessageHandler(c, err)
		}
		channels_list := television.FilterChannels(channels.Result, language_int, category_int)
		tvContext["Channels"] = channels_list
		return c.Render("views/tv_index", tvContext)
	}

	if len(config.Cfg.DefaultCategories) > 0 || len(config.Cfg.DefaultLanguages) > 0 {
		channels_list := television.FilterChannelsByDefaults(channels.Result, config.Cfg.DefaultCategories, config.Cfg.DefaultLanguages)
		tvContext["Channels"] = channels_list
		return c.Render("views/tv_index", tvContext)
	}

	tvContext["Channels"] = channels.Result
	return c.Render("views/tv_index", tvContext)
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
	if EnableDRM {
		if utils.ContainsString(id, SONY_LIST) {
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
