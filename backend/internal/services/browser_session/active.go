package browsersession

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *BrowserSessionService) activeSessionExists(ctx context.Context, userID uuid.UUID, platform string, now time.Time) (bool, error) {
	var sessions []models.RemoteBrowserSession
	// Search for ALL sessions with active statuses (ignore expires_at for now to handle stale index rows)
	err := s.writerDB(ctx).
		Where("user_id = ? AND platform = ? AND status IN ?", userID, platform, activeBrowserSessionStatuses()).
		Find(&sessions).Error
	if err != nil {
		return false, err
	}

	for i := range sessions {
		session := &sessions[i]

		// 1. If actually expired by time, mark it so and continue
		if session.ExpiresAt.Before(now) {
			if err := s.expireStaleSession(ctx, session, "session expired"); err != nil {
				return false, err
			}
			continue
		}

		// 2. If no worker ref, check for pending timeout
		if session.WorkerSessionRef == "" {
			if session.CreatedAt.Add(pendingSessionStaleAfter).After(now) {
				return true, nil
			}
			if err := s.expireStaleSession(ctx, session, "worker session reference is missing"); err != nil {
				return false, err
			}
			continue
		}

		// 3. Verify with worker
		if _, err := s.workerClient.GetSession(ctx, session.WorkerSessionRef); err != nil {
			if err := s.expireStaleSession(ctx, session, "worker session is unavailable"); err != nil {
				return false, err
			}
			continue
		}
		return true, nil
	}

	return false, nil
}

func (s *BrowserSessionService) expireStaleSession(ctx context.Context, session *models.RemoteBrowserSession, message string) error {
	return s.writerDB(ctx).Model(session).Updates(map[string]any{
		"status":        models.BrowserSessionStatusExpired,
		"error_message": message,
	}).Error
}

func (s *BrowserSessionService) expireSupersededActiveRows(ctx context.Context, userID uuid.UUID, platform string) error {
	return s.writerDB(ctx).Model(&models.RemoteBrowserSession{}).
		Where("user_id = ? AND platform = ? AND status IN ?", userID, platform, activeBrowserSessionStatuses()).
		Updates(map[string]any{
			"status":        models.BrowserSessionStatusExpired,
			"error_message": "superseded by redis active-session lock recovery",
		}).Error
}
