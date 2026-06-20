package project

import (
	"gorm.io/datatypes"

	"github.com/kurodakayn/mpp-backend/internal/models"
	"github.com/kurodakayn/mpp-backend/internal/services/project/publicationselection"
	"github.com/kurodakayn/mpp-backend/internal/services/publicationpayload"
)

func pendingPublicationConfigForTemplate(title, summary, coverImageURL string, template *models.ContentTemplate) publicationselection.ConfigForPlatform {
	return func(platform string) (datatypes.JSON, error) {
		config, err := publicationpayload.DefaultConfig(title, summary, coverImageURL)
		if err != nil {
			return nil, err
		}
		return mergePublicationConfig(config, contentTemplatePlatformConfig(template, platform))
	}
}

func defaultPublicationConfigForProjectTitle(title string) publicationselection.ConfigForPlatform {
	return func(string) (datatypes.JSON, error) {
		return publicationpayload.DefaultConfig(title, "", "")
	}
}
