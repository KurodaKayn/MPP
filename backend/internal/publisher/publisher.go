package publisher

import (
	"github.com/kurodakayn/mpp-backend/internal/publisher/browser"
	"github.com/kurodakayn/mpp-backend/internal/publisher/core"
	douyinpub "github.com/kurodakayn/mpp-backend/internal/publisher/platforms/douyin"
	wechatpub "github.com/kurodakayn/mpp-backend/internal/publisher/platforms/wechat"
	xpub "github.com/kurodakayn/mpp-backend/internal/publisher/platforms/x"
	zhihupub "github.com/kurodakayn/mpp-backend/internal/publisher/platforms/zhihu"
)

type PlatformPublisher = core.PlatformPublisher
type AdaptedContent = core.AdaptedContent
type GeneratedBy = core.GeneratedBy
type AdaptedAsset = core.AdaptedAsset

type Action = browser.Action
type BrowserWorkerClient = browser.BrowserWorkerClient
type CaptureWorkerSessionResponse = browser.CaptureWorkerSessionResponse
type Cookie = browser.Cookie
type CookieRequirement = browser.CookieRequirement
type CookieStore = browser.CookieStore
type DomainRule = browser.DomainRule
type EncryptedEnvelope = browser.EncryptedEnvelope
type GetWorkerSessionResponse = browser.GetWorkerSessionResponse
type HTTPBrowserWorkerClient = browser.HTTPBrowserWorkerClient
type MockBrowserWorkerClient = browser.MockBrowserWorkerClient
type RemoteAccountProfile = browser.RemoteAccountProfile
type RemoteBrowserPlatformAdapter = browser.RemoteBrowserPlatformAdapter
type RemoteLoginState = browser.RemoteLoginState
type StartDouyinPublishRequest = browser.StartDouyinPublishRequest
type StartWorkerSessionRequest = browser.StartWorkerSessionRequest
type StartWorkerSessionResponse = browser.StartWorkerSessionResponse

type DouyinAdapter = douyinpub.DouyinAdapter
type DouyinPublisher = douyinpub.DouyinPublisher
type WechatConfig = wechatpub.WechatConfig
type WechatPublisher = wechatpub.WechatPublisher
type XConfig = xpub.XConfig
type XPublisher = xpub.XPublisher
type ZhihuAdapter = zhihupub.ZhihuAdapter
type ZhihuPublisher = zhihupub.ZhihuPublisher

var ErrCookieEncryptionKeyMissing = browser.ErrCookieEncryptionKeyMissing
var ErrCookieEncryptionKeyInvalid = browser.ErrCookieEncryptionKeyInvalid
var ErrCookieValidationFailed = browser.ErrCookieValidationFailed
var ErrCookieNotFound = browser.ErrCookieNotFound
var ErrBrowserWorkerPoolExhausted = browser.ErrBrowserWorkerPoolExhausted

var BuildXPostIntentURL = xpub.BuildXPostIntentURL
var ContextKeyRemoteURL = browser.ContextKeyRemoteURL
var NewCookieStore = browser.NewCookieStore
var NewHTTPBrowserWorkerClient = browser.NewHTTPBrowserWorkerClient
var NewHTTPBrowserWorkerClientWithToken = browser.NewHTTPBrowserWorkerClientWithToken
var NewMockBrowserWorkerClient = browser.NewMockBrowserWorkerClient
var NormalizePlatformCookies = browser.NormalizePlatformCookies
var PasteContent = browser.PasteContent
var PasteFile = browser.PasteFile
var SetupBrowser = browser.SetupBrowser
var ValidateDouyinCookies = douyinpub.ValidateDouyinCookies
var ValidateZhihuCookies = zhihupub.ValidateZhihuCookies
var WaitForElement = browser.WaitForElement
