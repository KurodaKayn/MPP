package publisher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
)

// TestZhihuPublisher_Publish_NoAccount 验证没有账号信息时是否报错
func TestZhihuPublisher_Publish_NoAccount(t *testing.T) {
	p := &ZhihuPublisher{}
	pub := &models.ProjectPlatformPublication{
		ID:       uuid.New(),
		Platform: "zhihu",
	}

	remoteID, url, err := p.Publish(context.Background(), pub, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account information is required")
	assert.Empty(t, remoteID)
	assert.Empty(t, url)
}

// TestZhihuPublisher_AdaptContent 验证内容适配逻辑
func TestZhihuPublisher_AdaptContent(t *testing.T) {
	p := &ZhihuPublisher{}
	project := &models.Project{
		Title:         "知乎标题",
		SourceContent: `<h2>小标题</h2><p>这是一段<strong>知乎测试</strong>正文。</p><blockquote>引用</blockquote><ul><li>第一点</li></ul><p><img src="https://example.com/a.png" alt="配图"></p>`,
	}

	content, err := p.AdaptContent(project)

	assert.NoError(t, err)
	var adapted AdaptedContent
	assert.NoError(t, json.Unmarshal(content, &adapted))
	assert.Equal(t, 1, adapted.SchemaVersion)
	assert.Equal(t, "markdown", adapted.Format)
	assert.Equal(t, "zhihu-markdown-adapter", adapted.GeneratedBy.ID)
	assert.Contains(t, adapted.Markdown, "## 小标题")
	assert.Contains(t, adapted.Markdown, "**知乎测试**")
	assert.Contains(t, adapted.Markdown, "> 引用")
	assert.Contains(t, adapted.Markdown, "- 第一点")
	assert.Contains(t, adapted.Markdown, "![配图](https://example.com/a.png)")
}

// TestZhihuPublisher_Publish_AccountWithEmptyCookies 验证账号存在但 Cookie 为空时的初始校验
func TestZhihuPublisher_Publish_AccountWithEmptyCookies(t *testing.T) {
	// 注意：由于真实的 Publish 会弹出浏览器，这里仅做逻辑占位或 Mock 测试
	// 在目前的实现中，即使 Cookie 为空，SetupBrowser 也会被调用。
	// 这里通过一个带 context 的超时来模拟一个快速失败的场景。

	p := &ZhihuPublisher{}
	pub := &models.ProjectPlatformPublication{
		ID:             uuid.New(),
		Platform:       "zhihu",
		AdaptedContent: []byte("Test content"),
	}
	account := &models.PlatformAccount{
		Platform: "zhihu",
		Cookies:  datatypes.JSON("[]"),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消以防止真的打开浏览器

	_, _, err := p.Publish(ctx, pub, account)

	// 验证是否触发了流程（因为 context 已取消，会报 context canceled）
	assert.Error(t, err)
}
