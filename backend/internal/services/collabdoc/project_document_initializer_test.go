package collabdoc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	collabdoc "github.com/kurodakayn/mpp-backend/internal/services/collabdoc"
	"github.com/stretchr/testify/require"
)

func TestHTTPProjectDocumentInitializerRequestsProjectState(t *testing.T) {
	documentID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/internal/collab/documents/"+documentID.String()+"/project-state", r.URL.Path)
		require.Equal(t, "Bearer collab-secret", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	initializer := collabdoc.NewHTTPProjectDocumentInitializer(server.URL, []byte("collab-secret"), server.Client())

	require.NoError(t, initializer.InitializeProjectDocument(context.Background(), documentID))
}

func TestHTTPProjectDocumentInitializerRequestsProjectSourceContentSync(t *testing.T) {
	documentID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/internal/collab/documents/"+documentID.String()+"/project-source-content", r.URL.Path)
		require.Equal(t, "Bearer collab-secret", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	initializer := collabdoc.NewHTTPProjectDocumentInitializer(server.URL, []byte("collab-secret"), server.Client())

	require.NoError(t, initializer.SyncProjectSourceContent(context.Background(), documentID))
}

func TestHTTPProjectDocumentInitializerRejectsFailedStateInitialization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	initializer := collabdoc.NewHTTPProjectDocumentInitializer(server.URL, []byte("collab-secret"), server.Client())

	err := initializer.InitializeProjectDocument(context.Background(), uuid.New())

	require.ErrorIs(t, err, collabdoc.ErrProjectDocumentInitialization)
}

func TestHTTPProjectDocumentInitializerRejectsFailedSourceContentSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	initializer := collabdoc.NewHTTPProjectDocumentInitializer(server.URL, []byte("collab-secret"), server.Client())

	err := initializer.SyncProjectSourceContent(context.Background(), uuid.New())

	require.ErrorIs(t, err, collabdoc.ErrProjectSourceContentSync)
}

func TestHTTPProjectDocumentInitializerRejectsInvalidConfiguration(t *testing.T) {
	initializer := collabdoc.NewHTTPProjectDocumentInitializer("", nil, nil)

	err := initializer.InitializeProjectDocument(context.Background(), uuid.New())

	require.ErrorIs(t, err, collabdoc.ErrProjectDocumentInitialization)

	err = initializer.SyncProjectSourceContent(context.Background(), uuid.New())

	require.ErrorIs(t, err, collabdoc.ErrProjectSourceContentSync)
}
