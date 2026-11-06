package handlers

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
)

func setMediaHeaders(c *fiber.Ctx, upstreamURL, channelID string) {
	internalUtils.SetPlayerHeaders(c, PLAYER_USER_AGENT)
	for _, name := range []string{
		"Authorization", "Proxy-Authorization",
		"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site", "Sec-Fetch-User",
	} {
		c.Request().Header.Del(name)
	}
	contextHeaders := []string{
		"ssotoken", "accesstoken", "subscriberId", "crmid", "uniqueId",
		"appkey", "deviceId", "devicetype", "os", "osVersion", "userId",
		"usergroup", "versionCode", "lbcookie", "channelid", "channel_id",
	}
	for _, name := range contextHeaders {
		c.Request().Header.Del(name)
	}
	parsed, err := url.Parse(upstreamURL)
	if err != nil || parsed.Scheme != "https" || TV == nil {
		return
	}
	host := strings.ToLower(parsed.Hostname())
	if !strings.HasSuffix(host, ".media.jio.com") && !strings.HasSuffix(host, ".cdn.jio.com") {
		return
	}
	for _, name := range contextHeaders {
		if value := TV.Headers[name]; value != "" {
			c.Request().Header.Set(name, value)
		}
	}
	c.Request().Header.Set("ssotoken", TV.SsoToken)
	c.Request().Header.Set("accesstoken", TV.AccessToken)
	c.Request().Header.Set("subscriberId", TV.Crm)
	c.Request().Header.Set("crmid", TV.Crm)
	c.Request().Header.Set("uniqueId", TV.UniqueID)
	if channelID != "" {
		c.Request().Header.Set("channelid", channelID)
		c.Request().Header.Set("channel_id", channelID)
	}
}
