package publish

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/testsupport"
)

func TestPublicationLifecycleCompletesPublication(t *testing.T) {
	db := testsupport.SetupTestDB()
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Lifecycle",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusPublishing,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)

	lifecycle := NewPublicationLifecycle(db)
	require.NoError(t, lifecycle.CompletePublication(&pub, publicationCompletion{
		Status:     models.PublicationStatusSucceeded,
		RemoteID:   "remote-1",
		PublishURL: "https://example.com/published",
	}))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	require.Equal(t, models.PublicationStatusSucceeded, saved.Status)
	require.Equal(t, "remote-1", saved.RemoteID)
	require.NotNil(t, saved.PublishedAt)
	require.Equal(t, 0, saved.RetryCount)

	require.NoError(t, lifecycle.CompletePublication(&saved, publicationCompletion{
		Status:       models.PublicationStatusFailed,
		ErrorMessage: "platform unavailable",
	}))
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	require.Equal(t, models.PublicationStatusFailed, saved.Status)
	require.Equal(t, "platform unavailable", saved.ErrorMessage)
	require.Equal(t, 1, saved.RetryCount)
}

func TestPublicationLifecycleStartsAndFinishesScheduledAttempt(t *testing.T) {
	db := testsupport.SetupTestDB()
	require.NoError(t, db.AutoMigrate(&models.ScheduledPublication{}, &models.PublishAttempt{}, &models.ProjectVersion{}))
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Scheduled",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:      project.ID,
		Platform:       "wechat",
		Enabled:        true,
		Status:         models.PublicationStatusQueued,
		Config:         datatypes.JSON(`{}`),
		AdaptedContent: datatypes.JSON(`{"summary":"ready"}`),
	}
	require.NoError(t, db.Create(&pub).Error)
	schedule := models.ScheduledPublication{
		WorkspaceID:   models.PersonalWorkspaceID(user.ID),
		ProjectID:     project.ID,
		PublicationID: pub.ID,
		ScheduledAt:   time.Now().UTC(),
		Status:        models.ScheduledPublicationStatusScheduled,
		CreatedBy:     user.ID,
	}
	require.NoError(t, db.Create(&schedule).Error)

	lifecycle := NewPublicationLifecycle(db)
	attempt, ok, err := lifecycle.StartPublishAttempt(schedule.ID, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, attempt.AttemptNo)

	var savedSchedule models.ScheduledPublication
	require.NoError(t, db.First(&savedSchedule, "id = ?", schedule.ID).Error)
	require.Equal(t, models.ScheduledPublicationStatusRunning, savedSchedule.Status)

	require.NoError(t, lifecycle.FinishPublishAttempt(&attempt, publishAttemptCompletion{
		Status:     models.PublishAttemptStatusSucceeded,
		RemoteID:   "remote-1",
		PublishURL: "https://example.com/published",
	}))
	require.NoError(t, db.First(&savedSchedule, "id = ?", schedule.ID).Error)
	require.Equal(t, models.ScheduledPublicationStatusPublished, savedSchedule.Status)
}

func TestPublicationLifecycleMarksPrepublishSyncing(t *testing.T) {
	db := testsupport.SetupTestDB()
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Prepublish",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:    project.ID,
		Platform:     "wechat",
		Enabled:      true,
		Status:       models.PublicationStatusDraft,
		DraftStatus:  models.PublicationDraftStatusUnsynced,
		ErrorMessage: "old error",
	}
	require.NoError(t, db.Create(&pub).Error)

	lifecycle := NewPublicationLifecycle(db)
	require.NoError(t, lifecycle.MarkPrepublishSyncing(project.ID, []string{"wechat"}))

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	require.Equal(t, models.PublicationStatusSyncing, saved.Status)
	require.Equal(t, models.PublicationDraftStatusSyncing, saved.DraftStatus)
	require.Empty(t, saved.ErrorMessage)
}

func TestPublicationLifecycleRejectsPrepublishSyncingDuringActivePublish(t *testing.T) {
	db := testsupport.SetupTestDB()
	user := models.User{Username: "owner", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	project := models.Project{
		UserID:        user.ID,
		Title:         "Prepublish active",
		SourceContent: "content",
		Status:        models.ProjectStatusReady,
	}
	require.NoError(t, db.Create(&project).Error)
	pub := models.ProjectPlatformPublication{
		ProjectID:   project.ID,
		Platform:    "wechat",
		Enabled:     true,
		Status:      models.PublicationStatusQueued,
		DraftStatus: models.PublicationDraftStatusReady,
	}
	require.NoError(t, db.Create(&pub).Error)

	lifecycle := NewPublicationLifecycle(db)
	err := lifecycle.MarkPrepublishSyncing(project.ID, []string{"wechat"})
	require.ErrorIs(t, err, ErrPublicationAlreadyPublishing)

	var saved models.ProjectPlatformPublication
	require.NoError(t, db.First(&saved, "id = ?", pub.ID).Error)
	require.Equal(t, models.PublicationStatusQueued, saved.Status)
	require.Equal(t, models.PublicationDraftStatusReady, saved.DraftStatus)
}
