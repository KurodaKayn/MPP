package extension_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	dbrouter "github.com/kurodakayn/mpp-backend/internal/db"
	"github.com/kurodakayn/mpp-backend/internal/dto"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestGetExtensionSessionReturnsCurrentUser(t *testing.T) {
	db := testsupport.SetupTestDB()
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
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)

	_, err := s.GetExtensionSession(uuid.New())

	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestListExtensionPrepublishReturnsCurrentUserDouyinDrafts(t *testing.T) {
	db := testsupport.SetupTestDB()
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

func TestListExtensionPrepublishReturnsXDrafts(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "X post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"x draft"}`),
	}).Error)

	resp, err := s.ListExtensionPrepublish(user.ID)

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, project.ID, resp.Items[0].ProjectID)
	require.Len(t, resp.Items[0].Platforms, 1)
	platform := resp.Items[0].Platforms[0]
	assert.Equal(t, "x", platform.Platform)
	assert.Equal(t, "POST_X", platform.AdapterKey)
	assert.Equal(t, "dynamic_post", platform.ContentKind)
	assert.Equal(t, "x draft", platform.Preview)
}

func TestListExtensionPrepublishUsesReader(t *testing.T) {
	writer := testsupport.SetupTestDB()
	reader := testsupport.SetupTestDB()
	s := services.NewDashboardServiceWithRouter(writer, dbrouter.NewRouter(writer, dbrouter.WithReader(reader)))
	user := models.User{Username: "reader-owner", Email: "reader-owner@example.com"}
	require.NoError(t, reader.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Reader draft",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
		UpdatedAt:     time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, reader.Create(&project).Error)
	require.NoError(t, reader.Model(&project).UpdateColumn("updated_at", project.UpdatedAt).Error)
	require.NoError(t, reader.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"reader draft"}`),
	}).Error)

	resp, err := s.ListExtensionPrepublish(user.ID)

	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	assert.Equal(t, project.ID, resp.Items[0].ProjectID)
	assert.Equal(t, "reader draft", resp.Items[0].Platforms[0].Preview)
}

func TestCreateExtensionHandoffReturnsDouyinArticleHandoff(t *testing.T) {
	db := testsupport.SetupTestDB()
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

func TestCreateExtensionHandoffReturnsXPostHandoff(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "X post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"schema_version":1,"format":"text","text":"x ready text"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"x"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	require.NoError(t, err)
	require.Len(t, handoff.Platforms, 1)
	platform := handoff.Platforms[0]
	assert.Equal(t, "x", platform.Platform)
	assert.Equal(t, "POST_X", platform.AdapterKey)
	assert.Equal(t, "https://x.com/compose/post", platform.InjectURL)
	assert.Equal(t, "dynamic_post", platform.ContentKind)
	assert.False(t, platform.AutoPublish)
	assert.True(t, platform.RequiresReview)
	assert.Equal(t, "text", platform.AdaptedContent["format"])
	assert.Equal(t, "x ready text", platform.AdaptedContent["text"])

	var token models.ExtensionCallbackToken
	require.NoError(t, db.First(&token, "token = ?", platform.Callback.Token).Error)
	assert.Equal(t, handoff.ExecutionID, token.ExecutionID)
	assert.Equal(t, "x", token.Platform)
}

func TestCreateExtensionHandoffReturnsMultiplePlatformHandoff(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Multi platform post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "douyin",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"douyin ready"}`),
	}).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"x ready"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"douyin", "x"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	require.NoError(t, err)
	require.Len(t, handoff.Platforms, 2)
	assert.Equal(t, "douyin", handoff.Platforms[0].Platform)
	assert.Equal(t, "DYNAMIC_DOUYIN", handoff.Platforms[0].AdapterKey)
	assert.Equal(t, "x", handoff.Platforms[1].Platform)
	assert.Equal(t, "POST_X", handoff.Platforms[1].AdapterKey)
	assert.NotEqual(t, handoff.Platforms[0].Callback.Token, handoff.Platforms[1].Callback.Token)

	var tokenCount int64
	require.NoError(t, db.Model(&models.ExtensionCallbackToken{}).
		Where("execution_id = ?", handoff.ExecutionID).
		Count(&tokenCount).Error)
	assert.Equal(t, int64(2), tokenCount)
}

func TestRecordExtensionEventAcceptsKnownTokenAndDeduplicatesEventID(t *testing.T) {
	db := testsupport.SetupTestDB()
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
		Metadata: map[string]any{
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

func TestRecordExtensionEventMarksXPublicationReadyForUserReview(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "X post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		DraftStatus:    models.PublicationDraftStatusReady,
		ReviewStatus:   models.PublicationReviewStatusDraft,
		ErrorMessage:   "old error",
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"x"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)

	resp, err := s.RecordExtensionEvent(dto.ExtensionEventCallbackRequest{
		Token:      handoff.Platforms[0].Callback.Token,
		EventID:    "x-user-review-1",
		Platform:   "x",
		Status:     "user_review",
		Message:    "Draft prepared",
		PublishURL: "https://x.com/compose/post",
	})

	require.NoError(t, err)
	assert.False(t, resp.Duplicate)
	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, models.PublicationStatusAdapted, publication.Status)
	assert.Equal(t, models.PublicationDraftStatusReady, publication.DraftStatus)
	assert.Equal(t, models.PublicationReviewStatusReviewing, publication.ReviewStatus)
	assert.Empty(t, publication.ErrorMessage)
	assert.Equal(t, "https://x.com/compose/post", publication.PublishURL)
}

func TestRecordExtensionEventMarksXPublicationFailedWithMessage(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "X post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		DraftStatus:    models.PublicationDraftStatusReady,
		ReviewStatus:   models.PublicationReviewStatusDraft,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"x"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)

	resp, err := s.RecordExtensionEvent(dto.ExtensionEventCallbackRequest{
		Token:        handoff.Platforms[0].Callback.Token,
		EventID:      "x-failed-1",
		Platform:     "x",
		Status:       "failed",
		Message:      "Could not find composer.",
		ErrorMessage: "Composer missing",
	})

	require.NoError(t, err)
	assert.False(t, resp.Duplicate)
	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, models.PublicationStatusFailed, publication.Status)
	assert.Equal(t, "Composer missing", publication.ErrorMessage)
	assert.Equal(t, 1, publication.RetryCount)
}

func TestRecordExtensionEventDoesNotApplyDuplicatePublicationUpdate(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "X post",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	require.NoError(t, db.Create(&models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "x",
		Enabled:        true,
		Status:         models.PublicationStatusAdapted,
		AdaptedContent: datatypes.JSON(`{"format":"text","text":"ready text"}`),
	}).Error)

	handoff, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"x"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")
	require.NoError(t, err)
	req := dto.ExtensionEventCallbackRequest{
		Token:        handoff.Platforms[0].Callback.Token,
		EventID:      "x-failed-duplicate",
		Platform:     "x",
		Status:       "failed",
		ErrorMessage: "Composer missing",
	}

	first, err := s.RecordExtensionEvent(req)
	require.NoError(t, err)
	assert.False(t, first.Duplicate)
	second, err := s.RecordExtensionEvent(req)
	require.NoError(t, err)
	assert.True(t, second.Duplicate)

	var publication models.ProjectPlatformPublication
	require.NoError(t, db.First(&publication, "project_id = ? AND platform = ?", project.ID, "x").Error)
	assert.Equal(t, 1, publication.RetryCount)
}

func TestRecordExtensionEventRejectsUnknownToken(t *testing.T) {
	db := testsupport.SetupTestDB()
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
	db := testsupport.SetupTestDB()
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
	db := testsupport.SetupTestDB()
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
	db := testsupport.SetupTestDB()
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
	db := testsupport.SetupTestDB()
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

func TestCreateExtensionHandoffRejectsUnsupportedPlatform(t *testing.T) {
	db := testsupport.SetupTestDB()
	s := services.NewDashboardService(db)
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Unsupported platform",
		SourceContent: "source",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)

	_, err := s.CreateExtensionHandoff(user.ID, dto.CreateExtensionHandoffRequest{
		ProjectID: project.ID,
		Platforms: []string{"wechat"},
	}, "https://mpp.example.com/api/user/dashboard/extension/events")

	assert.ErrorIs(t, err, services.ErrInvalidProject)
}
