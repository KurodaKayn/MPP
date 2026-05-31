package main

import (
	"net"
	"net/url"
	"strings"
)

// Reusing types defined in main.go for consistency
// (In a larger project, these would be in a shared 'pkg' or 'types' directory)

func IsDomainAllowed(rawURL string, rules []DomainRule) bool {
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
		schemeMatch := false
		for _, s := range rule.Schemes {
			if s == scheme {
				schemeMatch = true
				break
			}
		}
		if !schemeMatch {
			continue
		}

		// Check host
		if rule.Match == "exact" {
			if host == rule.Host {
				return true
			}
		} else if rule.Match == "suffix" {
			if host == rule.Host || strings.HasSuffix(host, "."+rule.Host) {
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
