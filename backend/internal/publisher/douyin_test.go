package publisher

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

// TestDouyinPublisher_Publish_NoAccount 验证没有账号信息时是否正确报错
func TestDouyinPublisher_Publish_NoAccount(t *testing.T) {
	p := &DouyinPublisher{}
	pub := &models.ProjectPlatformPublication{
		ID:       uuid.New(),
		Platform: "douyin",
	}

	remoteID, url, err := p.Publish(context.Background(), pub, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires an account with cookies")
	assert.Empty(t, remoteID)
	assert.Empty(t, url)
}

// TestDouyinPublisher_AdaptContent 验证正文适配逻辑
func TestDouyinPublisher_AdaptContent(t *testing.T) {
	p := &DouyinPublisher{}
	project := &models.Project{
		SourceContent: "抖音测试正文 #自动化发布",
	}

	content, err := p.AdaptContent(project)

	assert.NoError(t, err)
	// 抖音目前直接透传原始正文，验证内容是否存在
	assert.Equal(t, "抖音测试正文 #自动化发布", string(content))
}

// TestDouyinPublisher_Publish_InvalidContext 验证在异常环境下的快速失败
func TestDouyinPublisher_Publish_InvalidContext(t *testing.T) {
	p := &DouyinPublisher{}
	pub := &models.ProjectPlatformPublication{
		ID:       uuid.New(),
		Platform: "douyin",
		AdaptedContent: []byte("Test content"),
	}
	account := &models.PlatformAccount{
		Platform: "douyin",
		Cookies:  datatypes.JSON("[]"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, _, err := p.Publish(ctx, pub, account)
	
	// 由于 context 已取消，应该报错
	assert.Error(t, err)
}
