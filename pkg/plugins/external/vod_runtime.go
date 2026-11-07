package external

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type VODRequest struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Query     map[string]any    `json:"query"`
	Body      any               `json:"body"`
	ErrorPath string            `json:"error_path"`
}

type VODCatalogue struct {
	Request   VODRequest        `json:"request"`
	ItemsPath string            `json:"items_path"`
	Fields    map[string]string `json:"fields"`
}

type VODSeries struct {
	Seasons  VODCatalogue `json:"seasons"`
	Episodes VODCatalogue `json:"episodes"`
	PageSize int          `json:"page_size"`
}

type VODPlaybackConfig struct {
	Request VODRequest        `json:"request"`
	Fields  map[string]string `json:"fields"`
}

type VODFallback struct {
	Language       string    `json:"language"`
	Items          []vodItem `json:"items"`
	AllowedRatings []string  `json:"allowed_ratings"`
}

type VODScheduleConfig struct {
	Mode     string `json:"mode"`
	Timezone string `json:"timezone"`
	Logo     bool   `json:"logo"`
}

type VODConfig struct {
	Schedules                map[string]VODScheduleConfig `json:"schedules,omitempty"`
	Fallbacks                map[string]VODFallback       `json:"fallbacks,omitempty"`
	CertificateResponseField int                          `json:"certificate_response_field"`
	CertificateRequest       string                       `json:"certificate_request"`
	ChannelPrefix            string                       `json:"channel_prefix"`
	LanguageCodes            map[string]string            `json:"language_codes"`
	AssetTypes               map[string]int               `json:"asset_types"`
	Constants                map[string]any               `json:"constants"`
	SessionCookies           map[string]string            `json:"session_cookies"`
	SessionDefaults          map[string]any               `json:"session_defaults"`
	RequiredSession          []string                     `json:"required_session"`
	Series                   *VODSeries                   `json:"series,omitempty"`
	Catalogue                VODCatalogue                 `json:"catalogue"`
	Playback                 VODPlaybackConfig            `json:"playback"`
	License                  VODRequest                   `json:"license"`
}

var vodVariablePattern = regexp.MustCompile(`\{\{([A-Za-z0-9_.:]+)\}\}`)

func vodPathValues(value any, path string) []any {
	if path == "" {
		return nil
	}
	var visit func(any, []string) []any
	visit = func(current any, parts []string) []any {
		if len(parts) == 0 {
			return []any{current}
		}
		if parts[0] == "*" {
			var result []any
			if list, ok := current.([]any); ok {
				for _, item := range list {
					result = append(result, visit(item, parts[1:])...)
				}
			}
			return result
		}
		if object, ok := current.(map[string]any); ok {
			if child, exists := object[parts[0]]; exists {
				return visit(child, parts[1:])
			}
		}
		return nil
	}
	return visit(value, strings.Split(path, "."))
}

func vodPath(value any, path string) any {
	items := vodPathValues(value, path)
	if len(items) > 0 {
		return items[0]
	}
	return nil
}
func vodString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
func vodInt(value any) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := strconv.Atoi(string(n))
		return i
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}
func vodBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	default:
		return vodInt(value) != 0
	}
}

func expandVOD(value any, variables map[string]any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			expanded, err := expandVOD(item, variables)
			if err != nil {
				return nil, err
			}
			result[key] = expanded
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			expanded, err := expandVOD(item, variables)
			if err != nil {
				return nil, err
			}
			result[i] = expanded
		}
		return result, nil
	case string:
		resolve := func(expression string) (any, error) {
			base64Value := strings.HasPrefix(expression, "base64:")
			expression = strings.TrimPrefix(expression, "base64:")
			found := vodPathValues(variables, expression)
			if len(found) == 0 {
				return nil, fmt.Errorf("runtime request references an unavailable variable")
			}
			if base64Value {
				encoded, err := json.Marshal(found[0])
				if err != nil {
					return nil, err
				}
				return base64.StdEncoding.EncodeToString(encoded), nil
			}
			return found[0], nil
		}
		if match := vodVariablePattern.FindStringSubmatch(v); len(match) > 0 && match[0] == v {
			return resolve(match[1])
		}
		var failure error
		result := vodVariablePattern.ReplaceAllStringFunc(v, func(token string) string {
			match := vodVariablePattern.FindStringSubmatch(token)
			expanded, err := resolve(match[1])
			if err != nil {
				failure = err
				return ""
			}
			return fmt.Sprint(expanded)
		})
		return result, failure
	default:
		return value, nil
	}
}

func (p *Plugin) vodVariables(session *vodSession) map[string]any {
	encoded, _ := json.Marshal(session)
	var values map[string]any
	_ = json.Unmarshal(encoded, &values)
	for key, value := range p.cfg.VOD.SessionDefaults {
		if values[key] == nil || values[key] == "" {
			values[key] = value
		}
	}
	return map[string]any{"session": values, "constants": p.cfg.VOD.Constants}
}

func (p *Plugin) vodBuildRequest(session *vodSession, config VODRequest, variables map[string]any) (*http.Request, error) {
	target, err := expandVOD(config.URL, variables)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(vodString(target))
	if err != nil || !vodSameProvider(session.Origin, u.String()) {
		return nil, fmt.Errorf("runtime API URL must use HTTPS on the session provider's domain")
	}
	query := u.Query()
	for key, value := range config.Query {
		expanded, err := expandVOD(value, variables)
		if err != nil {
			return nil, err
		}
		query.Set(key, fmt.Sprint(expanded))
	}
	u.RawQuery = query.Encode()
	payload, err := expandVOD(config.Body, variables)
	if err != nil {
		return nil, err
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("cannot encode runtime request")
		}
	}
	method := config.Method
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, fmt.Errorf("runtime API requests only support GET and POST")
	}
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid runtime request")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range config.Headers {
		expanded, err := expandVOD(value, variables)
		if err != nil {
			return nil, err
		}
		req.Header.Set(key, fmt.Sprint(expanded))
	}
	return req, nil
}
