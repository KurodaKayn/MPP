package isolation

import (
	"testing"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestIsDomainAllowed(t *testing.T) {
	rules := []session.DomainRule{
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
		{"http://douyin.com", false},
		{"https://evil-douyin.com", false},
		{"https://douyin.com.evil.com", false},
		{"https://sub.exact.com", false},
		{"https://google.com", false},
		{"https://127.0.0.1", false},
		{"https://169.254.169.254", false},
		{"https://10.0.0.10", false},
		{"invalid-url", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.allowed, IsDomainAllowed(tt.url, rules))
		})
	}
}
