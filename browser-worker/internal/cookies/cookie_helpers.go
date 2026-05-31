package cookies

import (
	"strings"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func FilterPreserved(cookies []session.Cookie, requirements []session.CookieRequirement) []session.Cookie {
	filtered := make([]session.Cookie, 0, len(cookies))
	seen := make(map[string]int)
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" || !cookiePreserved(cookie, requirements) {
			continue
		}
		if cookie.Path == "" {
			cookie.Path = "/"
		}
		key := strings.ToLower(cookie.Name + "\x00" + cookie.Domain + "\x00" + cookie.Path)
		if existing, ok := seen[key]; ok {
			filtered[existing] = cookie
			continue
		}
		seen[key] = len(filtered)
		filtered = append(filtered, cookie)
	}
	return filtered
}

func DefaultAccountUsername(platform string) string {
	switch platform {
	case "douyin":
		return "Connected Douyin account"
	case "zhihu":
		return "Connected Zhihu account"
	default:
		return "Connected account"
	}
}

func cookiePreserved(cookie session.Cookie, requirements []session.CookieRequirement) bool {
	for _, req := range requirements {
		if !req.Required && !req.Preserve {
			continue
		}
		if cookie.Name != req.Name {
			continue
		}
		for _, suffix := range req.DomainSuffixes {
			if DomainMatches(cookie.Domain, suffix) {
				return true
			}
		}
	}
	return false
}

func ValidateRequired(cookies []session.Cookie, requirements []session.CookieRequirement) (bool, []string) {
	var missing []string
	hasRequired := false
	for _, req := range requirements {
		if !req.Required {
			continue
		}
		hasRequired = true
		if !hasRequiredCookie(cookies, req) {
			missing = append(missing, req.Name)
		}
	}
	return hasRequired && len(missing) == 0, missing
}

func hasRequiredCookie(cookies []session.Cookie, req session.CookieRequirement) bool {
	for _, cookie := range cookies {
		if cookie.Name != req.Name || cookie.Value == "" {
			continue
		}
		for _, suffix := range req.DomainSuffixes {
			if DomainMatches(cookie.Domain, suffix) {
				return true
			}
		}
	}
	return false
}

func DomainMatches(domain, suffix string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	suffix = strings.TrimPrefix(strings.ToLower(suffix), ".")
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}
