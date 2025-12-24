package orbo

import (
	"context"
	"log"

	config "orbo/gen/config"
)

// config service example implementation.
// The example methods log the requests and return zero values.
// NOTE: The actual implementation is in internal/services/config_impl.go
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

// GetDinov3 returns the current DINOv3 AI configuration
func (s *configsrvc) GetDinov3(ctx context.Context) (res *config.DINOv3Config, err error) {
	res = &config.DINOv3Config{}
	s.logger.Print("config.get_dinov3")
	return
}

// UpdateDinov3 updates the DINOv3 AI configuration
func (s *configsrvc) UpdateDinov3(ctx context.Context, p *config.DINOv3Config) (res *config.DINOv3Config, err error) {
	res = &config.DINOv3Config{}
	s.logger.Print("config.update_dinov3")
	return
}

// TestDinov3 tests the DINOv3 service connectivity
func (s *configsrvc) TestDinov3(ctx context.Context) (res *config.TestDinov3Result, err error) {
	res = &config.TestDinov3Result{}
	s.logger.Print("config.test_dinov3")
	return
}

// GetYolo returns the current YOLO detection configuration
func (s *configsrvc) GetYolo(ctx context.Context) (res *config.YOLOConfig, err error) {
	res = &config.YOLOConfig{}
	s.logger.Print("config.get_yolo")
	return
}

// UpdateYolo updates the YOLO detection configuration
func (s *configsrvc) UpdateYolo(ctx context.Context, p *config.YOLOConfig) (res *config.YOLOConfig, err error) {
	res = &config.YOLOConfig{}
	s.logger.Print("config.update_yolo")
	return
}

// TestYolo tests the YOLO service connectivity
func (s *configsrvc) TestYolo(ctx context.Context) (res *config.TestYoloResult, err error) {
	res = &config.TestYoloResult{}
	s.logger.Print("config.test_yolo")
	return
}

// GetDetection returns the combined detection configuration
func (s *configsrvc) GetDetection(ctx context.Context) (res *config.DetectionConfig, err error) {
	res = &config.DetectionConfig{}
	s.logger.Print("config.get_detection")
	return
}

// UpdateDetection updates the combined detection configuration
func (s *configsrvc) UpdateDetection(ctx context.Context, p *config.DetectionConfig) (res *config.DetectionConfig, err error) {
	res = &config.DetectionConfig{}
	s.logger.Print("config.update_detection")
	return
}
