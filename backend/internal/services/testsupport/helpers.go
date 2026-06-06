package testsupport

import (
	"context"
	"fmt"
	"net/url"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
)

type FakeXOAuth2Provider struct {
	AuthConfig       pkgx.OAuth2Config
	AuthState        string
	AuthChallenge    string
	ExchangeCode     string
	ExchangeVerifier string
	RefreshConfig    pkgx.OAuth2Config
	RefreshToken     string
	Token            pkgx.OAuth2Token
	User             pkgx.User
}

type FakeProjectDraftCompiler struct {
	Err           error
	BeforeReturn  func()
	LastProject   *models.Project
	LastPlatforms []string
}

type FakeProjectDocumentInitializer struct {
	Err                          error
	SyncErr                      error
	DocumentIDs                  []uuid.UUID
	SourceContentDocumentIDs     []uuid.UUID
	SyncProjectSourceContentFunc func(context.Context, uuid.UUID) error
}

func (f *FakeProjectDocumentInitializer) InitializeProjectDocument(_ context.Context, documentID uuid.UUID) error {
	f.DocumentIDs = append(f.DocumentIDs, documentID)
	return f.Err
}

func (f *FakeProjectDocumentInitializer) SyncProjectSourceContent(ctx context.Context, documentID uuid.UUID) error {
	f.SourceContentDocumentIDs = append(f.SourceContentDocumentIDs, documentID)
	if f.SyncProjectSourceContentFunc != nil {
		return f.SyncProjectSourceContentFunc(ctx, documentID)
	}
	return f.SyncErr
}

func (f *FakeProjectDraftCompiler) CompileProjectDrafts(_ context.Context, project *models.Project, _ []models.ProjectPlatformPublication, platforms []string) (map[string][]byte, error) {
	f.LastProject = project
	f.LastPlatforms = append([]string(nil), platforms...)
	if f.Err != nil {
		return nil, f.Err
	}

	drafts := make(map[string][]byte, len(platforms))
	for _, platform := range platforms {
		switch platform {
		case "wechat":
			drafts[platform] = fmt.Appendf(nil, `{"format":"html","html":%q}`, project.SourceContent)
		case "zhihu":
			drafts[platform] = []byte(`{"format":"markdown","markdown":"## Heading\n\nHello **draft**"}`)
		case "x":
			drafts[platform] = fmt.Appendf(nil, `{"format":"text","text":%q}`, project.Title+"\n\nHello draft")
		case "douyin":
			drafts[platform] = []byte(`{"format":"text","text":"Hello draft"}`)
		default:
			drafts[platform] = []byte(`{"format":"text","text":"Hello draft"}`)
		}
	}
	if f.BeforeReturn != nil {
		f.BeforeReturn()
	}
	return drafts, nil
}

func (f *FakeXOAuth2Provider) AuthorizationURL(config pkgx.OAuth2Config, state, codeChallenge string) (string, error) {
	f.AuthConfig = config
	f.AuthState = state
	f.AuthChallenge = codeChallenge

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

func (f *FakeXOAuth2Provider) Exchange(_ context.Context, _ pkgx.OAuth2Config, code, codeVerifier string) (pkgx.OAuth2Token, error) {
	f.ExchangeCode = code
	f.ExchangeVerifier = codeVerifier
	return f.Token, nil
}

func (f *FakeXOAuth2Provider) Refresh(_ context.Context, config pkgx.OAuth2Config, refreshToken string) (pkgx.OAuth2Token, error) {
	f.RefreshConfig = config
	f.RefreshToken = refreshToken
	return f.Token, nil
}

func (f *FakeXOAuth2Provider) Me(_ context.Context, _ string) (pkgx.User, error) {
	return f.User, nil
}

func SetupTestDB() *gorm.DB {
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

	db.Exec(`CREATE TABLE workspace_invites (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		email TEXT NOT NULL,
		role TEXT NOT NULL,
		invited_by TEXT NOT NULL,
		accepted_by TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		token_hash TEXT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		accepted_at DATETIME,
		revoked_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	)`)

	db.Exec(`CREATE TABLE workspace_activities (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		actor_user_id TEXT NOT NULL,
		target_user_id TEXT,
		event_type TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
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

	db.Exec(`CREATE TABLE collab_document_states (
		document_id TEXT PRIMARY KEY,
		y_doc_state BLOB NOT NULL,
		state_vector BLOB,
		compacted_until_seq INTEGER NOT NULL DEFAULT 0,
		state_size_bytes INTEGER NOT NULL DEFAULT 0,
		updated_at DATETIME NOT NULL
	)`)

	db.Exec(`CREATE TABLE collab_document_update_batches (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		document_id TEXT NOT NULL,
		from_seq INTEGER NOT NULL,
		to_seq INTEGER NOT NULL,
		update_payload BLOB NOT NULL,
		update_count INTEGER NOT NULL,
		payload_size_bytes INTEGER NOT NULL,
		actor_user_id TEXT,
		created_at DATETIME NOT NULL
	)`)

	db.Exec(`CREATE TABLE project_collaborators (
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT NOT NULL,
		created_by TEXT NOT NULL,
		created_at DATETIME,
		PRIMARY KEY (project_id, user_id)
	)`)

	db.Exec(`CREATE TABLE project_activities (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		actor_user_id TEXT NOT NULL,
		target_user_id TEXT,
		event_type TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL
	)`)

	db.Exec(`CREATE TABLE project_comments (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		author_id TEXT NOT NULL,
		body TEXT NOT NULL,
		anchor_text TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'open',
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL,
		resolved_at DATETIME
	)`)

	db.Exec(`CREATE TABLE project_versions (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		created_by TEXT NOT NULL,
		version_number INTEGER NOT NULL,
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		collab_document_id TEXT,
		collab_seq INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL,
		created_at DATETIME NOT NULL
	)`)

	db.Exec(`CREATE TABLE project_share_links (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		created_by TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		role TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		expires_at DATETIME,
		created_at DATETIME NOT NULL,
		revoked_at DATETIME
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

type FakeWechatTester struct {
	Result dto.WechatConnectionTestResponse
	AppID  string
	Secret string
}

func (f *FakeWechatTester) Test(appID, appSecret string) dto.WechatConnectionTestResponse {
	f.AppID = appID
	f.Secret = appSecret
	return f.Result
}

type FakePlatformPublisher struct {
	Config         datatypes.JSON
	AdaptedContent datatypes.JSON
	AccountCookies datatypes.JSON
	RemoteURL      string
}

func (f *FakePlatformPublisher) ValidateConfig(_ []byte) error {
	return nil
}

func (f *FakePlatformPublisher) Publish(ctx context.Context, pub *models.ProjectPlatformPublication, account *models.PlatformAccount) (string, string, error) {
	f.Config = append(datatypes.JSON(nil), pub.Config...)
	f.AdaptedContent = append(datatypes.JSON(nil), pub.AdaptedContent...)
	if account != nil {
		f.AccountCookies = append(datatypes.JSON(nil), account.Cookies...)
	}
	if remoteURL, ok := ctx.Value(publisher.ContextKeyRemoteURL).(string); ok {
		f.RemoteURL = remoteURL
	}
	return "remote-id", "https://example.com/published", nil
}
