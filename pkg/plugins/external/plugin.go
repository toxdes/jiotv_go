package external

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/jiotv-go/jiotv_go/v3/pkg/television"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:147.0) Gecko/20100101 Firefox/147.0"

type RuntimeConfig struct {
	Name     string `json:"name"`
	Category int    `json:"category"`
	Channels []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		URL      string `json:"url"`
		Logo     string `json:"logo"`
		Language int    `json:"language"`
		Genre    int    `json:"genre"`
		Slug     string `json:"slug"`
	} `json:"channels"`
	API struct {
		PlaybackURL        string `json:"playback_url"`
		AuthURL            string `json:"auth_url"`
		PlatformTokenRegex string `json:"platform_token_regex"`
	} `json:"api"`
	Headers map[string]string `json:"headers"`
}

type Plugin struct {
	Name     string
	cfg      RuntimeConfig
	channels []television.Channel
	urlCache *expirable.LRU[string, string]
	client   *http.Client
}

func New(name, configURL string) (*Plugin, error) {
	rc, err := fetchConfig(configURL)
	if err != nil {
		return nil, fmt.Errorf("fetch config %s: %w", configURL, err)
	}

	var channels []television.Channel
	for _, item := range rc.Channels {
		channels = append(channels, television.Channel{
			ID:                 item.ID,
			Name:               item.Name,
			URL:                name + "/" + item.ID,
			LogoURL:            item.Logo,
			Category:           item.Genre,
			Language:           item.Language,
			IsHD:               false,
			IsCustom:           true,
			PluginID:           name,
			IsCatchupAvailable: false,
		})
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", userAgent)
			if origin := rc.Headers["Origin"]; origin != "" {
				req.Header.Set("Origin", origin)
			}
			if referer := rc.Headers["Referer"]; referer != "" {
				req.Header.Set("Referer", referer)
			}
			return nil
		},
	}

	return &Plugin{
		Name:     name,
		cfg:      *rc,
		channels: channels,
		urlCache: expirable.NewLRU[string, string](100, nil, time.Hour),
		client:   client,
	}, nil
}

func (p *Plugin) Channels() []television.Channel {
	return p.channels
}

func fetchConfig(url string) (*RuntimeConfig, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var rc RuntimeConfig
	if err := json.Unmarshal(b, &rc); err != nil {
		return nil, err
	}
	return &rc, nil
}

func (p *Plugin) GetOrigin() string {
	if p.cfg.Headers != nil {
		if o, ok := p.cfg.Headers["Origin"]; ok {
			return o
		}
	}
	return ""
}

func (p *Plugin) GetReferer() string {
	if p.cfg.Headers != nil {
		if r, ok := p.cfg.Headers["Referer"]; ok {
			return r
		}
	}
	return ""
}

func (p *Plugin) FetchPlatformToken() (string, error) {
	req, err := http.NewRequest("GET", p.cfg.API.AuthURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating auth request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if origin := p.GetOrigin(); origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer := p.GetReferer(); referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth GET: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth GET returned HTTP %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading auth body: %w", err)
	}

	re, err := regexp.Compile(p.cfg.API.PlatformTokenRegex)
	if err != nil {
		return "", fmt.Errorf("compiling token regex: %w", err)
	}
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("token not found in auth response (len=%d)", len(matches))
	}

	return string(matches[1]), nil
}

func generateDDToken() string {
	data := map[string]interface{}{
		"schema_version":   "1",
		"os_name":          "N/A",
		"os_version":       "N/A",
		"platform_name":    "Chrome",
		"platform_version": "104",
		"device_name":      "",
		"app_name":         "Web",
		"app_version":      "2.52.31",
		"player_capabilities": map[string]interface{}{
			"audio_channel": []string{"STEREO"},
			"video_codec":   []string{"H264"},
			"container":     []string{"MP4", "TS"},
			"package":       []string{"DASH", "HLS"},
			"resolution":    []string{"240p", "SD", "HD", "FHD"},
			"dynamic_range": []string{"SDR"},
		},
		"security_capabilities": map[string]interface{}{
			"encryption":              []string{"WIDEVINE_AES_CTR"},
			"widevine_security_level": []string{"L3"},
			"hdcp_version":            []string{"HDCP_V1", "HDCP_V2", "HDCP_V2_1", "HDCP_V2_2"},
		},
	}
	jsonBytes, _ := json.Marshal(data)
	return base64.StdEncoding.EncodeToString(jsonBytes)
}

func generateGuestToken() string {
	return uuid.New().String()
}

type playbackRequest struct {
	XAccessToken  string `json:"x-access-token"`
	XZ5GuestToken string `json:"X-Z5-Guest-Token"`
	XDDToken      string `json:"x-dd-token"`
}

type playbackResponse struct {
	KeyOsDetails struct {
		VideoToken string `json:"video_token"`
	} `json:"keyOsDetails"`
}

func (p *Plugin) FetchVideoToken(channelID string) (string, error) {
	platformToken, err := p.FetchPlatformToken()
	if err != nil {
		return "", fmt.Errorf("platform token: %w", err)
	}

	guestToken := generateGuestToken()
	ddToken := generateDDToken()

	q := url.Values{}
	q.Set("channel_id", channelID)
	q.Set("device_id", guestToken)
	q.Set("platform_name", "desktop_web")
	q.Set("translation", "en")
	q.Set("user_language", "en")
	q.Set("country", "IN")
	q.Set("state", "KA")
	q.Set("app_version", "5.8.0")
	q.Set("user_type", "guest")
	q.Set("check_parental_control", "false")
	q.Set("ppid", guestToken)
	q.Set("version", "15")

	body, _ := json.Marshal(playbackRequest{
		XAccessToken:  platformToken,
		XZ5GuestToken: guestToken,
		XDDToken:      ddToken,
	})

	req, err := http.NewRequest("POST", p.cfg.API.PlaybackURL+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating playback request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if origin := p.GetOrigin(); origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer := p.GetReferer(); referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("playback POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("playback POST returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var pr playbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return "", fmt.Errorf("decoding playback response: %w", err)
	}

	if pr.KeyOsDetails.VideoToken == "" {
		return "", fmt.Errorf("video_token not found in API response")
	}

	return pr.KeyOsDetails.VideoToken, nil
}
