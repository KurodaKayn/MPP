package services

import (
	"errors"
	"strings"
	"testing"
)

func TestWechatConnectionFailureMessageDoesNotLeakSecret(t *testing.T) {
	result := buildWechatConnectionResult(errors.New(`Get "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=wx-app&secret=wx-secret": dial tcp: i/o timeout`))

	if strings.Contains(result.Message, "wx-secret") || strings.Contains(result.Message, "secret=") {
		t.Fatalf("connection failure message leaked request details: %q", result.Message)
	}
}

func TestUserFacingErrorMessageRedactsSensitiveWechatParams(t *testing.T) {
	message := sanitizeUserFacingErrorMessage(`failed to create draft: Get "https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=wx-app&secret=wx-secret": failed with access_token=token-value`)

	if strings.Contains(message, "wx-secret") || strings.Contains(message, "token-value") {
		t.Fatalf("user-facing error leaked credential material: %q", message)
	}
	if !strings.Contains(message, "secret=<redacted>") || !strings.Contains(message, "access_token=<redacted>") {
		t.Fatalf("user-facing error did not mark redacted parameters: %q", message)
	}
}
