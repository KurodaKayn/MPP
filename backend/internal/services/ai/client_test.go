package ai

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/stretchr/testify/require"
)

func TestAIServiceClientEditContentPostsToAIService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/content/edit", r.URL.Path)

		var req dto.AIEditContentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "<p>Draft</p>", req.Content)
		require.Equal(t, "Make it sharper", req.Message)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"content","content":"<p>Sharper draft</p>"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	resp, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "<p>Draft</p>",
		Message: "Make it sharper",
	})

	require.NoError(t, err)
	require.Equal(t, "content", resp.Channel)
	require.Equal(t, "<p>Sharper draft</p>", resp.Content)
}

func TestAIServiceClientEditContentAllowsEmptySource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req dto.AIEditContentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Empty(t, req.Content)
		require.Equal(t, "Write a hello world example", req.Message)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"content","content":"print(\"hello world\")"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	resp, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Message: "Write a hello world example",
	})

	require.NoError(t, err)
	require.Equal(t, `print("hello world")`, resp.Content)
}

func TestAIServiceClientEditPrepublishPostsToAIService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/prepublish/edit", r.URL.Path)

		var req dto.AIEditPrepublishRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, "wechat", req.Platform)
		require.Equal(t, "Make it concise", req.Message)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"prepublish","platform":"wechat","adapted_content":{"format":"html","html":"<p>Concise</p>"},"content":"<p>Concise</p>"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	resp, err := client.EditPrepublish(t.Context(), dto.AIEditPrepublishRequest{
		Platform: "wechat",
		Message:  "Make it concise",
		AdaptedContent: map[string]interface{}{
			"format": "html",
			"html":   "<p>Long draft</p>",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "prepublish", resp.Channel)
	require.Equal(t, "wechat", resp.Platform)
	require.Equal(t, "<p>Concise</p>", resp.Content)
	require.Equal(t, "html", resp.AdaptedContent["format"])
}

func TestAIServiceClientStreamsEditedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/content/edit/stream", r.URL.Path)

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte("first "))
		_, _ = w.Write([]byte("second"))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	stream, err := client.StreamEditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})
	require.NoError(t, err)
	defer stream.Body.Close()

	body, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, "text/markdown; charset=utf-8", stream.ContentType)
	require.Equal(t, "first second", string(body))
}

func TestAIServiceClientMapsBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"message is required"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	_, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidAIEditRequest))
	require.Contains(t, err.Error(), "message is required")
}

func TestAIServiceClientRejectsInvalidContentEditMessage(t *testing.T) {
	client := NewAIServiceClient("http://example.invalid", nil)
	_, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: " ",
	})

	require.ErrorIs(t, err, ErrInvalidAIEditRequest)
}
