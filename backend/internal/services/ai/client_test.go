package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kurodakayn/mpp-backend/internal/dto"
)

func TestAIServiceClientEditContentPostsToAIService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/content/edit", r.URL.Path)

		var req dto.AIEditContentRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "<p>Draft</p>", req.Content)
		assert.Equal(t, "Make it sharper", req.Message)

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
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Empty(t, req.Content)
		assert.Equal(t, "Write a hello world example", req.Message)

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
		assert.Equal(t, "/prepublish/edit", r.URL.Path)

		var req dto.AIEditPrepublishRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "wechat", req.Platform)
		assert.Equal(t, "Make it concise", req.Message)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"prepublish","platform":"wechat","adapted_content":{"format":"html","html":"<p>Concise</p>"},"content":"<p>Concise</p>"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	resp, err := client.EditPrepublish(t.Context(), dto.AIEditPrepublishRequest{
		Platform: "wechat",
		Message:  "Make it concise",
		AdaptedContent: map[string]any{
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
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/content/edit/stream", r.URL.Path)

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
	defer func() { _ = stream.Body.Close() }()

	body, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, "text/markdown; charset=utf-8", stream.ContentType)
	require.Equal(t, "first second", string(body))
}

func TestAIServiceClientSendsInternalBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer ai-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"content","content":"edited"}`))
	}))
	defer server.Close()

	client := NewAIServiceClientWithToken(server.URL, server.Client(), "ai-token")
	resp, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})

	require.NoError(t, err)
	require.Equal(t, "edited", resp.Content)
}

func TestAIServiceClientFromEnvSendsInternalBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer env-ai-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"channel":"content","content":"edited"}`))
	}))
	defer server.Close()
	t.Setenv(aiServiceURLEnv, server.URL)
	t.Setenv(aiServiceInternalTokenEnv, "env-ai-token")

	client := NewAIServiceClientFromEnv()
	resp, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})

	require.NoError(t, err)
	require.Equal(t, "edited", resp.Content)
}

func TestAIServiceClientMapsBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	require.ErrorIs(t, err, ErrInvalidAIEditRequest)
	require.Contains(t, err.Error(), "message is required")
}

func TestAIServiceClientRedactsUpstreamFailureDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":"provider key sk-test-secret failed at internal.host"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	_, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrAIServiceUnavailable)
	require.Contains(t, err.Error(), aiServiceUnavailableMessage)
	require.NotContains(t, err.Error(), "sk-test-secret")
	require.NotContains(t, err.Error(), "internal.host")
}

func TestAIServiceClientRedactsAuthenticationFailureDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"wrong token used against ai.internal"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	_, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: "Edit",
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrAIServiceUnavailable)
	require.Contains(t, err.Error(), aiServiceAuthenticationError)
	require.NotContains(t, err.Error(), "ai.internal")
}

func TestAIServiceClientRejectsInvalidContentEditMessage(t *testing.T) {
	client := NewAIServiceClient("http://example.invalid", nil)
	_, err := client.EditContent(t.Context(), dto.AIEditContentRequest{
		Content: "Draft",
		Message: " ",
	})

	require.ErrorIs(t, err, ErrInvalidAIEditRequest)
}

func TestAIServiceClientMapsGrowthOptimizationBadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/growth/optimize/stream", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"source_content and goal are required"}`))
	}))
	defer server.Close()

	client := NewAIServiceClient(server.URL, server.Client())
	_, err := client.StreamGrowthOptimization(t.Context(), dto.CreateAIGrowthOptimizationRunRequest{
		Goal:            "improve platform fit",
		SourceContent:   "draft",
		TargetPlatforms: []string{"wechat"},
	})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidGrowthOptimizationRequest)
	require.Contains(t, err.Error(), "source_content and goal are required")
}
