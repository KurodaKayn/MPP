package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDomainAllowed(t *testing.T) {
	rules := []DomainRule{
		{Host: "douyin.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "test"},
		{Host: "snssdk.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "test"},
		{Host: "exact.com", Match: "exact", Schemes: []string{"https"}, Purpose: "test"},
	}

	tests := []struct {
		url     string
		allowed bool
	}{
		{"https://douyin.com", true},
		{"https://www.douyin.com", true},
		{"https://creator.douyin.com/home", true},
		{"https://snssdk.com/path", true},
		{"https://sub.snssdk.com", true},
		{"https://exact.com", true},
		
		// Blocked cases
		{"http://douyin.com", false},          // Wrong scheme
		{"https://evil-douyin.com", false},    // Not a subdomain (prefix match attempt)
		{"https://douyin.com.evil.com", false}, // Wrong suffix
		{"https://sub.exact.com", false},      // Exact match required
		{"https://google.com", false},         // Not in list
		{"invalid-url", false},                // Invalid format
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.allowed, IsDomainAllowed(tt.url, rules))
		})
	}
}
