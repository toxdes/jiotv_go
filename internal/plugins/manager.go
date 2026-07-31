package plugins

import (
	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	"github.com/jiotv-go/jiotv_go/v3/pkg/plugins/external"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
	"github.com/jiotv-go/jiotv_go/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

var activePlugins []*external.Plugin

func Init(app *fiber.App) {
	for name, configURL := range config.Cfg.Plugins {
		plugin, err := external.New(name, configURL)
		if err != nil {
			utils.Log.Printf("Plugin %s init error: %v", name, err)
			continue
		}
		plugin.RegisterRoutes(app)
		activePlugins = append(activePlugins, plugin)
		utils.Log.Printf("Plugin %s registered", name)
	}
}

func GetChannels() []television.Channel {
	var channels []television.Channel
	for _, p := range activePlugins {
		channels = append(channels, p.Channels()...)
	}
	return channels
}

func GetChannelPluginID(id string) (string, bool) {
	for _, ch := range GetChannels() {
		if ch.ID == id && ch.PluginID != "" {
			return ch.PluginID, true
		}
	}
	return "", false
}
