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

func TestUserFacingErrorMessageRedactsSignedURLParams(t *testing.T) {
	message := SanitizeUserFacingErrorMessage(`failed to download image: Get "https://bucket.example/object.png?X-Amz-Credential=access-key/20260606/auto/s3/aws4_request&X-Amz-Signature=signature-value&X-Amz-Security-Token=session-token&X-Amz-Date=20260606T120000Z": 403 Forbidden`)

	for _, leaked := range []string{"access-key", "signature-value", "session-token"} {
		if strings.Contains(message, leaked) {
			t.Fatalf("user-facing error leaked signed URL material: %q", message)
		}
	}
	for _, redacted := range []string{"X-Amz-Credential=<redacted>", "X-Amz-Signature=<redacted>", "X-Amz-Security-Token=<redacted>"} {
		if !strings.Contains(message, redacted) {
			t.Fatalf("user-facing error did not redact %s: %q", redacted, message)
		}
	}
}
