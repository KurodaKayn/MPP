package wechat

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	pkgwechat "github.com/kurodakayn/mpp-backend/internal/pkg/wechat"
)

type fakeWechatClient struct {
	uploadImageFn func([]byte, string) (*pkgwechat.MediaResponse, error)
	uploadThumbFn func([]byte, string) (*pkgwechat.MediaResponse, error)
	createDraftFn func([]pkgwechat.Article) (string, error)
	publishFn     func(string) (string, int, error)

	uploadImageCalls [][]byte
	uploadThumbCalls [][]byte
	articles         []pkgwechat.Article
	publishMediaID   string
}

func (f *fakeWechatClient) UploadImage(imageBytes []byte, filename string) (*pkgwechat.MediaResponse, error) {
	f.uploadImageCalls = append(f.uploadImageCalls, append([]byte(nil), imageBytes...))
	if f.uploadImageFn != nil {
		return f.uploadImageFn(imageBytes, filename)
	}
	return &pkgwechat.MediaResponse{URL: "https://mmbiz.qpic.cn/inline.jpg"}, nil
}

func (f *fakeWechatClient) UploadThumb(imageBytes []byte, filename string) (*pkgwechat.MediaResponse, error) {
	f.uploadThumbCalls = append(f.uploadThumbCalls, append([]byte(nil), imageBytes...))
	if f.uploadThumbFn != nil {
		return f.uploadThumbFn(imageBytes, filename)
	}
	return &pkgwechat.MediaResponse{MediaID: "thumb-media-id"}, nil
}

func (f *fakeWechatClient) CreateDraft(articles []pkgwechat.Article) (string, error) {
	f.articles = append([]pkgwechat.Article(nil), articles...)
	if f.createDraftFn != nil {
		return f.createDraftFn(articles)
	}
	return "draft-media-id", nil
}

func (f *fakeWechatClient) Publish(mediaID string) (string, int, error) {
	f.publishMediaID = mediaID
	if f.publishFn != nil {
		return f.publishFn(mediaID)
	}
	return "publish-id", 0, nil
}

func TestWechatPublisherValidateConfigRequiresAppCredentials(t *testing.T) {
	publisher := &WechatPublisher{}

	require.NoError(t, publisher.ValidateConfig([]byte(`{"app_id":"app","app_secret":"secret"}`)))
	require.Error(t, publisher.ValidateConfig([]byte(`{"app_id":"app"}`)))
}

func TestWechatPublisherPublishUsesCompiledAssetsAndCoverFallback(t *testing.T) {
	previousClientFactory := newWechatClient
	previousDownloadForPlatform := downloadAndProcessWechatMediaForPlatform
	previousDownload := downloadAndProcessWechatMedia
	previousRead := readProcessedWechatObject
	defer func() {
		newWechatClient = previousClientFactory
		downloadAndProcessWechatMediaForPlatform = previousDownloadForPlatform
		downloadAndProcessWechatMedia = previousDownload
		readProcessedWechatObject = previousRead
	}()

	fakeClient := &fakeWechatClient{}
	newWechatClient = func(appID, appSecret string) wechatClient {
		assert.Equal(t, "app", appID)
		assert.Equal(t, "secret", appSecret)
		return fakeClient
	}
	downloadAndProcessWechatMedia = func(source string) (string, error) {
		return "", errors.New("unexpected fallback path")
	}
	readProcessedWechatObject = func(_ context.Context, objectRef string) ([]byte, error) {
		if objectRef == "processed-inline" {
			return []byte("inline-bytes"), nil
		}
		if objectRef == "processed-cover" {
			return []byte("cover-bytes"), nil
		}
		return nil, errors.New("unexpected object ref")
	}

	downloadAndProcessWechatMediaForPlatform = func(source, platform, usage string) (string, error) {
		assert.Equal(t, "wechat", platform)
		switch {
		case source == "https://example.com/body.png" && usage == "inline_image":
			return "processed-inline", nil
		case source == "https://example.com/cover.png" && usage == "cover":
			return "processed-cover", nil
		default:
			return "", errors.New("unexpected source")
		}
	}

	pub := &models.ProjectPlatformPublication{
		Config: datatypes.JSON(`{
			"app_id":"app",
			"app_secret":"secret",
			"title":"WeChat title",
			"author":"Author",
			"digest":"Digest",
			"cover_image_url":"https://example.com/cover.png"
		}`),
		AdaptedContent: datatypes.JSON(`{
			"html":"<p>Base <img src=\"https://example.com/body.png\"></p>",
			"assets":[{"type":"image","source_url":"https://example.com/body.png"}]
		}`),
	}

	mediaID, publishURL, err := (&WechatPublisher{}).Publish(context.Background(), pub, nil)
	require.NoError(t, err)
	assert.Equal(t, "draft-media-id", mediaID)
	assert.Equal(t, "https://mp.weixin.qq.com/s?publish_id=publish-id", publishURL)
	require.Len(t, fakeClient.articles, 1)
	assert.Equal(t, "WeChat title", fakeClient.articles[0].Title)
	assert.Equal(t, "Author", fakeClient.articles[0].Author)
	assert.Equal(t, "Digest", fakeClient.articles[0].Digest)
	assert.Contains(t, fakeClient.articles[0].Content, "https://mmbiz.qpic.cn/inline.jpg")
	assert.Equal(t, []byte("inline-bytes"), fakeClient.uploadImageCalls[0])
	assert.Equal(t, []byte("cover-bytes"), fakeClient.uploadThumbCalls[0])
}

func TestWechatPublisherPublishFallsBackToSourceHTMLWhenInlineProcessingFails(t *testing.T) {
	previousClientFactory := newWechatClient
	previousDownloadForPlatform := downloadAndProcessWechatMediaForPlatform
	previousRead := readProcessedWechatObject
	defer func() {
		newWechatClient = previousClientFactory
		downloadAndProcessWechatMediaForPlatform = previousDownloadForPlatform
		readProcessedWechatObject = previousRead
	}()

	fakeClient := &fakeWechatClient{}
	newWechatClient = func(_, _ string) wechatClient { return fakeClient }
	downloadAndProcessWechatMediaForPlatform = func(source, _, usage string) (string, error) {
		if source == "https://example.com/cover.png" && usage == "cover" {
			return "processed-cover", nil
		}
		return "", errors.New("download failed")
	}
	readProcessedWechatObject = func(context.Context, string) ([]byte, error) {
		return []byte("cover-bytes"), nil
	}

	pub := &models.ProjectPlatformPublication{
		Config: datatypes.JSON(`{"app_id":"app","app_secret":"secret","title":"Title","cover_image_url":"https://example.com/cover.png"}`),
		AdaptedContent: datatypes.JSON(`{
			"html":"<p>Source <img src=\"https://example.com/body.png\"></p>",
			"assets":[{"type":"image","source_url":"https://example.com/body.png"}]
		}`),
	}

	mediaID, _, err := (&WechatPublisher{}).Publish(context.Background(), pub, nil)
	require.NoError(t, err)
	assert.Equal(t, "draft-media-id", mediaID)
	require.Len(t, fakeClient.articles, 1)
	assert.Equal(t, `<p>Source <img src="https://example.com/body.png"></p>`, fakeClient.articles[0].Content)
}

func TestLoadCoverImageUsesDefaultAssetWhenCoverMissing(t *testing.T) {
	previousRead := readProcessedWechatObject
	defer func() {
		readProcessedWechatObject = previousRead
	}()

	readProcessedWechatObject = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("unexpected cover lookup")
	}
	t.Chdir("../../../../")

	data, err := loadCoverImage(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, data)
}

func TestWechatPublisherReportsUnauthorizedPublishQualification(t *testing.T) {
	previousClientFactory := newWechatClient
	previousDownloadForPlatform := downloadAndProcessWechatMediaForPlatform
	previousRead := readProcessedWechatObject
	defer func() {
		newWechatClient = previousClientFactory
		downloadAndProcessWechatMediaForPlatform = previousDownloadForPlatform
		readProcessedWechatObject = previousRead
	}()

	fakeClient := &fakeWechatClient{
		publishFn: func(string) (string, int, error) {
			return "publish-id", 48001, nil
		},
	}
	newWechatClient = func(_, _ string) wechatClient { return fakeClient }
	downloadAndProcessWechatMediaForPlatform = func(_, _, _ string) (string, error) {
		return "processed-cover", nil
	}
	readProcessedWechatObject = func(context.Context, string) ([]byte, error) {
		return []byte("cover-bytes"), nil
	}

	pub := &models.ProjectPlatformPublication{
		Config:         datatypes.JSON(`{"app_id":"app","app_secret":"secret","title":"Title","cover_image_url":"https://example.com/cover.png"}`),
		AdaptedContent: datatypes.JSON(`{"html":"<p>Source HTML</p>"}`),
	}

	mediaID, publishURL, err := (&WechatPublisher{}).Publish(context.Background(), pub, nil)
	require.Error(t, err)
	assert.Equal(t, "draft-media-id", mediaID)
	assert.Empty(t, publishURL)
	assert.Contains(t, err.Error(), "统一发布资格")
}
