package service

import (
	"net/url"
	"strings"
)

func buildOpenAIEndpointURL(base string, endpoint string) string {
	normalized := strings.TrimSpace(base)
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	relative := strings.TrimPrefix(endpoint, "/v1")
	parsed, err := url.Parse(normalized)
	if err != nil {
		return strings.TrimRight(normalized, "/") + endpoint
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, endpoint) && !strings.HasSuffix(path, relative) {
		if openAIBaseURLHasVersionSuffix(path) {
			path += relative
		} else {
			path += endpoint
		}
	}
	parsed.Path = path
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}

func buildOpenAIResponsesInputTokensURL(base string) string {
	return buildOpenAIEndpointURL(base, "/v1/responses/input_tokens")
}

func openAIBaseURLHasVersionSuffix(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}

	pathValue := ""
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		pathValue = parsed.Path
	} else if slash := strings.Index(trimmed, "/"); slash >= 0 {
		pathValue = trimmed[slash:]
	}

	pathValue = strings.TrimRight(pathValue, "/")
	if pathValue == "" {
		return false
	}
	lastSlash := strings.LastIndex(pathValue, "/")
	segment := pathValue
	if lastSlash >= 0 {
		segment = pathValue[lastSlash+1:]
	}
	return isOpenAIAPIVersionSegment(segment)
}

func isOpenAIAPIVersionSegment(segment string) bool {
	s := strings.ToLower(strings.TrimSpace(segment))
	if len(s) < 2 || s[0] != 'v' || !isASCIIDigit(s[1]) {
		return false
	}

	i := 1
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
	}
	if i == len(s) {
		return true
	}
	if s[i] == '.' {
		i++
		if i == len(s) || !isASCIIDigit(s[i]) {
			return false
		}
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
		}
		return i == len(s)
	}

	suffix := s[i:]
	return strings.HasPrefix(suffix, "alpha") ||
		strings.HasPrefix(suffix, "beta") ||
		strings.HasPrefix(suffix, "preview")
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}


// buildNewAPIManagementURL builds NewAPI management endpoints under host root /api/*.
// OpenAI-compatible base URLs often end with /v1; NewAPI's token usage and user APIs
// are NOT under that version prefix (e.g. https://host/api/usage/token).
func buildNewAPIManagementURL(base string, endpoint string) string {
	normalized := strings.TrimSpace(base)
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		// Best-effort fallback: strip trailing /vN then join.
		trimmed := strings.TrimRight(normalized, "/")
		if openAIBaseURLHasVersionSuffix(trimmed) {
			if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
				trimmed = trimmed[:idx]
			}
		}
		return strings.TrimRight(trimmed, "/") + endpoint
	}
	path := strings.TrimRight(parsed.Path, "/")
	// Strip one trailing OpenAI-style version segment (/v1, /v2beta, ...).
	if openAIBaseURLHasVersionSuffix(path) {
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			path = path[:idx]
		} else {
			path = ""
		}
	}
	// Avoid double /api if base already ends with /api.
	if strings.HasSuffix(path, "/api") && strings.HasPrefix(endpoint, "/api/") {
		endpoint = endpoint[len("/api"):]
	}
	parsed.Path = path + endpoint
	parsed.RawPath = ""
	parsed.Fragment = ""
	return parsed.String()
}
