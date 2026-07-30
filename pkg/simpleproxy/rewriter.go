package simpleproxy

import (
	"fmt"
	"net/url"
	"strings"
)

// RewriteConfig carries parameters needed to rewrite HLS playlists.
type RewriteConfig struct {
	ChannelID string
	// CookieHex is the hex-encoded cookie string from the CDN response.
	// Maps to the "thor" param.
	CookieHex string
	// ServerHost is our own server's host (e.g. "http://192.168.1.5:5001").
	ServerHost string
}

// rewriteURILine rewrites an HLS line that contains a URI="..." attribute.
// It replaces the URI value with a proxy URL that includes the original URI
// as the pkey parameter. Everything after the closing quote (e.g. ,IV=0x...)
// is preserved.
func rewriteURILine(line, serverHost, cookieHex, channelID string) string {
	idx := strings.Index(line, `URI="`)
	if idx < 0 {
		return line
	}
	// Find the closing quote of the URI value
	start := idx + 5 // len(`URI="`) = 5
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return line
	}
	end = start + end
	originalURI := line[start:end]

	proxyURL := fmt.Sprintf(
		`%s/simple/proxy?pkey=%s&c=%s&id=%s`,
		serverHost, originalURI, cookieHex, channelID,
	)

	// Everything before URI=" + the replacement + everything after the closing quote
	return line[:idx] + `URI="` + proxyURL + `"` + line[end+1:]
}

// RewritePlaylist rewrites a top-level M3U8 playlist so that all sub-resource
// URLs point back to our simple proxy endpoint.
//
// Rewrites:
//   - URI="<url>"  →  URI="/simple/proxy?pkey=<url>&c=<cookie>&id=<ch>"
//   - <sub.m3u8>   →  /simple/proxy?hls=<encrypted_full_url>&c=<cookie>&id=<ch>
//
// The BaseURL parameter is the CDN base URL (everything before the filename)
// extracted from the original playlist URL.
func RewritePlaylist(content string, baseURL string, cfg RewriteConfig) (string, error) {
	lines := strings.Split(content, "\n")
	var buf strings.Builder
	buf.Grow(len(content) + 256)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			buf.WriteByte('\n')
			continue
		}

		switch {
		case strings.Contains(trimmed, `URI="`):
			rewritten := rewriteURILine(trimmed, cfg.ServerHost, cfg.CookieHex, cfg.ChannelID)
			buf.WriteString(rewritten)
			buf.WriteByte('\n')

		case !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ".m3u8"):
			// Replace sub-playlist URLs with encrypted proxy URL.
			// wanda URL: token=...&thor=...&id=...&jane_foster=...&hls=<encrypted>
			fullURL := baseURL + trimmed
			encrypted, err := Encrypt(fullURL)
			if err != nil {
				return "", fmt.Errorf("encrypt hls param: %w", err)
			}
			proxyURL := fmt.Sprintf(
				"%s/simple/proxy?hls=%s&c=%s&id=%s",
				cfg.ServerHost, encrypted, cfg.CookieHex, cfg.ChannelID,
			)
			buf.WriteString(proxyURL)
			buf.WriteByte('\n')

		default:
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}

	result := buf.String()
	if strings.HasPrefix(result, "#EXTM3U\n") {
		meta := fmt.Sprintf("#DEVELOPED_BY_SimpleProxy\n#AUTHOR-%s\n", cfg.ServerHost)
		result = strings.Replace(result, "#EXTM3U\n", "#EXTM3U\n"+meta, 1)
	}

	return result, nil
}

// RewriteSubPlaylist rewrites a sub-M3U8 playlist (from the hls proxy handler)
// so that TS segments and key URIs point back to our proxy.
//
// Rewrites:
//   - URI="<url>" → URI="/simple/proxy?pkey=<url>&c=<cookie>&id=<ch>"
//   - <segment.ts> → /simple/proxy?ts=<encrypted_full_url>&c=<cookie>&id=<ch>
func RewriteSubPlaylist(content string, baseURL string, cfg RewriteConfig) (string, error) {
	lines := strings.Split(content, "\n")
	var buf strings.Builder
	buf.Grow(len(content) + 256)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			buf.WriteByte('\n')
			continue
		}

		switch {
		case strings.Contains(trimmed, `URI="`):
			rewritten := rewriteURILine(trimmed, cfg.ServerHost, cfg.CookieHex, cfg.ChannelID)
			buf.WriteString(rewritten)
			buf.WriteByte('\n')

		case !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ".ts"):
			fullURL := baseURL + trimmed
			encrypted, err := Encrypt(fullURL)
			if err != nil {
				return "", fmt.Errorf("encrypt ts param: %w", err)
			}
			proxyURL := fmt.Sprintf(
				"%s/simple/proxy?ts=%s&c=%s&id=%s",
				cfg.ServerHost, encrypted, cfg.CookieHex, cfg.ChannelID,
			)
			buf.WriteString(proxyURL)
			buf.WriteByte('\n')

		default:
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}

	result := buf.String()
	if strings.HasPrefix(result, "#EXTM3U\n") {
		meta := fmt.Sprintf("#DEVELOPED_BY_SimpleProxy\n#AUTHOR-%s\n", cfg.ServerHost)
		result = strings.Replace(result, "#EXTM3U\n", "#EXTM3U\n"+meta, 1)
	}

	return result, nil
}

// ExtractBaseURL extracts the base CDN URL (everything up to and including the
// last / before the filename) from a playlist URL.
//
// Example:
//
//	Input:  "https://cdn.example.com/path/to/stream.m3u8?token=abc"
//	Output: "https://cdn.example.com/path/to/"
func ExtractBaseURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	u.RawQuery = ""
	u.Fragment = ""
	path := u.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[:idx+1]
	} else {
		path = "/"
	}
	u.Path = path
	return u.String()
}
