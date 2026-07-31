package handlers

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/jiotv-go/jiotv_go/v3/internal/config"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
	"github.com/jiotv-go/jiotv_go/v3/pkg/simpleproxy"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
	"github.com/jiotv-go/jiotv_go/v3/pkg/utils"
	"github.com/valyala/fasthttp"
)

// defaultFallbackURL is the fallback stream URL.
const defaultFallbackURL = "https://video.twimg.com/amplify_video/1797150287292981248/pl/-GLBpWJuiNKBrdvp.m3u8"

// SimpleLiveHandler serves a rewritten M3U8 playlist.
//
// Flow:
//  1. Calls JioTV API to get the HLS URL for the channel
//  2. Fetches the M3U8 playlist from the CDN (captures Set-Cookie)
//  3. Rewrites all URIs to point back to /simple/proxy
//  4. Returns the modified playlist to the client
//
// GET /simple/live/:id.m3u8
func SimpleLiveHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	id = strings.Replace(id, ".m3u8", "", 1)

	quality := c.Query("q")
	if quality == "" {
		quality = "auto"
	}

	// Custom channels are served directly (no proxy needed)
	if isCustomChannel(id) {
		channel, exists := television.GetCustomChannelByID(id)
		if !exists {
			return internalUtils.NotFoundError(c, fmt.Sprintf("Custom channel with ID %s not found", id))
		}
		return c.Redirect(channel.URL, fiber.StatusFound)
	}

	// Ensure tokens are fresh before API call
	if err := EnsureFreshTokens(); err != nil {
		utils.Log.Printf("SimpleLiveHandler: failed to ensure fresh tokens: %v", err)
	}

	liveResult, err := TV.Live(id)
	if err != nil {
		utils.Log.Printf("SimpleLiveHandler: TV.Live(%s) failed: %v", id, err)
		return serveFallbackStream(c, id)
	}

	liveURL := selectBestLiveHLSURL(liveResult, quality)
	if liveURL == "" {
		utils.Log.Printf("SimpleLiveHandler: no HLS URL for channel %s", id)
		return serveFallbackStream(c, id)
	}
	liveURL = toAbsoluteStreamURL(liveURL, liveResult)

	// Fetch the M3U8 from the CDN, capturing response headers for cookies
	m3u8Content, cookieValue, err := fetchURLWithCookie(liveURL, "")
	if err != nil {
		utils.Log.Printf("SimpleLiveHandler: failed to fetch M3U8 from CDN: %v", err)
		return serveFallbackStream(c, id)
	}

	// Hex-encode the cookie for the URL param
	var cookieHex string
	if cookieValue != "" {
		cookieHex = hex.EncodeToString([]byte(cookieValue))
	}

	baseURL := simpleproxy.ExtractBaseURL(liveURL)
	serverHost := c.Protocol() + "://" + c.Hostname()

	rewritten, err := simpleproxy.RewritePlaylist(string(m3u8Content), baseURL, simpleproxy.RewriteConfig{
		ChannelID: id,
		CookieHex: cookieHex,
		ServerHost: serverHost,
	})
	if err != nil {
		utils.Log.Printf("SimpleLiveHandler: RewritePlaylist failed: %v", err)
		return internalUtils.InternalServerError(c, err)
	}

	setSimpleM3U8Headers(c)
	return c.SendString(rewritten)
}

// SimpleProxyHandler handles all sub-resource proxying for the simple playback mode.
// It dispatches on the query parameter present: hls, ts, or pkey.
//
// GET /simple/proxy?hls=<encrypted>&c=<cookie>&id=<ch>
// GET /simple/proxy?ts=<encrypted>&c=<cookie>&id=<ch>
// GET /simple/proxy?pkey=<url>&c=<cookie>&id=<ch>
func SimpleProxyHandler(c *fiber.Ctx) error {
	cookieHex := c.Query("c")
	channelID := c.Query("id")
	serverHost := c.Protocol() + "://" + c.Hostname()

	// Decode cookie from hex
	var cookieValue string
	if cookieHex != "" {
		decoded, err := hex.DecodeString(cookieHex)
		if err == nil {
			cookieValue = string(decoded)
		}
	}

	switch {
	case c.Query("hls") != "":
		return handleSimpleHLS(c, c.Query("hls"), cookieValue, channelID, serverHost)

	case c.Query("ts") != "":
		return handleSimpleTS(c, c.Query("ts"), cookieValue, channelID)

	case c.Query("pkey") != "":
		return handleSimplePKey(c, c.Query("pkey"), cookieValue, channelID)

	default:
		return serveFallbackStream(c, channelID)
	}
}

// handleSimpleHLS handles sub-M3U8 playlist requests.
func handleSimpleHLS(c *fiber.Ctx, encryptedURL, cookieValue, channelID, serverHost string) error {
	decryptedURL, err := simpleproxy.Decrypt(encryptedURL)
	if err != nil {
		utils.Log.Printf("SimpleProxy: hls decrypt failed: %v", err)
		return internalUtils.BadRequestError(c, "invalid hls param")
	}

	baseURL := simpleproxy.ExtractBaseURL(decryptedURL)
	m3u8Content, newCookie, err := fetchURLWithCookie(decryptedURL, cookieValue)
	if err != nil {
		utils.Log.Printf("SimpleProxy: failed to fetch sub-M3U8: %v", err)
		return serveFallbackStream(c, channelID)
	}

	// Use the latest cookie from the response, falling back to the one we sent
	updatedCookie := cookieValue
	if newCookie != "" {
		updatedCookie = newCookie
	}
	var cookieHex string
	if updatedCookie != "" {
		cookieHex = hex.EncodeToString([]byte(updatedCookie))
	}

	rewritten, err := simpleproxy.RewriteSubPlaylist(string(m3u8Content), baseURL, simpleproxy.RewriteConfig{
		ChannelID:  channelID,
		CookieHex:  cookieHex,
		ServerHost: serverHost,
	})
	if err != nil {
		utils.Log.Printf("SimpleProxy: RewriteSubPlaylist failed: %v", err)
		return internalUtils.InternalServerError(c, err)
	}

	setSimpleM3U8Headers(c)
	return c.SendString(rewritten)
}

// setHdneaCookie sets the __hdnea__ cookie on an upstream request using the
// standard SetCookie method (matching Television.Render).  The cookieValue may
// be the raw HDNEA token ("st=...~exp=...~hmac=...") or prefixed.
func setHdneaCookie(c *fiber.Ctx, cookieValue string) {
	if cookieValue == "" {
		return
	}
	hdnea := strings.TrimPrefix(cookieValue, "__hdnea__=")
	hdnea = strings.TrimPrefix(hdnea, "hdnea=")
	c.Request().Header.SetCookie("__hdnea__", hdnea)
}

// handleSimpleTS proxies a TS segment request.
func handleSimpleTS(c *fiber.Ctx, encryptedURL, cookieValue, channelID string) error {
	decryptedURL, err := simpleproxy.Decrypt(encryptedURL)
	if err != nil {
		utils.Log.Printf("SimpleProxy: ts decrypt failed: %v", err)
		return internalUtils.BadRequestError(c, "invalid ts param")
	}

	setHdneaCookie(c, cookieValue)
	setSimpleUpstreamHeaders(c, channelID)
	c.Response().Header.Set("Content-Type", "video/mp2t")

	if err := proxy.Do(c, decryptedURL, TV.Client); err != nil {
		utils.Log.Printf("SimpleProxy: ts proxy failed: %v", err)
		return err
	}

	c.Response().Header.Del(fiber.HeaderServer)
	return nil
}

// handleSimplePKey proxies a key/license URL request.
func handleSimplePKey(c *fiber.Ctx, keyURL, cookieValue, channelID string) error {
	setHdneaCookie(c, cookieValue)
	setSimpleUpstreamHeaders(c, channelID)

	if err := proxy.Do(c, keyURL, TV.Client); err != nil {
		utils.Log.Printf("SimpleProxy: pkey proxy failed: %v", err)
		return err
	}

	c.Response().Header.Del(fiber.HeaderServer)
	return nil
}

// fetchURLWithCookie fetches a URL using fasthttp and returns the response body
// plus the first __hdnea__ Set-Cookie value.  If cookieValue is non-empty it's
// set as the __hdnea__ cookie on the request.  This mirrors the header/a
// handling in the standard pipeline's Television.Render().
func fetchURLWithCookie(targetURL string, cookieValue string) ([]byte, string, error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(targetURL)
	req.Header.SetMethod("GET")

	// Copy standard JioTV headers (includes appkey, deviceId, etc.)
	for key, value := range TV.Headers {
		req.Header.Set(key, value)
	}
	// Override User-Agent to player UA — some CDNs block okhttp
	req.Header.Set("User-Agent", PLAYER_USER_AGENT)
	// Add auth tokens
	if TV.AccessToken != "" {
		req.Header.Set("accesstoken", TV.AccessToken)
	}
	if TV.SsoToken != "" {
		req.Header.Set("ssotoken", TV.SsoToken)
	}
	req.Header.Set("srno", "230203144000")

	// Set HDNEA as a cookie (matching Television.Render)
	if cookieValue != "" {
		// cookieValue is typically "__hdnea__=st=...~exp=...~hmac=..."
		// Extract just the value part if it has the __hdnea__= prefix
		hdnea := strings.TrimPrefix(cookieValue, "__hdnea__=")
		hdnea = strings.TrimPrefix(hdnea, "hdnea=")
		req.Header.SetCookie("__hdnea__", hdnea)
	}

	if err := TV.Client.Do(req, resp); err != nil {
		return nil, "", err
	}

	body := make([]byte, len(resp.Body()))
	copy(body, resp.Body())

	// Extract __hdnea__ from Set-Cookie headers (matching TV.Render)
	var newHdnea string
	for _, setCookie := range resp.Header.PeekAll("Set-Cookie") {
		setCookieStr := string(setCookie)
		if !strings.Contains(setCookieStr, "__hdnea__=") {
			continue
		}
		parts := strings.Split(setCookieStr, ";")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if strings.HasPrefix(trimmed, "__hdnea__=") {
				newHdnea = strings.TrimPrefix(trimmed, "__hdnea__=")
				break
			}
		}
		if newHdnea != "" {
			break
		}
	}

	return body, newHdnea, nil
}

// setSimpleUpstreamHeaders sets the common JioTV headers on upstream requests
// made by the simple proxy handlers.
func setSimpleUpstreamHeaders(c *fiber.Ctx, channelID string) {
	for key, value := range TV.Headers {
		c.Request().Header.Set(key, value)
	}
	if TV.AccessToken != "" {
		c.Request().Header.Set("accesstoken", TV.AccessToken)
	}
	if TV.SsoToken != "" {
		c.Request().Header.Set("ssotoken", TV.SsoToken)
	}
	c.Request().Header.Set("User-Agent", PLAYER_USER_AGENT)
	c.Request().Header.Set("srno", "230203144000")
	if channelID != "" {
		c.Request().Header.Set("channelId", channelID)
	}
}

// setSimpleM3U8Headers sets the response headers for HLS playlist responses.
func setSimpleM3U8Headers(c *fiber.Ctx) {
	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Headers", "content-type, x-developed-by, x-powered-by, x-github-username, x-timestamp, x-readable-time")
	c.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")
}

// serveFallbackStream returns the hardcoded fallback M3U8 stream when a channel
// cannot be fetched.
func serveFallbackStream(c *fiber.Ctx, channelID string) error {
	fallbackURL := defaultFallbackURL
	if config.Cfg.SimplePlayback.FallbackURL != "" {
		fallbackURL = config.Cfg.SimplePlayback.FallbackURL
	}
	// Fetch and relay the fallback stream
	setSimpleM3U8Headers(c)
	return proxy.Do(c, fallbackURL, TV.Client)
}
