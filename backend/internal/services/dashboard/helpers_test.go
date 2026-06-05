package dashboard_test

import (
	"context"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"net/url"
	"time"
)

type fakeXOAuth2Provider struct {
	authConfig       pkgx.OAuth2Config
	authState        string
	authChallenge    string
	exchangeCode     string
	exchangeVerifier string
	refreshConfig    pkgx.OAuth2Config
	refreshToken     string
	token            pkgx.OAuth2Token
	user             pkgx.User
}

type fakeProjectDraftCompiler struct {
	err           error
	beforeReturn  func()
	lastProject   *models.Project
	lastPlatforms []string
}

type fakeProjectDocumentInitializer struct {
	err         error
	documentIDs []uuid.UUID
}

func (f *fakeProjectDocumentInitializer) InitializeProjectDocument(ctx context.Context, documentID uuid.UUID) error {
	f.documentIDs = append(f.documentIDs, documentID)
	return f.err
}

func (f *fakeProjectDraftCompiler) CompileProjectDrafts(ctx context.Context, project *models.Project, publications []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	f.lastProject = project
	f.lastPlatforms = append([]string(nil), platforms...)
	if f.err != nil {
		return nil, f.err
	}

	drafts := make(map[string][]byte, len(platforms))
	for _, platform := range platforms {
		switch platform {
		case "wechat":
			drafts[platform] = []byte(fmt.Sprintf(`{"format":"html","html":%q}`, project.SourceContent))
		case "zhihu":
			drafts[platform] = []byte(`{"format":"markdown","markdown":"## Heading\n\nHello **draft**"}`)
		case "x":
			drafts[platform] = []byte(fmt.Sprintf(`{"format":"text","text":%q}`, project.Title+"\n\nHello draft"))
		case "douyin":
			drafts[platform] = []byte(`{"format":"text","text":"Hello draft"}`)
		default:
			drafts[platform] = []byte(`{"format":"text","text":"Hello draft"}`)
		}
	}
	if f.beforeReturn != nil {
		f.beforeReturn()
	}
	return drafts, nil
}

func (f *fakeXOAuth2Provider) AuthorizationURL(config pkgx.OAuth2Config, state, codeChallenge string) (string, error) {
	f.authConfig = config
	f.authState = state
	f.authChallenge = codeChallenge

	endpoint := url.URL{
		Scheme: "https",
		Host:   "x.example.com",
		Path:   "/i/oauth2/authorize",
	}
	query := endpoint.Query()
	query.Set("state", state)
	query.Set("code_challenge", codeChallenge)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (f *fakeXOAuth2Provider) Exchange(ctx context.Context, config pkgx.OAuth2Config, code, codeVerifier string) (pkgx.OAuth2Token, error) {
	f.exchangeCode = code
	f.exchangeVerifier = codeVerifier
	return f.token, nil
}

func (f *fakeXOAuth2Provider) Refresh(ctx context.Context, config pkgx.OAuth2Config, refreshToken string) (pkgx.OAuth2Token, error) {
	f.refreshConfig = config
	f.refreshToken = refreshToken
	return f.token, nil
}

func (f *fakeXOAuth2Provider) Me(ctx context.Context, accessToken string) (pkgx.User, error) {
	return f.user, nil
}

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		email TEXT NOT NULL,
		is_email_verified BOOLEAN NOT NULL DEFAULT 0,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE projects (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		workspace_id TEXT,
		collab_document_id TEXT UNIQUE,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY,
		owner_user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		slug TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	)`)

	db.Exec(`CREATE TABLE workspace_members (
		workspace_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		invited_by TEXT,
		joined_at DATETIME,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (workspace_id, user_id)
	)`)

	db.Exec(`CREATE TABLE collab_documents (
		id TEXT PRIMARY KEY,
		owner_user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		schema_version INTEGER NOT NULL DEFAULT 1,
		current_seq INTEGER NOT NULL DEFAULT 0,
		last_edited_by TEXT,
		last_edited_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		deleted_at DATETIME
	)`)

	db.Exec(`CREATE TABLE project_collaborators (
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at DATETIME,
		PRIMARY KEY (project_id, user_id)
	)`)

	db.Exec(`CREATE TABLE platform_accounts (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		username TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'untested',
		credentials TEXT NOT NULL DEFAULT '{}',
		metadata TEXT NOT NULL DEFAULT '{}',
		cookies TEXT NOT NULL DEFAULT '[]',
		config TEXT NOT NULL DEFAULT '{}',
		avatar_url TEXT,
		last_tested_at DATETIME,
		last_test_error TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE project_platform_publications (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		adapted_content TEXT NOT NULL DEFAULT '{}',
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_attempt_at DATETIME,
		published_at DATETIME,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE remote_browser_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		status TEXT NOT NULL,
		worker_session_ref TEXT,
		container_id TEXT,
		cdp_endpoint_ref TEXT,
		stream_endpoint_ref TEXT,
		connect_token_hash TEXT,
		connect_token_expires_at DATETIME,
		error_message TEXT,
		created_at DATETIME,
		expires_at DATETIME,
		completed_at DATETIME,
		metadata TEXT NOT NULL DEFAULT '{}'
	)`)

	db.Exec(`CREATE TABLE extension_callback_tokens (
		id TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	db.Exec(`CREATE TABLE extension_execution_events (
		id TEXT PRIMARY KEY,
		callback_token_id TEXT NOT NULL,
		execution_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		event_id TEXT NOT NULL UNIQUE,
		platform TEXT NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		remote_id TEXT,
		publish_url TEXT,
		error_message TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME
	)`)

	return db
}

type fakeWechatTester struct {
	result dto.WechatConnectionTestResponse
	appID  string
	secret string
}

func (f *fakeWechatTester) Test(appID, appSecret string) dto.WechatConnectionTestResponse {
	f.appID = appID
	f.secret = appSecret
	return f.result
}

type fakePlatformPublisher struct {
	config         datatypes.JSON
	accountCookies datatypes.JSON
	remoteURL      string
}

func (f *fakePlatformPublisher) ValidateConfig(config []byte) error {
	return nil
}

func (f *fakePlatformPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	f.config = append(datatypes.JSON(nil), pub.Config...)
	if account != nil {
		f.accountCookies = append(datatypes.JSON(nil), account.Cookies...)
	}
	if remoteURL, ok := ctx.Value(publisher.ContextKeyRemoteURL).(string); ok {
		f.remoteURL = remoteURL
	}
	return "remote-id", "https://example.com/published", nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
