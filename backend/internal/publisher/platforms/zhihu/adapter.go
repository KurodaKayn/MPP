package zhihu

import (
	"context"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	pubbrowser "github.com/kurodakayn/mpp-backend/internal/publisher/browser"
)

type ZhihuAdapter struct{}

func (a *ZhihuAdapter) Platform() string {
	return "zhihu"
}

func (a *ZhihuAdapter) LoginURL() string {
	return "https://www.zhihu.com/signin"
}

func (a *ZhihuAdapter) AllowedDomains() []pubbrowser.DomainRule {
	return []pubbrowser.DomainRule{
		{Host: "zhihu.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "first-party web and auth"},
		{Host: "zhimg.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "images and static assets"},
		{Host: "zhihuusercontent.com", Match: "suffix", Schemes: []string{"https"}, Purpose: "user content and avatars"},
		// Common analytics and captcha domains for Zhihu
		{Host: "zcdn.net", Match: "suffix", Schemes: []string{"https"}, Purpose: "cdn assets"},
		{Host: "sensorsdata.cn", Match: "suffix", Schemes: []string{"https"}, Purpose: "analytics"},
	}
}

func (a *ZhihuAdapter) RequiredCookies() []pubbrowser.CookieRequirement {
	return []pubbrowser.CookieRequirement{
		{Name: "z_c0", DomainSuffixes: []string{".zhihu.com"}, Required: true, Preserve: true},
		{Name: "q_c1", DomainSuffixes: []string{".zhihu.com"}, Required: false, Preserve: true},
		{Name: "d_c0", DomainSuffixes: []string{".zhihu.com"}, Required: false, Preserve: true},
	}
}

func (a *ZhihuAdapter) DetectLogin(ctx context.Context) (pubbrowser.RemoteLoginState, error) {
	var currentURL string
	if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
		return pubbrowser.RemoteLoginState{}, err
	}

	// 1. Check if we have been redirected away from the signin page, which usually happens after successful login
	if strings.Contains(currentURL, "/signin") {
		return pubbrowser.RemoteLoginState{LoggedIn: false, CurrentURL: currentURL, Message: "Waiting for user to sign in"}, nil
	}

	// 2. Get all cookies from the browser
	var chromeCookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		chromeCookies, err = network.GetCookies().Do(ctx)
		return err
	})); err != nil {
		return pubbrowser.RemoteLoginState{}, err
	}

	// 3. Map to our internal Cookie type and validate
	var cookies []pubbrowser.Cookie
	for _, cc := range chromeCookies {
		cookies = append(cookies, pubbrowser.Cookie{
			Name:   cc.Name,
			Value:  cc.Value,
			Domain: cc.Domain,
			Path:   cc.Path,
		})
	}

	ok, missing := ValidateZhihuCookies(cookies)
	if !ok {
		return pubbrowser.RemoteLoginState{
			LoggedIn:       false,
			CurrentURL:     currentURL,
			MissingCookies: missing,
			Message:        "Waiting for required login cookies",
		}, nil
	}

	return pubbrowser.RemoteLoginState{
		LoggedIn:   true,
		Status:     "login_detected",
		CurrentURL: currentURL,
		Message:    "Login detected successfully",
	}, nil
}

// ExtractAccount attempts to get profile info from the page
func (a *ZhihuAdapter) ExtractAccount(ctx context.Context) (pubbrowser.RemoteAccountProfile, error) {
	var username string
	// Try to extract username from the Zhihu header or profile menu
	script := `(function() {
		const nameEl = document.querySelector('.AppHeader-profileAvatar') ? 
			document.querySelector('.AppHeader-profileAvatar').alt : 
			(document.querySelector('.ProfileHeader-name') || {}).innerText;
		return nameEl || "";
	})()`

	err := chromedp.Run(ctx, chromedp.Evaluate(script, &username))
	if err != nil {
		return pubbrowser.RemoteAccountProfile{}, err
	}

	if username == "" {
		username = "Connected Zhihu Account"
	}

	return pubbrowser.RemoteAccountProfile{
		Username: username,
	}, nil
}

func ValidateZhihuCookies(cookies []pubbrowser.Cookie) (bool, []string) {
	required := map[string]bool{
		"z_c0": false, // This is the main authentication cookie for Zhihu
	}

	for _, c := range cookies {
		if strings.HasSuffix(c.Domain, "zhihu.com") {
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
