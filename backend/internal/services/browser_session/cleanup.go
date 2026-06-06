package browsersession

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kurodakayn/mpp-backend/internal/models"
)

func (s *BrowserSessionService) StartCleanupWorker(ctx context.Context) {
	if s.redisClient == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := s.CleanupExpiredSessions(ctx, time.Now()); err != nil {
				log.Printf("browser session cleanup failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *BrowserSessionService) CleanupExpiredSessions(ctx context.Context, now time.Time) error {
	if s.redisClient == nil {
		return nil
	}
	sessionIDs, err := s.redisClient.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     browserSessionCleanupKey,
		Start:   "-inf",
		Stop:    fmt.Sprintf("%d", now.UnixMilli()),
		ByScore: true,
	}).Result()
	if err != nil {
		return err
	}
	for _, rawID := range sessionIDs {
		sessionID, err := uuid.Parse(rawID)
		if err != nil {
			_ = s.redisClient.ZRem(ctx, browserSessionCleanupKey, rawID).Err()
			continue
		}
		if err := s.cleanupExpiredSession(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func (s *BrowserSessionService) cleanupExpiredSession(ctx context.Context, sessionID uuid.UUID) error {
	var session models.RemoteBrowserSession
	if err := s.db.WithContext(ctx).First(&session, sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.removeRedisCleanupMember(ctx, sessionID)
		}
		return err
	}
	if isTerminalBrowserSessionStatus(session.Status) {
		return s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)
	}
	if session.WorkerSessionRef != "" {
		_ = s.workerClient.StopSession(ctx, session.WorkerSessionRef)
	}
	if err := s.db.WithContext(ctx).Model(&session).Updates(map[string]any{
		"status":             models.BrowserSessionStatusExpired,
		"error_message":      "session expired",
		"connect_token_hash": "",
	}).Error; err != nil {
		return err
	}
	return s.cleanupRedisSession(ctx, session.UserID, session.Platform, session.ID, session.WorkerSessionRef)
}
