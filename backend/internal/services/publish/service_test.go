package publish

import (
	"strings"
	"testing"
)

func TestUserFacingErrorMessageRedactsSensitiveWechatParams(t *testing.T) {
	message := SanitizeUserFacingErrorMessage(`failed to create draft: Get "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=wx-app&secret=wx-secret": failed with access_token=token-value`)

	if strings.Contains(message, "wx-secret") || strings.Contains(message, "token-value") {
		t.Fatalf("user-facing error leaked credential material: %q", message)
	}
	if !strings.Contains(message, "secret=<redacted>") || !strings.Contains(message, "access_token=<redacted>") {
		t.Fatalf("user-facing error did not mark redacted parameters: %q", message)
	}
}
