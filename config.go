package orbo

import (
	"context"
	"log"
	config "orbo/gen/config"
)

// config service example implementation.
// The example methods log the requests and return zero values.
type configsrvc struct {
	logger *log.Logger
}

// NewConfig returns the config service implementation.
func NewConfig(logger *log.Logger) config.Service {
	return &configsrvc{logger}
}

// Get current notification configuration
func (s *configsrvc) Get(ctx context.Context) (res *config.NotificationConfig, err error) {
	res = &config.NotificationConfig{}
	s.logger.Print("config.get")
	return
}

// Update notification configuration
func (s *configsrvc) Update(ctx context.Context, p *config.NotificationConfig) (res *config.NotificationConfig, err error) {
	res = &config.NotificationConfig{}
	s.logger.Print("config.update")
	return
}

// Send test notification
func (s *configsrvc) TestNotification(ctx context.Context) (res *config.TestNotificationResult, err error) {
	res = &config.TestNotificationResult{}
	s.logger.Print("config.test_notification")
	return
}
