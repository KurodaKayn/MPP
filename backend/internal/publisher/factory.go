package publisher

import (
	"fmt"
)

type PublisherFactory struct {
	publishers map[string]PlatformPublisher
}

func NewPublisherFactory() *PublisherFactory {
	return &PublisherFactory{
		publishers: make(map[string]PlatformPublisher),
	}
}

func (f *PublisherFactory) Register(platform string, p PlatformPublisher) {
	f.publishers[platform] = p
}

func (f *PublisherFactory) GetPublisher(platform string) (PlatformPublisher, error) {
	p, ok := f.publishers[platform]
	if !ok {
		return nil, fmt.Errorf("no publisher registered for platform: %s", platform)
	}
	return p, nil
}

// Global factory instance for ease of use (can also be injected via DI)
var Factory = NewPublisherFactory()

func init() {
	// Register default publishers
	Factory.Register("wechat", &WechatPublisher{})
	Factory.Register("x", &XPublisher{})
	Factory.Register("zhihu", &ZhihuPublisher{})
	Factory.Register("douyin", &DouyinPublisher{})
}
