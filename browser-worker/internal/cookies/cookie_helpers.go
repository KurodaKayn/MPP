package cookies

import (
	"strings"
	"time"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func FilterPreserved(cookies []session.Cookie, requirements []session.CookieRequirement) []session.Cookie {
	filtered := make([]session.Cookie, 0, len(cookies))
	seen := make(map[string]int)
	now := time.Now()
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" || cookieExpired(cookie, now) || !cookiePreserved(cookie, requirements) {
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
	now := time.Now()
	for _, req := range requirements {
		if !req.Required {
			continue
		}
		hasRequired = true
		if !hasRequiredCookie(cookies, req, now) {
			missing = append(missing, req.Name)
		}
	}
	return hasRequired && len(missing) == 0, missing
}

func hasRequiredCookie(cookies []session.Cookie, req session.CookieRequirement, now time.Time) bool {
	for _, cookie := range cookies {
		if cookie.Name != req.Name || cookie.Value == "" || cookieExpired(cookie, now) {
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

func cookieExpired(cookie session.Cookie, now time.Time) bool {
	return cookie.Expires > 0 && !time.Unix(int64(cookie.Expires), 0).After(now)
}

func DomainMatches(domain, suffix string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	suffix = strings.TrimPrefix(strings.ToLower(suffix), ".")
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}
