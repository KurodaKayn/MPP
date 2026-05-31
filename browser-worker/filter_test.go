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
		{"http://douyin.com", false},           // Wrong scheme
		{"https://evil-douyin.com", false},     // Not a subdomain (prefix match attempt)
		{"https://douyin.com.evil.com", false}, // Wrong suffix
		{"https://sub.exact.com", false},       // Exact match required
		{"https://google.com", false},          // Not in list
		{"invalid-url", false},                 // Invalid format
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.allowed, IsDomainAllowed(tt.url, rules))
		})
	}
}

func TestEndpointPort(t *testing.T) {
	port, err := endpointPort("ws://localhost:49152")
	assert.NoError(t, err)
	assert.Equal(t, 49152, port)

	_, err = endpointPort("ws://localhost")
	assert.Error(t, err)
}

func TestValidateRequiredCookies(t *testing.T) {
	requirements := []CookieRequirement{
		{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true},
		{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true},
		{Name: "optional", DomainSuffixes: []string{".douyin.com"}, Required: false},
	}

	ok, missing := validateRequiredCookies([]Cookie{
		{Name: "sessionid", Value: "session", Domain: ".douyin.com"},
		{Name: "sid_guard", Value: "guard", Domain: "creator.douyin.com"},
	}, requirements)
	assert.True(t, ok)
	assert.Empty(t, missing)

	ok, missing = validateRequiredCookies([]Cookie{
		{Name: "sessionid", Value: "session", Domain: "douyin.com.evil.test"},
	}, requirements)
	assert.False(t, ok)
	assert.ElementsMatch(t, []string{"sessionid", "sid_guard"}, missing)
}
