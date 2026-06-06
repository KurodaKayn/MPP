package isolation

import (
	"net"
	"net/url"
	"slices"

	"github.com/kurodakayn/mpp-browser-worker/internal/cookies"
	"github.com/kurodakayn/mpp-browser-worker/internal/session"
)

func IsDomainAllowed(rawURL string, rules []session.DomainRule) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Only allow standard schemes unless rules say otherwise (usually https)
	host := u.Hostname()
	scheme := u.Scheme
	if blockedHost(host) {
		return false
	}

	for _, rule := range rules {
		// Check scheme
		if !slices.Contains(rule.Schemes, scheme) {
			continue
		}

		// Check host
		switch rule.Match {
		case "exact":
			if host == rule.Host {
				return true
			}
		case "suffix":
			if cookies.DomainMatches(host, rule.Host) {
				return true
			}
		}
	}

	return false
}

func blockedHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
