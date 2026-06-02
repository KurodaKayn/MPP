package douyin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/publisher/core"
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
		Title:         "抖音标题",
		SourceContent: "<p>抖音测试正文 <strong>#自动化发布</strong></p>",
	}

	content, err := p.AdaptContent(project)

	assert.NoError(t, err)
	var adapted core.AdaptedContent
	assert.NoError(t, json.Unmarshal(content, &adapted))
	assert.Equal(t, 1, adapted.SchemaVersion)
	assert.Equal(t, "text", adapted.Format)
	assert.Equal(t, "douyin-text-adapter", adapted.GeneratedBy.ID)
	assert.Equal(t, "抖音测试正文 #自动化发布", adapted.Text)
}

func TestExtractDouyinTextSupportsUnifiedSchema(t *testing.T) {
	content := extractDouyinText([]byte(`{"format":"text","text":"抖音正文"}`))

	assert.Equal(t, "抖音正文", content)
}

// TestDouyinPublisher_Publish_InvalidContext 验证在异常环境下的快速失败
func TestDouyinPublisher_Publish_InvalidContext(t *testing.T) {
	p := &DouyinPublisher{}
	pub := &models.ProjectPlatformPublication{
		ID:             uuid.New(),
		Platform:       "douyin",
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
