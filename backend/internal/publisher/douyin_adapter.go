package publisher

import (
	"context"
	"strings"

	"github.com/chromedp/cdproto/network"
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
		
		// Newly discovered domains required for React/UI rendering and security checks
		{Host: "bytetos.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "security glue scripts"},
		{Host: "byted-static.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "react and systemjs bundles"},
		{Host: "zijieapi.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "monitoring and security metrics"},
		{Host: "ibytedapm.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "slardar apm monitoring"},
		{Host: "bytednsdoc.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "static assets and icons"},
		{Host: "volces.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "captcha images"},
		{Host: "bytecdn.cn", Match: "suffix", Schemes: []string{"https"}, Purpose: "cdn assets"},
		{Host: "yhgfb-cn-static.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "user center secure sdk"},
		{Host: "bytetcc.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "tcc config"},
		{Host: "bytedance.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "mssdk and ttwid security tokens"},
	}
}

func (a *DouyinAdapter) RequiredCookies() []CookieRequirement {
	return []CookieRequirement{
		{Name: "sessionid", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		{Name: "sid_guard", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
		{Name: "passport_csrf_token", DomainSuffixes: []string{".douyin.com"}, Required: true, Preserve: true},
	}
}

// DetectLogin checks if the user has successfully logged in by verifying cookies and URL
func (a *DouyinAdapter) DetectLogin(ctx context.Context) (RemoteLoginState, error) {
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return RemoteLoginState{}, err
	}

	// 1. Check if we are on the creator domain
	if !strings.Contains(currentURL, "douyin.com") {
		return RemoteLoginState{LoggedIn: false, CurrentURL: currentURL, Message: "Waiting for platform navigation"}, nil
	}

	// 2. Get all cookies from the browser
	var chromeCookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		chromeCookies, err = network.GetCookies().Do(ctx)
		return err
	})); err != nil {
		return RemoteLoginState{}, err
	}

	// 3. Map to our internal Cookie type and validate
	var cookies []Cookie
	for _, cc := range chromeCookies {
		cookies = append(cookies, Cookie{
			Name:   cc.Name,
			Value:  cc.Value,
			Domain: cc.Domain,
			Path:   cc.Path,
		})
	}

	ok, missing := ValidateDouyinCookies(cookies)
	if !ok {
		return RemoteLoginState{
			LoggedIn:       false,
			CurrentURL:     currentURL,
			MissingCookies: missing,
			Message:        "Waiting for required login cookies",
		}, nil
	}

	return RemoteLoginState{
		LoggedIn:   true,
		Status:     "login_detected",
		CurrentURL: currentURL,
		Message:    "Login detected successfully",
	}, nil
}

// ExtractAccount attempts to get profile info from the page
func (a *DouyinAdapter) ExtractAccount(ctx context.Context) (RemoteAccountProfile, error) {
	var username string
	// Try to extract username from the creator dashboard UI
	script := `(function() {
		const nameEl = document.querySelector('.name-G1vOOn') || 
		               document.querySelector('.user-name') || 
					   document.querySelector('[class*="user-name"]');
		return nameEl ? nameEl.innerText : "";
	})()`

	err := chromedp.Run(ctx, chromedp.Evaluate(script, &username))
	if err != nil {
		return RemoteAccountProfile{}, err
	}

	if username == "" {
		username = "Connected Douyin Account"
	}

	return RemoteAccountProfile{
		Username: username,
	}, nil
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
