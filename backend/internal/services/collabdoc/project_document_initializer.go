package collabdoc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/pkg/resilience"
)

var (
	ErrProjectDocumentInitialization = errors.New("project collaborative document initialization failed")
	ErrProjectSourceContentSync      = errors.New("project source content sync failed")
)

const projectDocumentInitializerTimeout = 10 * time.Second

type ProjectDocumentInitializer interface {
	InitializeProjectDocument(ctx context.Context, documentID uuid.UUID) error
	SyncProjectSourceContent(ctx context.Context, documentID uuid.UUID) error
}

type HTTPProjectDocumentInitializer struct {
	baseURL     string
	tokenSecret string
	httpClient  *http.Client
}

func NewHTTPProjectDocumentInitializer(baseURL string, tokenSecret []byte, httpClient *http.Client) *HTTPProjectDocumentInitializer {
	if httpClient == nil {
		httpClient = resilience.NewHTTPClient("collab-service", projectDocumentInitializerTimeout)
	}
	return &HTTPProjectDocumentInitializer{
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		tokenSecret: string(tokenSecret),
		httpClient:  httpClient,
	}
}

func (i *HTTPProjectDocumentInitializer) InitializeProjectDocument(ctx context.Context, documentID uuid.UUID) error {
	return i.postInternalProjectDocument(ctx, documentID, "project-state", ErrProjectDocumentInitialization)
}

func (i *HTTPProjectDocumentInitializer) SyncProjectSourceContent(ctx context.Context, documentID uuid.UUID) error {
	return i.postInternalProjectDocument(ctx, documentID, "project-source-content", ErrProjectSourceContentSync)
}

func (i *HTTPProjectDocumentInitializer) postInternalProjectDocument(ctx context.Context, documentID uuid.UUID, path string, sentinel error) error {
	if i == nil || i.baseURL == "" || i.tokenSecret == "" || documentID == uuid.Nil {
		return sentinel
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		i.baseURL+"/internal/collab/documents/"+documentID.String()+"/"+path,
		nil,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", sentinel, err)
	}
	req.Header.Set("Authorization", "Bearer "+i.tokenSecret)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", sentinel, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: returned status %d", sentinel, resp.StatusCode)
	}
	return nil
}
