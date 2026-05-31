package cookies

import (
	"testing"

	"github.com/kurodakayn/mpp-browser-worker/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestValidateRequired(t *testing.T) {
	requirements := []session.CookieRequirement{
		{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true},
		{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true},
		{Name: "optional", DomainSuffixes: []string{".douyin.com"}, Required: false},
	}

	ok, missing := ValidateRequired([]session.Cookie{
		{Name: "sessionid", Value: "session", Domain: ".douyin.com"},
		{Name: "sid_guard", Value: "guard", Domain: "creator.douyin.com"},
	}, requirements)
	assert.True(t, ok)
	assert.Empty(t, missing)

	ok, missing = ValidateRequired([]session.Cookie{
		{Name: "sessionid", Value: "session", Domain: "douyin.com.evil.test"},
	}, requirements)
	assert.False(t, ok)
	assert.ElementsMatch(t, []string{"sessionid", "sid_guard"}, missing)
}

func TestFilterPreserved(t *testing.T) {
	requirements := []session.CookieRequirement{
		{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
	}

	filtered := FilterPreserved([]session.Cookie{
		{Name: "sessionid", Value: "session", Domain: ".douyin.com"},
		{Name: "sid_guard", Value: "guard", Domain: "creator.douyin.com"},
		{Name: "unrelated", Value: "value", Domain: ".douyin.com"},
		{Name: "sessionid", Value: "evil", Domain: "douyin.com.evil.test"},
	}, requirements)

	assert.Equal(t, []session.Cookie{
		{Name: "sessionid", Value: "session", Domain: ".douyin.com", Path: "/"},
		{Name: "sid_guard", Value: "guard", Domain: "creator.douyin.com", Path: "/"},
	}, filtered)
}
