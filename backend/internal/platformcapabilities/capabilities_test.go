package platformcapabilities

import "testing"

func TestProjectPlatformSetComesFromGeneratedCapabilities(t *testing.T) {
	set := ProjectPlatformSet()

	for _, platform := range []string{"wechat", "zhihu", "x", "douyin"} {
		if _, ok := set[platform]; !ok {
			t.Fatalf("expected %s to be project selectable", platform)
		}
	}
	for _, platform := range []string{"xiaohongshu", "bilibili"} {
		if _, ok := set[platform]; ok {
			t.Fatalf("expected %s to stay extension-only", platform)
		}
	}
}

func TestExtensionHandoffCapabilitiesComeFromGeneratedCapabilities(t *testing.T) {
	douyin, ok := ExtensionHandoffConfigFor("douyin")
	if !ok {
		t.Fatal("expected douyin handoff config")
	}
	if douyin.AdapterKey != "DYNAMIC_DOUYIN" || douyin.ContentKind != "article" {
		t.Fatalf("unexpected douyin handoff config: %+v", douyin)
	}

	x, ok := ExtensionHandoffConfigFor("x")
	if !ok {
		t.Fatal("expected x handoff config")
	}
	if x.AdapterKey != "POST_X" || x.ContentKind != "dynamic_post" {
		t.Fatalf("unexpected x handoff config: %+v", x)
	}

	if _, ok := ExtensionHandoffConfigFor("zhihu"); ok {
		t.Fatal("expected zhihu extension capability without backend handoff")
	}
}

func TestStoredBrowserCookiePlatformsComeFromGeneratedCapabilities(t *testing.T) {
	for _, platform := range []string{"douyin", "zhihu"} {
		if !UsesStoredBrowserCookies(platform) {
			t.Fatalf("expected %s to use stored browser cookies", platform)
		}
	}
	for _, platform := range []string{"wechat", "x"} {
		if UsesStoredBrowserCookies(platform) {
			t.Fatalf("expected %s to avoid stored browser cookies", platform)
		}
	}
}
