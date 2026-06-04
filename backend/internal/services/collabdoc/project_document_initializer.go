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

var ErrProjectDocumentInitialization = errors.New("project collaborative document initialization failed")

const projectDocumentInitializerTimeout = 10 * time.Second

type ProjectDocumentInitializer interface {
	InitializeProjectDocument(ctx context.Context, documentID uuid.UUID) error
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
	if i == nil || i.baseURL == "" || i.tokenSecret == "" || documentID == uuid.Nil {
		return ErrProjectDocumentInitialization
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		i.baseURL+"/internal/collab/documents/"+documentID.String()+"/project-state",
		nil,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProjectDocumentInitialization, err)
	}
	req.Header.Set("Authorization", "Bearer "+i.tokenSecret)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProjectDocumentInitialization, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("%w: returned status %d", ErrProjectDocumentInitialization, resp.StatusCode)
	}
	return nil
}
