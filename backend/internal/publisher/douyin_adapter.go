package publisher

import (
	"context"
	"strings"

	"github.com/chromedp/chromedp"
)

type DouyinAdapter struct{}

func (a *DouyinAdapter) Platform() string {
	return "douyin"
}

func (a *DouyinAdapter) LoginURL() string {
	return "https://creator.douyin.com/creator-micro/home"
}

func (a *DouyinAdapter) AllowedDomains() []DomainRule {
	return []DomainRule{
		{Host: "douyin.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "first-party web and auth"},
		{Host: "douyinpic.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "images and avatars"},
		{Host: "douyinstatic.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "static assets"},
		{Host: "douyincdn.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "static assets"},
		{Host: "byteimg.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "static assets"},
		{Host: "bytegoofy.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "frontend bundles"},
		{Host: "snssdk.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "captcha and verification"},
		{Host: "bytedance.net", Match: "suffix", Schemes: []string{"https"}, Purpose: "verification and static dependencies"},
	}
}

func (a *DouyinAdapter) RequiredCookies() []CookieRequirement {
	return []CookieRequirement{
		{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		{Name: "passport_csrf_token", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
	}
}

// DetectLogin and ExtractAccount will be used by the worker or backend via CDP
func (a *DouyinAdapter) DetectLogin(ctx context.Context) (bool, string, []string, error) {
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return false, "", nil, err
	}

	// Check if URL is on douyin.com
	if !strings.Contains(currentURL, "douyin.com") {
		return false, currentURL, nil, nil
	}

	// Check for required cookies
	// network.GetCookies().Do(ctx) is needed here, but we are using worker interface for this usually.
	// For now, let's just implement the logic that validates the cookie list.
	return false, currentURL, nil, nil
}

func ValidateDouyinCookies(cookies []Cookie) (bool, []string) {
	required := map[string]bool{
		"sessionid":           false,
		"sid_guard":           false,
		"passport_csrf_token": false,
	}

	for _, c := range cookies {
		if strings.HasSuffix(c.Domain, "douyin.com") {
			if _, ok := required[c.Name]; ok {
				required[c.Name] = true
			}
		}
	}

	var missing []string
	for name, found := range required {
		if !found {
			missing = append(missing, name)
		}
	}

	return len(missing) == 0, missing
}
