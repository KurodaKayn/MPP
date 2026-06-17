package wechat

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientGetTokenCreatesAndCachesToken(t *testing.T) {
	requests := make([]*http.Request, 0, 2)
	client := NewClient("app", "secret")
	client.HTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, cloneRequest(req))
		return jsonResponse(`{"access_token":"token-1","expires_in":3600,"errcode":0}`), nil
	})}

	token, err := client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)

	token, err = client.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)
	require.Len(t, requests, 1)
	assert.Contains(t, requests[0].URL.RawQuery, "appid=app")
	assert.Contains(t, requests[0].URL.RawQuery, "secret=secret")
}

func TestClientUploadThumbCreatesMultipartMaterialRequest(t *testing.T) {
	client := clientWithToken("token-1")
	var materialRequest *http.Request
	client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		materialRequest = cloneRequest(req)
		return jsonResponse(`{"media_id":"media-1","url":"https://example.com/media.jpg","errcode":0}`), nil
	})

	response, err := client.UploadThumb([]byte("cover-bytes"), "cover.jpg")

	require.NoError(t, err)
	assert.Equal(t, "media-1", response.MediaID)
	require.NotNil(t, materialRequest)
	assert.Equal(t, http.MethodPost, materialRequest.Method)
	assert.Contains(t, materialRequest.URL.String(), "/material/add_material")
	assert.Equal(t, "token-1", materialRequest.URL.Query().Get("access_token"))
	assert.Equal(t, "thumb", materialRequest.URL.Query().Get("type"))

	body, err := io.ReadAll(materialRequest.Body)
	require.NoError(t, err)
	mediaType := materialRequest.Header.Get("Content-Type")
	_, params, err := mime.ParseMediaType(mediaType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	part, err := reader.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "media", part.FormName())
	assert.Equal(t, "cover.jpg", part.FileName())
	partBytes, err := io.ReadAll(part)
	require.NoError(t, err)
	assert.Equal(t, "cover-bytes", string(partBytes))
}

func TestClientCreateDraftSendsArticlesPayload(t *testing.T) {
	client := clientWithToken("token-1")
	var draftRequest *http.Request
	client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		draftRequest = cloneRequest(req)
		return jsonResponse(`{"media_id":"draft-1","errcode":0}`), nil
	})

	draftID, err := client.CreateDraft([]Article{{Title: "Title", Content: "Body"}})

	require.NoError(t, err)
	assert.Equal(t, "draft-1", draftID)
	require.NotNil(t, draftRequest)
	assert.Contains(t, draftRequest.URL.String(), "/draft/add")
	assert.Equal(t, "token-1", draftRequest.URL.Query().Get("access_token"))
	body, err := io.ReadAll(draftRequest.Body)
	require.NoError(t, err)
	var payload struct {
		Articles []Article `json:"articles"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Len(t, payload.Articles, 1)
	assert.Equal(t, "Title", payload.Articles[0].Title)
	assert.Equal(t, "Body", payload.Articles[0].Content)
}

func TestClientPublishReturnsPublishErrorCodeWithoutTransportError(t *testing.T) {
	client := clientWithToken("token-1")
	var publishRequest *http.Request
	client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		publishRequest = cloneRequest(req)
		return jsonResponse(`{"publish_id":"publish-1","errcode":48001}`), nil
	})

	publishID, errCode, err := client.Publish("draft-1")

	require.NoError(t, err)
	assert.Equal(t, "publish-1", publishID)
	assert.Equal(t, 48001, errCode)
	require.NotNil(t, publishRequest)
	body, err := io.ReadAll(publishRequest.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"media_id":"draft-1"}`, string(body))
}

func TestClientReturnsAPIErrorForTokenAndMaterialFailures(t *testing.T) {
	tokenClient := NewClient("app", "secret")
	tokenClient.HTTPClient = &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(`{"errcode":40013,"errmsg":"invalid appid"}`), nil
	})}
	_, err := tokenClient.GetToken()
	require.Error(t, err)
	var apiErr APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 40013, apiErr.ErrCode)

	materialClient := clientWithToken("token-1")
	materialClient.HTTPClient.Transport = roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(`{"errcode":48001,"errmsg":"unauthorized"}`), nil
	})
	_, err = materialClient.UploadImage([]byte("image"), "image.png")
	require.Error(t, err)
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 48001, apiErr.ErrCode)
}

func TestClientReturnsAPIErrorForDraftFailures(t *testing.T) {
	client := clientWithToken("token-1")
	client.HTTPClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/draft/add") {
			return nil, errors.New("unexpected request")
		}
		return jsonResponse(`{"errcode":40001,"errmsg":"draft failed"}`), nil
	})

	_, err := client.CreateDraft([]Article{{Title: "Title"}})

	require.Error(t, err)
	var apiErr APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 40001, apiErr.ErrCode)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func clientWithToken(token string) *Client {
	return &Client{
		AppID:     "app",
		AppSecret: "secret",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("unexpected request")
		})},
		token:  token,
		expiry: nowPlusHour(),
	}
}

func nowPlusHour() time.Time {
	return time.Now().Add(time.Hour)
}

func cloneRequest(req *http.Request) *http.Request {
	cloned := req.Clone(req.Context())
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		cloned.Body = io.NopCloser(bytes.NewReader(body))
	}
	return cloned
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
