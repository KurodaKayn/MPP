package rediskey

import "strings"

const Unknown = "unknown"

func Part(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return Unknown
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == '.' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return Unknown
	}
	return result
}

func Tag(scope string, value string) string {
	return "{" + Part(scope) + ":" + Part(value) + "}"
}

func ExtractTag(key string) (string, bool) {
	start := strings.IndexByte(key, '{')
	if start < 0 {
		return "", false
	}
	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return "", false
	}
	return key[start+1 : start+1+end], true
}

func ShareTag(keys ...string) bool {
	if len(keys) == 0 {
		return true
	}
	expected, ok := ExtractTag(keys[0])
	if !ok {
		return false
	}
	for _, key := range keys[1:] {
		tag, ok := ExtractTag(key)
		if !ok || tag != expected {
			return false
		}
	}
	return true
}
