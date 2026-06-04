package dashboard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgx "github.com/kurodakayn/mpp-backend/internal/pkg/x"
	"github.com/kurodakayn/mpp-backend/internal/publisher"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
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
	lastProject   *models.Project
	lastPlatforms []string
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
		title TEXT NOT NULL,
		source_content TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at DATETIME,
		updated_at DATETIME
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

func TestGetStats(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test1"}
	u2 := models.User{Username: "test2"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c", Status: models.ProjectStatusDraft}
	p2 := models.Project{UserID: u2.ID, Title: "p2", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p1)
	db.Create(&p2)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished})
	db.Create(&models.ProjectPlatformPublication{ProjectID: p2.ID, Platform: "zhihu", Status: models.PublicationStatusFailed})

	// Test Admin scope (nil scopeUserID)
	stats, err := s.GetStats(nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), stats.TotalUsers)
	assert.Equal(t, int64(2), stats.TotalProjects)
	assert.Equal(t, int64(1), stats.TotalPublishedPublications)
	assert.Equal(t, int64(1), stats.TotalFailedPublications)

	// Test Personal scope (u1)
	statsScoped, errScoped := s.GetStats(&u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(1), statsScoped.TotalUsers)
	assert.Equal(t, int64(1), statsScoped.TotalProjects)
	assert.Equal(t, int64(1), statsScoped.TotalPublishedPublications)
	assert.Equal(t, int64(0), statsScoped.TotalFailedPublications)
}

func TestGetExtensionSessionReturnsCurrentUser(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "creator", Email: "creator@example.com"}
	require.NoError(t, db.Create(&user).Error)

	resp, err := s.GetExtensionSession(user.ID)

	require.NoError(t, err)
	assert.True(t, resp.Authenticated)
	assert.Equal(t, user.ID, resp.User.ID)
	assert.Equal(t, "creator", resp.User.Username)
}

func TestGetExtensionSessionReturnsNotFoundForMissingUser(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	_, err := s.GetExtensionSession(uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestListExtensionPrepublishReturnsCurrentUserDouyinDrafts(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	owner := models.User{Username: "owner", Email: "owner@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&stranger).Error)

	olderUpdatedAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	newerUpdatedAt := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	unsupportedUpdatedAt := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	otherUpdatedAt := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	olderProject := models.Project{
		UserID:        owner.ID,
		Title:         "Older Douyin",
		SourceContent: "older body",
		Status:        models.ProjectStatusReady,
		UpdatedAt:     olderUpdatedAt,
	}
	newerProject := models.Project{
		UserID:        owner.ID,
		Title:         "Newer Douyin",
		SourceContent: "newer body",
		Status:        models.ProjectStatusDraft,
		UpdatedAt:     newerUpdatedAt,
	}
	unsupportedProject := models.Project{
		UserID:        owner.ID,
		Title:         "Zhihu only",
		SourceContent: "zhihu body",
		Status:        models.ProjectStatusReady,
		UpdatedAt:     unsupportedUpdatedAt,
	}
	otherProject := models.Project{
		UserID:        stranger.ID,
		Title:         "Other Douyin",
		SourceContent: "other body",
		Status:        models.ProjectStatusReady,
		UpdatedAt:     otherUpdatedAt,
	}
	require.NoError(t, db.Create(&olderProject).Error)
	require.NoError(t, db.Create(&newerProject).Error)
	require.NoError(t, db.Create(&unsupportedProject).Error)
	require.NoError(t, db.Create(&otherProject).Error)
	require.NoError(t, db.Model(&olderProject).UpdateColumn("updated_at", olderUpdatedAt).Error)
	require.NoError(t, db.Model(&newerProject).UpdateColumn("updated_at", newerUpdatedAt).Error)
	require.NoError(t, db.Model(&unsupportedProject).UpdateColumn("updated_at", unsupportedUpdatedAt).Error)
	require.NoError(t, db.Model(&otherProject).UpdateColumn("updated_at", otherUpdatedAt).Error)

	longText := strings.Repeat("a", 90)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      olderProject.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"` + longText + `"}`),
	}).Error)
	disabledPublication := models.ProjectPlatformPublication{
		ProjectID:      newerProject.ID,
		Platform:       "douyin",
		Enabled:        false,
		Status:         models.PublicationStatusDisabled,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"disabled draft"}`),
	}
	require.NoError(t, db.Create(&disabledPublication).Error)
	require.NoError(t, db.Model(&disabledPublication).UpdateColumn("enabled", false).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      unsupportedProject.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"markdown":"zhihu draft"}`),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      otherProject.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"text":"other draft"}`),
	}).Error)

	resp, err := s.ListExtensionPrepublish(owner.ID)

	require.NoError(t, err)
	require.Len(t, resp.Items, 2)
	assert.Equal(t, newerProject.ID, resp.Items[0].ProjectID)
	assert.False(t, resp.Items[0].Platforms[0].Enabled)
	assert.Equal(t, "DYNAMIC_DOUYIN", resp.Items[0].Platforms[0].AdapterKey)
	assert.Equal(t, "article", resp.Items[0].Platforms[0].ContentKind)
	assert.Equal(t, olderProject.ID, resp.Items[1].ProjectID)
	assert.Equal(t, strings.Repeat("a", 80), resp.Items[1].Platforms[0].Preview)
}

func TestCreateExtensionHandoffReturnsDouyinArticleHandoff(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin article",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"schema_version":1,"format":"text","text":"ready text"}`),
	}
	require.NoError(t, db.Create(&publication).Error)

	before := time.Now().UTC()
	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	require.NoError(t, err)
	assert.Equal(t, 1, handoff.SchemaVersion)
	assert.Equal(t, "mpp.extension_publish_handoff", handoff.Type)
	assert.NotEmpty(t, handoff.ExecutionID)
	assert.True(t, handoff.ExpiresAt.After(before))
	assert.Equal(t, project.ID, handoff.Project.ID)
	assert.Equal(t, "Douyin article", handoff.Project.Title)
	require.Len(t, handoff.Platforms, 1)
	platform := handoff.Platforms[0]
	assert.Equal(t, "douyin", platform.Platform)
	assert.Equal(t, "DYNAMIC_DOUYIN", platform.AdapterKey)
	assert.Equal(t, "https://creator.douyin.com/creator-micro/content/upload?default-tab=5", platform.InjectURL)
	assert.Equal(t, "article", platform.ContentKind)
	assert.False(t, platform.AutoPublish)
	assert.True(t, platform.RequiresReview)
	assert.Empty(t, platform.Assets)
	assert.Equal(t, "https://mpp.example.com/api/user/dashboard/extension/events", platform.Callback.URL)
	assert.NotEmpty(t, platform.Callback.Token)
	assert.Equal(t, 1, platform.AdaptedContent["schema_version"])
	assert.Equal(t, "text", platform.AdaptedContent["format"])
	assert.Equal(t, "ready text", platform.AdaptedContent["text"])

	var token models.ExtensionCallbackToken
	require.NoError(t, db.First(&token, "token = ?", platform.Callback.Token).Error)
	assert.Equal(t, handoff.ExecutionID, token.ExecutionID)
	assert.Equal(t, project.ID, token.ProjectID)
	assert.Equal(t, user.ID, token.UserID)
	assert.Equal(t, "douyin", token.Platform)
	assert.WithinDuration(t, handoff.ExpiresAt, token.ExpiresAt, time.Second)
}

func TestRecordExtensionEventAcceptsKnownTokenAndDeduplicatesEventID(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin article",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)

	req := dto.ExtensionEventCallbackRequest{
		Token:        handoff.Platforms[0].Callback.Token,
		EventID:      "event-1",
		Platform:     "douyin",
		Status:       "user_review",
		Message:      "Draft prepared",
		RemoteID:     "remote-1",
		PublishURL:   "https://creator.douyin.com/item/1",
		ErrorMessage: "",
		Metadata: map[string]interface{}{
			"adapter": "DYNAMIC_DOUYIN",
		},
	}
	first, err := s.RecordExtensionEvent(req)
	require.NoError(t, err)
	assert.False(t, first.Duplicate)

	second, err := s.RecordExtensionEvent(req)
	require.NoError(t, err)
	assert.True(t, second.Duplicate)

	var events []models.ExtensionExecutionEvent
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, handoff.ExecutionID, events[0].ExecutionID)
	assert.Equal(t, project.ID, events[0].ProjectID)
	assert.Equal(t, user.ID, events[0].UserID)
	assert.Equal(t, "event-1", events[0].EventID)
	assert.Equal(t, "user_review", events[0].Status)
	assert.Contains(t, string(events[0].Metadata), "DYNAMIC_DOUYIN")
}

func TestRecordExtensionEventRejectsUnknownToken(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	_, err := s.RecordExtensionEvent(dto.ExtensionEventCallbackRequest{
		Token:    "missing-token",
		EventID:  "event-1",
		Platform: "douyin",
		Status:   "failed",
	})

	assert.ErrorIs(t, err, services.ErrExtensionCallbackTokenInvalid)
}

func TestRecordExtensionEventRejectsExpiredToken(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Douyin article",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}).Error)
	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.ExtensionCallbackToken{}).
		Where("token = ?", handoff.Platforms[0].Callback.Token).
		Update("expires_at", time.Now().Add(-time.Minute).UTC()).Error)

	_, err = s.RecordExtensionEvent(dto.ExtensionEventCallbackRequest{
		Token:    handoff.Platforms[0].Callback.Token,
		EventID:  "event-1",
		Platform: "douyin",
		Status:   "failed",
	})

	assert.ErrorIs(t, err, services.ErrExtensionCallbackTokenExpired)
}

func TestCreateExtensionHandoffRejectsForeignProject(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	owner := models.User{Username: "owner", Email: "owner@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&stranger).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Not yours",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	_, err := s.CreateExtensionHandoff(stranger.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	assert.ErrorIs(t, err, services.ErrForbidden)
}

func TestCreateExtensionHandoffRejectsDisabledPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Disabled Douyin",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	publication := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        false,
		Status:         models.PublicationStatusDisabled,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}
	require.NoError(t, db.Create(&publication).Error)
	require.NoError(t, db.Model(&publication).UpdateColumn("enabled", false).Error)

	_, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	assert.ErrorIs(t, err, services.ErrPublicationDisabled)
}

func TestCreateExtensionHandoffRejectsMissingAdaptedText(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Pending Douyin",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		AdaptedContent: datatypes.JSON(`{}`),
	}).Error)

	_, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	assert.ErrorIs(t, err, services.ErrPublicationRequiresSync)
}

func TestListProjects(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "test"}
	u2 := models.User{Username: "other"}
	db.Create(&u1)
	db.Create(&u2)

	p1 := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished, CreatedAt: time.Now().Add(-1 * time.Hour)}
	p2 := models.Project{UserID: u1.ID, Title: "p2", SourceContent: "c2", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	p3 := models.Project{UserID: u2.ID, Title: "p3", SourceContent: "c3", Status: models.ProjectStatusDraft, CreatedAt: time.Now()}
	db.Create(&p1)
	db.Create(&p2)
	db.Create(&p3)

	db.Create(&models.ProjectPlatformPublication{ProjectID: p1.ID, Platform: "wechat", Status: models.PublicationStatusPublished, PublishURL: "url1"})

	// Test global admin pagination
	res, err := s.ListProjects(1, 10, "", "", "", nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), res.Total)

	// Test Personal scope (u1)
	resScoped, errScoped := s.ListProjects(1, 10, "", "", "", &u1.ID)
	assert.NoError(t, errScoped)
	assert.Equal(t, int64(2), resScoped.Total)
	items := resScoped.Items.([]dto.ProjectListItem)
	assert.Equal(t, 2, len(items))
	// Ensure p3 is not in list
	for _, item := range items {
		assert.NotEqual(t, p3.ID, item.ID)
	}
}

func TestListProjectsIncludesCollaboratorProjectsWithRoles(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	collaborator := models.User{Username: "collaborator", Email: "collab@example.com"}
	hiddenOwner := models.User{Username: "hidden-owner", Email: "hidden@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)
	require.NoError(t, db.Create(&hiddenOwner).Error)

	ownedProject := models.Project{UserID: collaborator.ID, Title: "Owned", SourceContent: "owned", Status: models.ProjectStatusDraft, CreatedAt: time.Now().Add(2 * time.Hour)}
	sharedProject := models.Project{UserID: owner.ID, Title: "Shared", SourceContent: "shared", Status: models.ProjectStatusReady, CreatedAt: time.Now().Add(time.Hour)}
	hiddenProject := models.Project{UserID: hiddenOwner.ID, Title: "Hidden", SourceContent: "hidden", Status: models.ProjectStatusReady, CreatedAt: time.Now().Add(3 * time.Hour)}
	require.NoError(t, db.Create(&ownedProject).Error)
	require.NoError(t, db.Create(&sharedProject).Error)
	require.NoError(t, db.Create(&hiddenProject).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: sharedProject.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	res, err := s.ListProjects(1, 10, "", "", "", &collaborator.ID)
	require.NoError(t, err)
	require.Equal(t, int64(2), res.Total)

	items := res.Items.([]dto.ProjectListItem)
	require.Len(t, items, 2)
	roles := map[uuid.UUID]string{}
	for _, item := range items {
		roles[item.ID] = item.Role
		require.NotEqual(t, hiddenProject.ID, item.ID)
	}
	require.Equal(t, models.ProjectRoleOwner, roles[ownedProject.ID])
	require.Equal(t, models.ProjectRoleViewer, roles[sharedProject.ID])
}

func TestCreateProjectCreatesSelectedPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "WeChat title",
		SourceContent: "<p>Hello WeChat</p>",
		Summary:       "Hello WeChat",
		CoverImageURL: "data:image/png;base64,aGVsbG8=",
		Platforms:     []string{"wechat", "wechat", "douyin"},
	})

	assert.NoError(t, err)
	assert.Equal(t, "WeChat title", resp.Title)
	assert.Equal(t, models.ProjectStatusReady, resp.Status)
	assert.Equal(t, models.ProjectRoleOwner, resp.Role)
	assert.Len(t, resp.Publications, 2)

	var project models.Project
	assert.NoError(t, db.First(&project, "id = ?", resp.ID).Error)
	assert.Equal(t, user.ID, project.UserID)
	assert.Equal(t, "<p>Hello WeChat</p>", project.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", resp.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusPending, wechatPub.Status)

	var config map[string]string
	assert.NoError(t, json.Unmarshal(wechatPub.Config, &config))
	assert.Equal(t, "WeChat title", config["title"])
	assert.Equal(t, "Hello WeChat", config["digest"])
	assert.Equal(t, "data:image/png;base64,aGVsbG8=", config["cover_image_url"])

	var adapted map[string]string
	assert.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &adapted))
	assert.Empty(t, adapted)

	var douyinPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", resp.ID, "douyin").Error)
	assert.Equal(t, models.PublicationStatusPending, douyinPub.Status)
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Missing platform",
		SourceContent: "content",
	})
	assert.ErrorIs(t, err, services.ErrInvalidProject)

	_, err = s.CreateProject(user.ID, dto.CreateProjectRequest{
		Title:         "Unknown platform",
		SourceContent: "content",
		Platforms:     []string{"threads"},
	})
	assert.ErrorIs(t, err, services.ErrInvalidProject)
}

func TestGetProjectReturnsSourceContentForOwner(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	db.Create(&owner)
	db.Create(&editor)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Existing post",
		SourceContent: "<p>Editable body</p>",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	})
	db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	})

	resp, err := s.GetProject(project.ID, &owner.ID)
	assert.NoError(t, err)
	assert.Equal(t, project.ID, resp.ID)
	assert.Equal(t, models.ProjectRoleOwner, resp.Role)
	assert.Equal(t, "<p>Editable body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 1)

	collaboratorResp, err := s.GetProject(project.ID, &editor.ID)
	assert.NoError(t, err)
	assert.Equal(t, project.ID, collaboratorResp.ID)
	assert.Equal(t, models.ProjectRoleEditor, collaboratorResp.Role)
	assert.Equal(t, "<p>Editable body</p>", collaboratorResp.SourceContent)

	_, err = s.GetProject(project.ID, &stranger.ID)
	assert.ErrorIs(t, err, services.ErrForbidden)
}

func TestUpdateProjectRebuildsSelectedPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	db.Create(&owner)
	db.Create(&stranger)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusPublished,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusPublished,
		PublishURL:   "https://example.com/old",
		RemoteID:     "old-remote",
		PublishedAt:  ptrTime(time.Now()),
		RetryCount:   2,
		ErrorMessage: "old error",
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "zhihu",
		Enabled:      true,
		Status:       models.PublicationStatusFailed,
		ErrorMessage: "failed before",
	})

	resp, err := s.UpdateProject(project.ID, owner.ID, dto.UpdateProjectRequest{
		Title:         "New title",
		SourceContent: "<p>New body</p>",
		Summary:       "New body",
		Platforms:     []string{"zhihu", "douyin"},
	})

	assert.NoError(t, err)
	assert.Equal(t, "New title", resp.Title)
	assert.Equal(t, "<p>New body</p>", resp.SourceContent)
	assert.Len(t, resp.Publications, 3)

	var saved models.Project
	assert.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	assert.Equal(t, "New title", saved.Title)
	assert.Equal(t, "<p>New body</p>", saved.SourceContent)
	assert.Equal(t, models.ProjectStatusReady, saved.Status)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.False(t, wechatPub.Enabled)
	assert.Equal(t, models.PublicationStatusDisabled, wechatPub.Status)

	var zhihuPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.True(t, zhihuPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, zhihuPub.Status)
	assert.Empty(t, zhihuPub.ErrorMessage)
	assert.Empty(t, zhihuPub.PublishURL)
	assert.Nil(t, zhihuPub.PublishedAt)

	var douyinPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", project.ID, "douyin").Error)
	assert.True(t, douyinPub.Enabled)
	assert.Equal(t, models.PublicationStatusPending, douyinPub.Status)

	_, err = s.UpdateProject(project.ID, stranger.ID, dto.UpdateProjectRequest{
		Title:         "Not allowed",
		SourceContent: "content",
		Platforms:     []string{"wechat"},
	})
	assert.ErrorIs(t, err, services.ErrForbidden)
}

func TestUpdateProjectAllowsEditorAndRejectsViewer(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPublished,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.UpdateProject(project.ID, editor.ID, dto.UpdateProjectRequest{
		Title:         "Editor title",
		SourceContent: "editor body",
		Platforms:     []string{"zhihu"},
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)
	require.Equal(t, "Editor title", updated.Title)
	require.Equal(t, "editor body", updated.SourceContent)

	var wechatPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	require.False(t, wechatPub.Enabled)
	require.Equal(t, models.PublicationStatusDisabled, wechatPub.Status)

	var zhihuPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.True(t, zhihuPub.Enabled)
	require.Equal(t, models.PublicationStatusPending, zhihuPub.Status)

	_, err = s.UpdateProject(project.ID, viewer.ID, dto.UpdateProjectRequest{
		Title:         "Viewer title",
		SourceContent: "viewer body",
		Platforms:     []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestSaveProjectContentAllowsEditorAndRejectsViewer(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Old title",
		SourceContent: "old body",
		Status:        models.ProjectStatusDraft,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.SaveProjectContent(project.ID, editor.ID, dto.SaveProjectContentRequest{
		Title:         "Editor title",
		SourceContent: "editor body",
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)
	require.Equal(t, "Editor title", updated.Title)
	require.Equal(t, "editor body", updated.SourceContent)

	_, err = s.SaveProjectContent(project.ID, viewer.ID, dto.SaveProjectContentRequest{
		Title:         "Viewer title",
		SourceContent: "viewer body",
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	var saved models.Project
	require.NoError(t, db.First(&saved, "id = ?", project.ID).Error)
	require.Equal(t, "Editor title", saved.Title)
	require.Equal(t, "editor body", saved.SourceContent)
}

func TestSaveProjectPlatformsAllowsEditorAndRejectsViewer(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	editor := models.User{Username: "editor", Email: "editor@example.com"}
	viewer := models.User{Username: "viewer", Email: "viewer@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&editor).Error)
	require.NoError(t, db.Create(&viewer).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Draft title",
		SourceContent: "draft body",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusAdapted,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    editor.ID,
		Role:      models.ProjectRoleEditor,
		CreatedBy: owner.ID,
	}).Error)
	require.NoError(t, db.Create(&models.ProjectCollaborator{
		ProjectID: project.ID,
		UserID:    viewer.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: owner.ID,
	}).Error)

	updated, err := s.SaveProjectPlatforms(project.ID, editor.ID, dto.SaveProjectPlatformsRequest{
		Platforms: []string{"wechat", "zhihu"},
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleEditor, updated.Role)

	var zhihuPub models.ProjectPlatformPublication
	require.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.True(t, zhihuPub.Enabled)
	require.Equal(t, models.PublicationStatusPending, zhihuPub.Status)

	_, err = s.SaveProjectPlatforms(project.ID, viewer.ID, dto.SaveProjectPlatformsRequest{
		Platforms: []string{"wechat"},
	})
	require.ErrorIs(t, err, services.ErrForbidden)
}

func TestProjectCollaboratorManagement(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	owner := models.User{Username: "owner", Email: "owner@example.com"}
	collaborator := models.User{Username: "collaborator", Email: "collaborator@example.com"}
	stranger := models.User{Username: "stranger", Email: "stranger@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&collaborator).Error)
	require.NoError(t, db.Create(&stranger).Error)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Shared project",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	added, err := s.AddProjectCollaborator(project.ID, owner.ID, dto.AddProjectCollaboratorRequest{
		Email: "COLLABORATOR@example.com",
		Role:  models.ProjectRoleEditor,
	})
	require.NoError(t, err)
	require.Equal(t, collaborator.ID, added.UserID)
	require.Equal(t, collaborator.Email, added.Email)
	require.Equal(t, models.ProjectRoleEditor, added.Role)
	require.Equal(t, owner.ID, added.CreatedBy)

	list, err := s.ListProjectCollaborators(project.ID, owner.ID)
	require.NoError(t, err)
	require.Len(t, list.Items, 1)
	require.Equal(t, collaborator.ID, list.Items[0].UserID)

	updated, err := s.UpdateProjectCollaborator(project.ID, owner.ID, collaborator.ID, dto.UpdateProjectCollaboratorRequest{
		Role: models.ProjectRoleViewer,
	})
	require.NoError(t, err)
	require.Equal(t, models.ProjectRoleViewer, updated.Role)

	_, err = s.AddProjectCollaborator(project.ID, owner.ID, dto.AddProjectCollaboratorRequest{
		UserID: owner.ID,
		Role:   models.ProjectRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrInvalidProjectCollaborator)

	_, err = s.AddProjectCollaborator(project.ID, stranger.ID, dto.AddProjectCollaboratorRequest{
		UserID: collaborator.ID,
		Role:   models.ProjectRoleViewer,
	})
	require.ErrorIs(t, err, services.ErrForbidden)

	require.NoError(t, s.RemoveProjectCollaborator(project.ID, owner.ID, collaborator.ID))
	list, err = s.ListProjectCollaborators(project.ID, owner.ID)
	require.NoError(t, err)
	require.Empty(t, list.Items)
}

func TestSyncProjectPrepublishGeneratesPlatformDrafts(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	compiler := &fakeProjectDraftCompiler{}
	s.SetDraftCompiler(compiler)

	owner := models.User{Username: "owner"}
	db.Create(&owner)

	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`,
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "wechat",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "zhihu",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "x",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: project.ID,
		Platform:  "douyin",
		Enabled:   true,
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"title":"Platform title"}`),
	})

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"wechat", "zhihu", "x", "douyin"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	assert.NoError(t, err)
	assert.Equal(t, project.ID, resp.ProjectID)
	assert.Len(t, resp.Items, 4)
	assert.Equal(t, []string{"wechat", "zhihu", "x", "douyin"}, compiler.lastPlatforms)

	var wechatPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&wechatPub, "project_id = ? AND platform = ?", project.ID, "wechat").Error)
	assert.Equal(t, models.PublicationStatusAdapted, wechatPub.Status)

	var wechatContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(wechatPub.AdaptedContent, &wechatContent))
	assert.Equal(t, "html", wechatContent["format"])
	assert.Equal(t, `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`, wechatContent["html"])

	var zhihuPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&zhihuPub, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	assert.Equal(t, models.PublicationStatusAdapted, zhihuPub.Status)

	var zhihuContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(zhihuPub.AdaptedContent, &zhihuContent))
	assert.Equal(t, "markdown", zhihuContent["format"])
	assert.Contains(t, zhihuContent["markdown"], "## Heading")
	assert.Contains(t, zhihuContent["markdown"], "**draft**")

	var xPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&xPub, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, models.PublicationStatusAdapted, xPub.Status)

	var xContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(xPub.AdaptedContent, &xContent))
	assert.Equal(t, "text", xContent["format"])
	assert.Contains(t, xContent["text"], "Platform title")
	assert.Contains(t, xContent["text"], "Hello draft")

	var douyinPub models.ProjectPlatformPublication
	assert.NoError(t, db.First(&douyinPub, "project_id = ? AND platform = ?", project.ID, "douyin").Error)
	assert.Equal(t, models.PublicationStatusAdapted, douyinPub.Status)

	var douyinContent map[string]interface{}
	assert.NoError(t, json.Unmarshal(douyinPub.AdaptedContent, &douyinContent))
	assert.Equal(t, "text", douyinContent["format"])
	assert.Contains(t, douyinContent["text"], "Hello draft")
}

func TestSyncProjectPrepublishMarksFailedWhenContentPipelineCompilerFails(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	s.SetDraftCompiler(&fakeProjectDraftCompiler{err: fmt.Errorf("content pipeline unavailable")})

	owner := models.User{Username: "owner"}
	require.NoError(t, db.Create(&owner).Error)
	project := models.Project{
		UserID:        owner.ID,
		Title:         "Platform title",
		SourceContent: `<h2>Heading</h2><p>Hello <strong>draft</strong></p>`,
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "zhihu",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Platform title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}).Error)

	resp, err := s.SyncProjectPrepublish(project.ID, owner.ID, dto.SyncPrepublishRequest{
		Platforms: []string{"zhihu"},
		Actor:     dto.SyncActor{Type: "system"},
	})

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, models.PublicationStatusFailed, resp.Items[0].Status)
	require.Contains(t, resp.Items[0].ErrorMessage, "content pipeline unavailable")

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "zhihu").Error)
	require.Equal(t, models.PublicationStatusFailed, publication.Status)
	require.JSONEq(t, `{}`, string(publication.AdaptedContent))
}

func TestGetProjectPublications(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u1 := models.User{Username: "owner"}
	collaborator := models.User{Username: "collaborator", Email: "collaborator@example.com"}
	u2 := models.User{Username: "stranger"}
	db.Create(&u1)
	db.Create(&collaborator)
	db.Create(&u2)

	p := models.Project{UserID: u1.ID, Title: "p1", SourceContent: "c1", Status: models.ProjectStatusPublished}
	db.Create(&p)

	configJSON := `{"title": "safe_title", "secret_token": "hidden"}`
	contentJSON := `{"summary": "safe_summary", "full_text": "huge..."}`

	pub := models.ProjectPlatformPublication{
		ProjectID:      p.ID,
		Platform:       "wechat",
		Status:         models.PublicationStatusPublished,
		Config:         datatypes.JSON(configJSON),
		AdaptedContent: datatypes.JSON(contentJSON),
	}
	db.Create(&pub)
	db.Create(&models.ProjectCollaborator{
		ProjectID: p.ID,
		UserID:    collaborator.ID,
		Role:      models.ProjectRoleViewer,
		CreatedBy: u1.ID,
	})

	// Admin can see it
	res, err := s.GetProjectPublications(p.ID, nil, false)
	assert.NoError(t, err)
	assert.Equal(t, p.ID, res.ProjectID)

	// Owner can see it
	resOwner, errOwner := s.GetProjectPublications(p.ID, &u1.ID, false)
	assert.NoError(t, errOwner)
	assert.Equal(t, p.ID, resOwner.ProjectID)

	// Collaborator can see it
	resCollaborator, errCollaborator := s.GetProjectPublications(p.ID, &collaborator.ID, false)
	assert.NoError(t, errCollaborator)
	assert.Equal(t, p.ID, resCollaborator.ProjectID)

	// Stranger gets Forbidden
	_, errStranger := s.GetProjectPublications(p.ID, &u2.ID, false)
	assert.ErrorIs(t, errStranger, services.ErrForbidden)
}

func TestBatchPublishProject(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	u := models.User{Username: "tester"}
	db.Create(&u)

	p := models.Project{UserID: u.ID, Title: "p", SourceContent: "c", Status: models.ProjectStatusDraft}
	db.Create(&p)

	// Create publications for multiple platforms
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: p.ID,
		Platform:  "wechat",
		Status:    models.PublicationStatusPending,
		Config:    datatypes.JSON(`{"app_id": "test", "app_secret": "test"}`),
	})
	db.Create(&models.ProjectPlatformPublication{
		ProjectID: p.ID,
		Platform:  "zhihu",
		Status:    models.PublicationStatusPending,
	})

	// Test batch publish
	platforms := []string{"wechat", "zhihu"}
	results, err := s.BatchPublishProject(p.ID, platforms, &u.ID)

	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))

	// Check results
	for _, platform := range platforms {
		assert.Contains(t, results, platform)
	}
}

func TestWechatAccountSettingsSaveMasksSecret(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	resp, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	assert.NoError(t, err)
	assert.Equal(t, "wechat", resp.Platform)
	assert.Equal(t, "wx-app", resp.AppID)
	assert.True(t, resp.HasAppSecret)
	assert.Equal(t, models.PlatformAccountStatusUntested, resp.Status)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "wx-app", saved.AppID)
	assert.True(t, saved.HasAppSecret)
}

func TestWechatAccountTestUsesSavedSecretAndUpdatesStatus(t *testing.T) {
	db := setupTestDB()
	tester := &fakeWechatTester{
		result: dto.WechatConnectionTestResponse{
			Connected: true,
			Status:    models.PlatformAccountStatusConnected,
			Message:   "ok",
			TestedAt:  time.Now(),
		},
	}
	s := services.NewDashboardServiceWithWechatTester(db, tester)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-app",
		AppSecret: "wx-secret",
	})
	assert.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID: "wx-app",
	})
	assert.NoError(t, err)
	assert.True(t, result.Connected)
	assert.Equal(t, "wx-app", tester.appID)
	assert.Equal(t, "wx-secret", tester.secret)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusConnected, saved.Status)
	assert.Empty(t, saved.LastTestError)
}

func TestWechatAccountTestDoesNotPersistUnsavedCredentialsStatus(t *testing.T) {
	db := setupTestDB()
	testedAt := time.Now()
	tester := &fakeWechatTester{
		result: dto.WechatConnectionTestResponse{
			Connected: false,
			Status:    models.PlatformAccountStatusFailed,
			Message:   "failed",
			TestedAt:  testedAt,
		},
	}
	s := services.NewDashboardServiceWithWechatTester(db, tester)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-saved",
		AppSecret: "saved-secret",
	})
	assert.NoError(t, err)

	result, err := s.TestWechatAccount(user.ID, dto.TestWechatAccountRequest{
		AppID:     "wx-draft",
		AppSecret: "draft-secret",
	})
	assert.NoError(t, err)
	assert.False(t, result.Connected)
	assert.Equal(t, "wx-draft", tester.appID)
	assert.Equal(t, "draft-secret", tester.secret)

	saved, err := s.GetWechatAccount(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)
	assert.Nil(t, saved.LastTestedAt)
	assert.Empty(t, saved.LastTestError)
}

func TestXAccountSettingsClearsUsernameAndMetadataWhenCredentialsChange(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)

	_, err := s.UpsertXAccount(user.ID, dto.UpsertXAccountRequest{
		APIKey:            "x-old-key",
		APISecret:         "x-old-secret",
		AccessToken:       "x-old-token",
		AccessTokenSecret: "x-old-token-secret",
		Username:          "oldhandle",
	})
	assert.NoError(t, err)

	var account models.PlatformAccount
	assert.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	assert.NoError(t, db.Model(&account).Update("metadata", datatypes.JSON(`{"username":"oldmeta"}`)).Error)

	_, err = s.UpsertXAccount(user.ID, dto.UpsertXAccountRequest{
		APIKey:            "x-new-key",
		APISecret:         "x-new-secret",
		AccessToken:       "x-new-token",
		AccessTokenSecret: "x-new-token-secret",
	})
	assert.NoError(t, err)

	saved, err := s.GetXAccount(user.ID)
	assert.NoError(t, err)
	assert.Empty(t, saved.Username)
	assert.Equal(t, models.PlatformAccountStatusUntested, saved.Status)

	assert.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var credentials map[string]string
	assert.NoError(t, json.Unmarshal(account.Credentials, &credentials))
	assert.Equal(t, "x-new-key", credentials["api_key"])
	assert.Empty(t, credentials["username"])

	var metadata map[string]string
	assert.NoError(t, json.Unmarshal(account.Metadata, &metadata))
	assert.Empty(t, metadata["username"])
}

func TestXOAuth2FlowStoresConnectedAccount(t *testing.T) {
	t.Setenv("X_OAUTH2_CLIENT_ID", "client-id")
	t.Setenv("X_OAUTH2_CLIENT_SECRET", "client-secret")

	db := setupTestDB()
	expiresAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	provider := &fakeXOAuth2Provider{
		token: pkgx.OAuth2Token{
			AccessToken:  "oauth2-access",
			RefreshToken: "oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    expiresAt,
		},
		user: pkgx.User{
			ID:       "x-user-id",
			Name:     "Creator",
			Username: "creator",
		},
	}
	s := services.NewDashboardServiceWithXOAuth2Provider(db, provider)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	authURL, err := s.StartXOAuth2(user.ID, "https://app.example.com/api/user/dashboard/settings/x/oauth2/callback")
	require.NoError(t, err)
	require.NotEmpty(t, provider.authState)
	require.NotEmpty(t, provider.authChallenge)
	assert.Equal(t, "client-id", provider.authConfig.ClientID)
	assert.Equal(t, "client-secret", provider.authConfig.ClientSecret)

	parsedAuthURL, err := url.Parse(authURL)
	require.NoError(t, err)
	state := parsedAuthURL.Query().Get("state")
	require.NotEmpty(t, state)

	resp, err := s.CompleteXOAuth2(context.Background(), state, "auth-code")
	require.NoError(t, err)

	assert.Equal(t, "auth-code", provider.exchangeCode)
	assert.NotEmpty(t, provider.exchangeVerifier)
	assert.Equal(t, "oauth2", resp.AuthType)
	assert.Equal(t, "creator", resp.Username)
	assert.True(t, resp.HasOAuth2Refresh)
	assert.Equal(t, models.PlatformAccountStatusConnected, resp.Status)
	require.NotNil(t, resp.ExpiresAt)
	assert.Equal(t, expiresAt, *resp.ExpiresAt)

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)

	var credentials map[string]string
	require.NoError(t, json.Unmarshal(account.Credentials, &credentials))
	assert.Equal(t, "oauth2", credentials["auth_type"])
	assert.Equal(t, "oauth2-access", credentials["oauth2_access_token"])
	assert.Equal(t, "oauth2-refresh", credentials["oauth2_refresh_token"])

	var metadata map[string]string
	require.NoError(t, json.Unmarshal(account.Metadata, &metadata))
	assert.Equal(t, "creator", metadata["username"])
	assert.Equal(t, "x-user-id", metadata["user_id"])
}

func TestPublishProjectUsesSavedWechatCredentials(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	db.Create(&user)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"app_id":"stale","app_secret":"stale-secret","title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	db.Create(&pub)
	_, err := s.UpsertWechatAccount(user.ID, dto.UpsertWechatAccountRequest{
		AppID:     "wx-saved",
		AppSecret: "saved-secret",
	})
	assert.NoError(t, err)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)
	assert.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]string
	assert.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "wx-saved", config["app_id"])
	assert.Equal(t, "saved-secret", config["app_secret"])
	assert.Equal(t, "Title", config["title"])
}

func TestPublishProjectPassesDecryptedBrowserCookiesToPublisher(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("douyin", fakePublisher)
	defer publisher.Factory.Register("douyin", &publisher.DouyinPublisher{})
	t.Setenv("COOKIE_ENCRYPTION_KEY", "12345678901234567890123456789012")

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	cookies := []publisher.Cookie{
		{Name: "sessionid", Value: "secret-value", Domain: ".douyin.com", Path: "/", Secure: true},
		{Name: "sid_guard", Value: "guard-value", Domain: ".douyin.com", Path: "/", Secure: true},
		{Name: "passport_csrf_token", Value: "csrf-value", Domain: ".douyin.com", Path: "/", Secure: true},
	}
	require.NoError(t, publisher.NewCookieStore(db).Save(context.Background(), user.ID, "douyin", cookies, publisher.RemoteAccountProfile{
		Username: "creator",
	}))

	result, err := s.PublishProject(project.ID, "douyin", &user.ID, uuid.Nil)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Contains(t, string(fakePublisher.accountCookies), "secret-value")
	assert.NotContains(t, string(fakePublisher.accountCookies), "ciphertext")

	var passedCookies []publisher.Cookie
	require.NoError(t, json.Unmarshal(fakePublisher.accountCookies, &passedCookies))
	assert.Equal(t, cookies, passedCookies)
}

func TestPublishProjectIgnoresBrowserSessionIDForAsyncPublishing(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)
	sessionID := uuid.New()
	require.NoError(t, db.Create(&models.RemoteBrowserSession{
		ID:        sessionID,
		UserID:    user.ID,
		Platform:  "wechat",
		Status:    models.BrowserSessionStatusReady,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(15 * time.Minute).UTC(),
	}).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, sessionID)

	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Empty(t, fakePublisher.remoteURL)
}

func TestPublishProjectRequiresSavedCookiesForBrowserCookiePlatforms(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("douyin", fakePublisher)
	defer publisher.Factory.Register("douyin", &publisher.DouyinPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	_, err := s.PublishProject(project.ID, "douyin", &user.ID, uuid.Nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, services.ErrInvalidPlatformAccount)
	assert.Empty(t, fakePublisher.accountCookies)
}

func TestGetDouyinAccount(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)

	empty, err := s.GetDouyinAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "douyin", empty.Platform)
	assert.Equal(t, "unconfigured", empty.Status)

	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:    user.ID,
		Platform:  "douyin",
		Username:  "creator",
		AvatarURL: "https://example.com/avatar.png",
		Status:    models.PlatformAccountStatusConnected,
	}).Error)

	account, err := s.GetDouyinAccount(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "creator", account.Username)
	assert.Equal(t, "https://example.com/avatar.png", account.AvatarURL)
	assert.Equal(t, models.PlatformAccountStatusConnected, account.Status)
}

func TestPublishProjectRequiresPrepublishSyncForPendingPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("wechat", fakePublisher)
	defer publisher.Factory.Register("wechat", &publisher.WechatPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "<p>source</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)

	require.ErrorIs(t, err, services.ErrPublicationRequiresSync)
	require.Nil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusPending, saved.Status)
	assert.Empty(t, saved.ErrorMessage)
}

func TestPublishProjectUsesSavedXOAuth2Credentials(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("x", fakePublisher)
	defer publisher.Factory.Register("x", &publisher.XPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"api_key":"stale","api_secret":"stale","access_token":"stale","access_token_secret":"stale","title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)
	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:   user.ID,
		Platform: "x",
		Username: "X",
		Status:   models.PlatformAccountStatusConnected,
		Credentials: datatypes.JSON(`{
			"auth_type":"oauth2",
			"oauth2_access_token":"oauth2-access",
			"oauth2_refresh_token":"oauth2-refresh",
			"username":"creator"
		}`),
		Metadata: datatypes.JSON(`{"username":"creator"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "x", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "oauth2", config["auth_type"])
	assert.Equal(t, "oauth2-access", config["oauth2_access_token"])
	assert.Equal(t, "oauth2-refresh", config["oauth2_refresh_token"])
	assert.Equal(t, "creator", config["username"])
	assert.NotContains(t, config, "api_key")
	assert.NotContains(t, config, "api_secret")
	assert.NotContains(t, config, "access_token")
	assert.NotContains(t, config, "access_token_secret")
	assert.Equal(t, "Title", config["title"])
}

func TestPublishProjectRefreshesExpiredXOAuth2Token(t *testing.T) {
	t.Setenv("X_OAUTH2_CLIENT_ID", "client-id")
	t.Setenv("X_OAUTH2_CLIENT_SECRET", "client-secret")

	db := setupTestDB()
	refreshedExpiresAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	provider := &fakeXOAuth2Provider{
		token: pkgx.OAuth2Token{
			AccessToken:  "new-oauth2-access",
			RefreshToken: "new-oauth2-refresh",
			Scope:        "tweet.read tweet.write users.read offline.access",
			ExpiresAt:    refreshedExpiresAt,
		},
	}
	s := services.NewDashboardServiceWithXOAuth2Provider(db, provider)
	fakePublisher := &fakePlatformPublisher{}
	publisher.Factory.Register("x", fakePublisher)
	defer publisher.Factory.Register("x", &publisher.XPublisher{})

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	expiredAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	credentials, err := json.Marshal(map[string]interface{}{
		"auth_type":            "oauth2",
		"oauth2_access_token":  "old-oauth2-access",
		"oauth2_refresh_token": "oauth2-refresh",
		"oauth2_expires_at":    expiredAt,
		"username":             "creator",
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.PlatformAccount{
		UserID:      user.ID,
		Platform:    "x",
		Username:    "X",
		Status:      models.PlatformAccountStatusConnected,
		Credentials: datatypes.JSON(credentials),
		Metadata:    datatypes.JSON(`{"username":"creator"}`),
	}).Error)

	result, err := s.PublishProject(project.ID, "x", &user.ID, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, models.PublicationStatusPublished, result["status"])
	assert.Equal(t, "oauth2-refresh", provider.refreshToken)
	assert.Equal(t, "client-id", provider.refreshConfig.ClientID)
	assert.Equal(t, "client-secret", provider.refreshConfig.ClientSecret)
	assert.Empty(t, provider.refreshConfig.RedirectURI)

	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(fakePublisher.config, &config))
	assert.Equal(t, "oauth2", config["auth_type"])
	assert.Equal(t, "new-oauth2-access", config["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", config["oauth2_refresh_token"])
	assert.Equal(t, "creator", config["username"])

	var account models.PlatformAccount
	require.NoError(t, db.First(&account, "user_id = ? AND platform = ?", user.ID, "x").Error)
	var savedCredentials map[string]interface{}
	require.NoError(t, json.Unmarshal(account.Credentials, &savedCredentials))
	assert.Equal(t, "new-oauth2-access", savedCredentials["oauth2_access_token"])
	assert.Equal(t, "new-oauth2-refresh", savedCredentials["oauth2_refresh_token"])
	assert.Equal(t, "tweet.read tweet.write users.read offline.access", savedCredentials["oauth2_scope"])
}

func TestCreateXPostIntentReturnsManualPublishURL(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"text":"hello x & \u4e2d\u6587"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &user.ID)
	require.NoError(t, err)
	assert.Equal(t, "manual_required", result["status"])
	assert.Equal(t, "x", result["platform"])

	publishURL, ok := result["publish_url"].(string)
	require.True(t, ok)
	parsed, err := url.Parse(publishURL)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "x.com", parsed.Host)
	assert.Equal(t, "/intent/tweet", parsed.Path)
	assert.Equal(t, "hello x & \u4e2d\u6587", parsed.Query().Get("text"))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusAdapted, saved.Status)
	assert.Equal(t, publishURL, saved.PublishURL)
	assert.Empty(t, saved.ErrorMessage)
}

func TestCreateXPostIntentRequiresPrepublishSyncForPendingPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "pending x",
		SourceContent: "<p>source content</p>",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusPending,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	result, err := s.CreateXPostIntent(project.ID, &user.ID)

	require.ErrorIs(t, err, services.ErrPublicationRequiresSync)
	require.Nil(t, result)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	assert.Equal(t, models.PublicationStatusPending, saved.Status)
	assert.JSONEq(t, `{}`, string(saved.AdaptedContent))
}

func TestPublishProjectRejectsDisabledPublication(t *testing.T) {
	db := setupTestDB()
	s := services.NewDashboardService(db)

	user := models.User{Username: "owner"}
	db.Create(&user)
	project := models.Project{
		UserID:        user.ID,
		Title:         "p1",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	db.Create(&project)
	db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        false,
		Status:         models.PublicationStatusDisabled,
		Config:         datatypes.JSON(`{"title":"Title"}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	})

	_, err := s.PublishProject(project.ID, "wechat", &user.ID, uuid.Nil)
	assert.ErrorIs(t, err, services.ErrPublicationDisabled)
}
