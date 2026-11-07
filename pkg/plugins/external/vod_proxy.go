package external

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jiotv-go/jiotv_go/v3/pkg/secureurl"
)

var vodTemplatePattern = regexp.MustCompile(`\$(RepresentationID|Number|Bandwidth|Time)(?:%0[0-9]+d)?\$`)
var vodTemplateValue = regexp.MustCompile(`^[A-Za-z0-9_./=-]{1,128}$`)

func (p *Plugin) vodResourceURL(stream string, target string) (string, error) {
	encoded, err := secureurl.EncryptURLDeterministic(target)
	if err != nil {
		return "", fmt.Errorf("cannot protect media URL")
	}
	local := "/" + p.Name + "/vod/resource/" + stream + "/" + encoded
	seen := make(map[string]bool)
	var query []string
	for _, match := range vodTemplatePattern.FindAllStringSubmatch(target, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			query = append(query, match[1]+"="+match[0])
		}
	}
	if len(query) > 0 {
		local += "?" + strings.Join(query, "&")
	}
	return local, nil
}

func (p *Plugin) vodResourceHandler(c *fiber.Ctx) error {
	if p.vodStreams == nil {
		return fiber.ErrNotFound
	}
	stream, ok := p.vodStreams.Get(c.Params("stream"))
	if !ok {
		return fiber.NewError(fiber.StatusGone, "Experimental playback expired; reopen the channel")
	}
	target, err := secureurl.DecryptURL(c.Params("auth"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	valid := true
	target = vodTemplatePattern.ReplaceAllStringFunc(target, func(template string) string {
		parts := vodTemplatePattern.FindStringSubmatch(template)
		value := c.Query(parts[1])
		if !vodTemplateValue.MatchString(value) || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") {
			valid = false
			return ""
		}
		for _, segment := range strings.Split(value, "/") {
			if segment == "." || segment == ".." {
				valid = false
				return ""
			}
		}
		if parts[1] != "RepresentationID" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			valid = false
			return ""
		}
		return value
	})
	if !valid {
		return fiber.ErrBadRequest
	}
	return p.vodServeMedia(c, stream, target)
}

func (p *Plugin) vodServeMedia(c *fiber.Ctx, stream *vodPlayback, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fiber.ErrBadRequest
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return fiber.ErrBadRequest
	}
	p.vodMediaHeaders(req)
	if value := c.Get("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Provider media request failed")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("Provider media returned HTTP %d", resp.StatusCode))
	}
	contentType := resp.Header.Get("Content-Type")
	path := resp.Request.URL.Path
	if strings.Contains(contentType, "dash+xml") || strings.HasSuffix(path, ".mpd") {
		defer resp.Body.Close()
		body, err := p.vodRewriteMPD(io.LimitReader(resp.Body, 8<<20), resp.Request.URL, stream.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "Cannot rewrite provider manifest")
		}
		c.Set("Content-Type", "application/dash+xml")
		c.Set("Cache-Control", "no-store")
		return c.Send(body)
	}
	if strings.Contains(contentType, "mpegurl") || strings.HasSuffix(path, ".m3u8") {
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "Cannot read provider playlist")
		}
		lines := strings.Split(string(body), "\n")
		uriPattern := regexp.MustCompile(`URI="([^"]+)"`)
		rewrite := func(reference string) string {
			relative, err := url.Parse(reference)
			if err != nil {
				return reference
			}
			local, err := p.vodResourceURL(stream.ID, resp.Request.URL.ResolveReference(relative).String())
			if err != nil {
				return reference
			}
			return local
		}
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "#") {
				lines[i] = uriPattern.ReplaceAllStringFunc(line, func(match string) string {
					parts := uriPattern.FindStringSubmatch(match)
					return `URI="` + rewrite(parts[1]) + `"`
				})
			} else {
				lines[i] = rewrite(line)
			}
		}
		c.Set("Content-Type", "application/vnd.apple.mpegurl")
		c.Set("Cache-Control", "no-store")
		return c.SendString(strings.Join(lines, "\n"))
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := resp.Header.Get(name); value != "" {
			c.Set(name, value)
		}
	}
	c.Status(resp.StatusCode)
	return c.SendStream(resp.Body)
}

func (p *Plugin) vodMediaHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", p.GetOrigin())
	req.Header.Set("Referer", p.GetReferer())
}

func (p *Plugin) vodLicenseHandler(c *fiber.Ctx) error {
	if p.vodStreams == nil {
		return fiber.ErrNotFound
	}
	stream, ok := p.vodStreams.Get(c.Params("stream"))
	if !ok {
		return fiber.NewError(fiber.StatusGone, "Experimental playback expired; reopen the channel")
	}
	if !stream.IsDRM {
		return fiber.ErrBadRequest
	}
	challenge := c.Body()
	if c.Method() == http.MethodGet {
		var err error
		challenge, err = base64.StdEncoding.DecodeString(p.cfg.VOD.CertificateRequest)
		if err != nil || len(challenge) == 0 {
			return fiber.ErrNotFound
		}
	}
	req, err := http.NewRequest(http.MethodPost, stream.LicenseURL, strings.NewReader(string(challenge)))
	if err != nil {
		return fiber.ErrBadRequest
	}
	p.vodMediaHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")
	for name, values := range stream.LicenseHeaders {
		req.Header[name] = append([]string(nil), values...)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	client := *p.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Provider license request failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "Cannot read provider license")
	}
	c.Set("Cache-Control", "no-store")
	if resp.StatusCode != http.StatusOK {
		return fiber.NewError(resp.StatusCode, "Provider license request rejected")
	}
	if c.Method() == http.MethodGet && p.cfg.VOD.CertificateResponseField > 0 {
		body, err = vodBinaryField(body, uint64(p.cfg.VOD.CertificateResponseField))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "Cannot read provider certificate")
		}
	}
	c.Set("Content-Type", "application/octet-stream")
	return c.Send(body)
}

func vodBinaryField(data []byte, field uint64) ([]byte, error) {
	for len(data) > 0 {
		tag, n := binary.Uvarint(data)
		if n <= 0 || tag>>3 == 0 {
			break
		}
		data = data[n:]
		switch tag & 7 {
		case 0:
			_, n = binary.Uvarint(data)
			if n <= 0 {
				return nil, fmt.Errorf("invalid binary certificate")
			}
			data = data[n:]
		case 1:
			if len(data) < 8 {
				return nil, fmt.Errorf("invalid binary certificate")
			}
			data = data[8:]
		case 2:
			size, used := binary.Uvarint(data)
			if used <= 0 || size > uint64(len(data)-used) {
				return nil, fmt.Errorf("invalid binary certificate")
			}
			data = data[used:]
			if tag>>3 == field && size > 0 {
				return data[:int(size)], nil
			}
			data = data[int(size):]
		case 5:
			if len(data) < 4 {
				return nil, fmt.Errorf("invalid binary certificate")
			}
			data = data[4:]
		default:
			return nil, fmt.Errorf("invalid binary certificate")
		}
	}
	return nil, fmt.Errorf("certificate field is unavailable")
}

type vodXMLNode struct {
	Start    xml.StartElement
	Children []any
}

func readVODXML(decoder *xml.Decoder, start xml.StartElement) (*vodXMLNode, error) {
	node := &vodXMLNode{Start: start}
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			child, err := readVODXML(decoder, value)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
		case xml.EndElement:
			return node, nil
		default:
			node.Children = append(node.Children, xml.CopyToken(value))
		}
	}
}

func (p *Plugin) vodRewriteMPD(reader io.Reader, base *url.URL, stream string) ([]byte, error) {
	decoder := xml.NewDecoder(reader)
	var root *vodXMLNode
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if start, ok := token.(xml.StartElement); ok {
			root, err = readVODXML(decoder, start)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if root.Start.Name.Local != "MPD" {
		return nil, fmt.Errorf("not an MPD")
	}
	var rewrite func(*vodXMLNode, *url.URL) error
	rewrite = func(node *vodXMLNode, parent *url.URL) error {
		current := parent
		for _, child := range node.Children {
			if value, ok := child.(*vodXMLNode); ok && value.Start.Name.Local == "BaseURL" {
				var text strings.Builder
				for _, part := range value.Children {
					if data, ok := part.(xml.CharData); ok {
						text.Write(data)
					}
				}
				relative, err := url.Parse(strings.TrimSpace(text.String()))
				if err != nil {
					return err
				}
				absolute := parent.ResolveReference(relative)
				if current == parent {
					current = absolute
				}
				local, err := p.vodResourceURL(stream, absolute.String())
				if err != nil {
					return err
				}
				value.Children = []any{xml.CharData(local)}
			}
		}
		for i, attribute := range node.Start.Attr {
			isURI := node.Start.Name.Local == "SegmentTemplate" && (attribute.Name.Local == "media" || attribute.Name.Local == "initialization") || node.Start.Name.Local == "SegmentURL" && (attribute.Name.Local == "media" || attribute.Name.Local == "index") || node.Start.Name.Local == "Initialization" && attribute.Name.Local == "sourceURL"
			if isURI {
				reference, err := url.Parse(attribute.Value)
				if err != nil {
					return err
				}
				local, err := p.vodResourceURL(stream, current.ResolveReference(reference).String())
				if err != nil {
					return err
				}
				node.Start.Attr[i].Value = local
			}
		}
		for _, child := range node.Children {
			if value, ok := child.(*vodXMLNode); ok && value.Start.Name.Local != "BaseURL" {
				if err := rewrite(value, current); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := rewrite(root, base); err != nil {
		return nil, err
	}
	var output strings.Builder
	encoder := xml.NewEncoder(&output)
	var write func(*vodXMLNode) error
	write = func(node *vodXMLNode) error {
		start := node.Start
		start.Attr = nil
		for _, attribute := range node.Start.Attr {
			if attribute.Name.Local != "xmlns" && attribute.Name.Space != "xmlns" {
				start.Attr = append(start.Attr, attribute)
			}
		}
		if err := encoder.EncodeToken(start); err != nil {
			return err
		}
		for _, child := range node.Children {
			if value, ok := child.(*vodXMLNode); ok {
				if err := write(value); err != nil {
					return err
				}
			} else if err := encoder.EncodeToken(child); err != nil {
				return err
			}
		}
		return encoder.EncodeToken(start.End())
	}
	if err := write(root); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}
