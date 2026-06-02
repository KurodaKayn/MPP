package dashboard

import (
	"context"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/dto"
)

func (s *DashboardService) BatchPublishProject(projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	return s.publisher.BatchPublishProject(projectID, platforms, scopeUserID)
}

func (s *DashboardService) PublishProject(projectID uuid.UUID, platform string, scopeUserID *uuid.UUID, browserSessionID uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.PublishProject(projectID, platform, scopeUserID, browserSessionID)
}

func (s *DashboardService) CreateXPostIntent(projectID uuid.UUID, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.CreateXPostIntent(projectID, scopeUserID)
}

func (s *DashboardService) EnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platform string, scopeUserID *uuid.UUID) (map[string]interface{}, error) {
	return s.publisher.EnqueuePublishProject(ctx, projectID, platform, scopeUserID)
}

func (s *DashboardService) BatchEnqueuePublishProject(ctx context.Context, projectID uuid.UUID, platforms []string, scopeUserID *uuid.UUID) (map[string]map[string]interface{}, error) {
	return s.publisher.BatchEnqueuePublishProject(ctx, projectID, platforms, scopeUserID)
}

func (s *DashboardService) StartPublishWorker(ctx context.Context) {
	s.publisher.StartPublishWorker(ctx)
}

func (s *DashboardService) GetWechatAccount(userID uuid.UUID) (*dto.WechatAccountResponse, error) {
	return s.accounts.GetWechatAccount(userID)
}

func (s *DashboardService) UpsertWechatAccount(userID uuid.UUID, req dto.UpsertWechatAccountRequest) (*dto.WechatAccountResponse, error) {
	return s.accounts.UpsertWechatAccount(userID, req)
}

func (s *DashboardService) TestWechatAccount(userID uuid.UUID, req dto.TestWechatAccountRequest) (*dto.WechatConnectionTestResponse, error) {
	return s.accounts.TestWechatAccount(userID, req)
}

func (s *DashboardService) GetDouyinAccount(userID uuid.UUID) (*dto.DouyinAccountResponse, error) {
	return s.accounts.GetDouyinAccount(userID)
}

func (s *DashboardService) GetZhihuAccount(userID uuid.UUID) (*dto.ZhihuAccountResponse, error) {
	return s.accounts.GetZhihuAccount(userID)
}

func (s *DashboardService) GetXAccount(userID uuid.UUID) (*dto.XAccountResponse, error) {
	return s.accounts.GetXAccount(userID)
}

func (s *DashboardService) UpsertXAccount(userID uuid.UUID, req dto.UpsertXAccountRequest) (*dto.XAccountResponse, error) {
	return s.accounts.UpsertXAccount(userID, req)
}

func (s *DashboardService) TestXAccount(userID uuid.UUID, req dto.TestXAccountRequest) (*dto.XConnectionTestResponse, error) {
	return s.accounts.TestXAccount(userID, req)
}

func (s *DashboardService) StartXOAuth2(userID uuid.UUID, redirectURI string) (string, error) {
	return s.accounts.StartXOAuth2(userID, redirectURI)
}

func (s *DashboardService) CompleteXOAuth2(ctx context.Context, state, code string) (*dto.XAccountResponse, error) {
	return s.accounts.CompleteXOAuth2(ctx, state, code)
}
