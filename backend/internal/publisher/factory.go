// Package publisher wires platform publishing adapters behind a shared factory.
package publisher

import (
	"fmt"
)

// Registry stores platform publishers by platform key.
type Registry struct {
	publishers map[string]PlatformPublisher
}

// NewPublisherFactory creates an empty publisher registry.
func NewPublisherFactory() *Registry {
	return &Registry{
		publishers: make(map[string]PlatformPublisher),
	}
}

// Register adds or replaces the publisher for a platform key.
func (f *Registry) Register(platform string, p PlatformPublisher) {
	f.publishers[platform] = p
}

// GetPublisher returns the publisher registered for a platform key.
func (f *Registry) GetPublisher(platform string) (PlatformPublisher, error) {
	p, ok := f.publishers[platform]
	if !ok {
		return nil, fmt.Errorf("no publisher registered for platform: %s", platform)
	}
	return p, nil
}

// Factory is the default publisher registry.
var Factory = NewPublisherFactory()

func init() {
	// Register default publishers
	Factory.Register("wechat", &WechatPublisher{})
	Factory.Register("x", &XPublisher{})
	Factory.Register("zhihu", &ZhihuPublisher{})
	Factory.Register("douyin", &DouyinPublisher{})
}
