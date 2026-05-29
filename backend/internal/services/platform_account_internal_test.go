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
