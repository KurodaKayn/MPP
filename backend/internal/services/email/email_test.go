package email

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderVerificationCodeEmail(t *testing.T) {
	body, err := renderVerificationCodeEmail(verificationCodeEmailData{
		Title:       "Registering Account",
		Description: "Please enter the following verification code on the page to complete verification",
		Code:        "348862",
		Purpose:     "account registration",
	})

	require.NoError(t, err)
	assert.Contains(t, body, "Registering Account")
	assert.Contains(t, body, "348862")
	assert.Contains(t, body, "valid for 10 minutes")
	assert.Contains(t, body, `src="cid:mpp-logo"`)
	assert.Contains(t, body, "background:#1d1d1f")
	assert.Contains(t, body, "color:#f6d77b")
	assert.NotContains(t, strings.ToLower(body), "<script")
}

func TestBuildHTMLMessageEmbedsLogoPNG(t *testing.T) {
	message, err := buildHTMLMessage(
		"no-reply@example.com",
		"user@example.com",
		"MPP Registration Verification Code",
		`<html><body><img src="cid:mpp-logo" alt="MPP" /></body></html>`,
	)

	require.NoError(t, err)
	assert.Contains(t, message, "Content-Type: multipart/related;")
	assert.Contains(t, message, "Content-Type: text/html; charset=UTF-8")
	assert.Contains(t, message, `Content-Type: image/png; name="mpp-with-name-white.png"`)
	assert.Contains(t, message, "Content-Id: <mpp-logo>")
	assert.Contains(t, message, `Content-Disposition: inline; filename="mpp-with-name-white.png"`)
	assert.Contains(t, message, "Content-Transfer-Encoding: base64")
}
