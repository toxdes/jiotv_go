package external

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	internalUtils "github.com/jiotv-go/jiotv_go/v3/internal/utils"
	"github.com/jiotv-go/jiotv_go/v3/pkg/secureurl"
)

func transformURL(relURLStr string, baseURL *url.URL, hostPrefix, routeName string) string {
	relURL, err := url.Parse(relURLStr)
	if err != nil {
		return relURLStr
	}

	absURL := baseURL.ResolveReference(relURL).String()
	codedURL, err := secureurl.EncryptURL(absURL)
	if err != nil {
		return absURL
	}

	path := relURL.Path
	if path == "" {
		path = relURL.String()
	}

	isM3U8 := strings.Contains(path, ".m3u8")
	isSegment := strings.Contains(path, ".ts") || strings.Contains(path, ".mp4") || strings.Contains(path, ".m4s") || strings.Contains(path, "init")

	newParams := url.Values{}
	newParams.Set("auth", codedURL)

	base := strings.TrimRight(hostPrefix, "/") + "/" + routeName

	if isM3U8 {
		return base + "/render/playlist.m3u8?" + newParams.Encode()
	}
	if isSegment {
		segmentType := "ts"
		if strings.Contains(path, ".mp4") || strings.Contains(path, ".m4s") || strings.Contains(path, "init") {
			segmentType = "mp4"
		}
		return base + "/render/segment." + segmentType + "?" + newParams.Encode()
	}

	return absURL
}

func (p *Plugin) handlePlaylist(c *fiber.Ctx, targetURLStr string) {
	if targetURLStr == "" {
		c.Status(fiber.StatusBadRequest).SendString("missing url")
		return
	}

	req, err := http.NewRequest("GET", targetURLStr, nil)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString("create request: " + err.Error())
		return
	}
	req.Header.Set("User-Agent", userAgent)
	if origin := p.GetOrigin(); origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer := p.GetReferer(); referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	resp, err := p.client.Do(req)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString("fetch playlist: " + err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Status(fiber.StatusBadGateway).SendString("upstream returned status " + http.StatusText(resp.StatusCode))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString("read playlist: " + err.Error())
		return
	}

	baseURL, err := url.Parse(targetURLStr)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString("invalid target url")
		return
	}

	hostURL := strings.ToLower(c.Protocol()) + "://" + c.Hostname()
	routeName := p.Name

	reMediaURI := regexp.MustCompile(`URI="([^"]+)"`)

	var processedLines []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			processedLines = append(processedLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "#EXT-X-MAP") || strings.HasPrefix(trimmed, "#EXT-X-MEDIA") {
			matches := reMediaURI.FindStringSubmatch(trimmed)
			if len(matches) > 1 {
				originalURI := matches[1]
				newURI := transformURL(originalURI, baseURL, hostURL, routeName)
				line = strings.Replace(line, originalURI, newURI, 1)
			}
			processedLines = append(processedLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			processedLines = append(processedLines, line)
			continue
		}

		newLine := transformURL(trimmed, baseURL, hostURL, routeName)
		processedLines = append(processedLines, newLine)
	}

	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	c.Set("Access-Control-Allow-Origin", "*")
	c.Send([]byte(strings.Join(processedLines, "\n")))
}

func (p *Plugin) LiveHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	id = strings.Replace(id, ".m3u8", "", 1)

	videoURL, found := p.urlCache.Get(id)
	if !found {
		var err error
		videoURL, err = p.FetchVideoToken(id)
		if err != nil {
			c.Status(fiber.StatusInternalServerError).SendString(err.Error())
			return nil
		}
		p.urlCache.Add(id, videoURL)
	}

	p.handlePlaylist(c, videoURL)
	return nil
}

func (p *Plugin) PlayHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	playerURL := "/" + p.Name + "/player/" + id

	internalUtils.SetCacheHeader(c, 3600)
	return c.Render("views/play", fiber.Map{
		"Title":      p.cfg.Name,
		"player_url": playerURL,
		"ChannelID":  id,
	})
}

func (p *Plugin) PlayerHandler(c *fiber.Ctx) error {
	id := c.Params("id")
	playURL := "/" + p.Name + "/" + id + ".m3u8"
	internalUtils.SetCacheHeader(c, 3600)
	return c.Render("views/player_hls", fiber.Map{
		"play_url": playURL,
	})
}

func (p *Plugin) RenderHandler(c *fiber.Ctx) error {
	codedURL, err := secureurl.DecryptURL(c.Query("auth"))
	if err != nil {
		c.Status(fiber.StatusBadRequest).SendString("invalid auth param")
		return nil
	}
	p.handlePlaylist(c, codedURL)
	return nil
}

func (p *Plugin) RenderTSChunkHandler(c *fiber.Ctx) error {
	p.ProxySegmentHandler(c)
	return nil
}

func (p *Plugin) RenderMP4ChunkHandler(c *fiber.Ctx) error {
	p.ProxySegmentHandler(c)
	return nil
}

func (p *Plugin) ProxySegmentHandler(c *fiber.Ctx) {
	codedURL := c.Query("auth")
	if codedURL == "" {
		c.Status(fiber.StatusBadRequest).SendString("missing auth param")
		return
	}

	targetURL, err := secureurl.DecryptURL(codedURL)
	if err != nil {
		c.Status(fiber.StatusBadRequest).SendString("invalid auth param")
		return
	}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return
	}
	req.Header.Set("User-Agent", userAgent)
	if origin := p.GetOrigin(); origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer := p.GetReferer(); referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	resp, err := p.client.Do(req)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Status(fiber.StatusBadGateway).SendString("upstream returned status " + http.StatusText(resp.StatusCode))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Set("Content-Type", ct)
	}
	c.Set("Access-Control-Allow-Origin", "*")
	c.Send(body)
}

func (p *Plugin) LogoHandler(c *fiber.Ctx) error {
	imageURL := c.Query("url")
	if imageURL == "" {
		c.Status(fiber.StatusBadRequest).SendString("missing url param")
		return nil
	}

	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return nil
	}
	req.Header.Set("User-Agent", userAgent)
	if origin := p.GetOrigin(); origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer := p.GetReferer(); referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Sec-Fetch-Dest", "image")
	req.Header.Set("Sec-Fetch-Mode", "no-cors")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, err := p.client.Do(req)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.Status(fiber.StatusBadGateway).SendString("upstream returned status " + http.StatusText(resp.StatusCode))
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.Status(fiber.StatusBadGateway).SendString(err.Error())
		return nil
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	c.Set("Content-Type", ct)
	c.Set("Cache-Control", "public, max-age=86400")
	c.Send(body)
	return nil
}

func (p *Plugin) RegisterRoutes(app *fiber.App) {
	prefix := "/" + p.Name
	app.Get(prefix+"/:id", p.LiveHandler)
	app.Get(prefix+"/play/:id", p.PlayHandler)
	app.Get(prefix+"/player/:id", p.PlayerHandler)
	app.Get(prefix+"/logo", p.LogoHandler)
	app.Get(prefix+"/render/playlist.m3u8", p.RenderHandler)
	app.Get(prefix+"/render/segment.ts", p.RenderTSChunkHandler)
	app.Get(prefix+"/render/segment.mp4", p.RenderMP4ChunkHandler)
}
