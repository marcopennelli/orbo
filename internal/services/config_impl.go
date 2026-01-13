package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	config "orbo/gen/config"
	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/motion"
	"orbo/internal/pipeline"
	"orbo/internal/telegram"
)

// splitAndTrim splits a string by separator and trims whitespace from each element
func splitAndTrim(s string, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ConfigImplementation implements the config service
type ConfigImplementation struct {
	mu              sync.RWMutex
	telegramBot     *telegram.TelegramBot
	dinov3Detector  *detection.DINOv3Detector
	yoloDetector    *detection.YOLODetector
	motionDetector  *motion.MotionDetector
	notificationCfg *config.NotificationConfig
	dinov3Cfg       *config.DINOv3Config
	yoloCfg         *config.YOLOConfig
	recognitionCfg  *config.RecognitionConfig
	detectionCfg    *config.DetectionConfig
	pipelineCfg     *config.PipelineConfig
	db              *database.Database
}

// NewConfigService creates a new config service implementation
func NewConfigService(telegramBot *telegram.TelegramBot, dinov3Detector *detection.DINOv3Detector, yoloDetector *detection.YOLODetector, motionDetector *motion.MotionDetector, db *database.Database) config.Service {
	// Initialize with defaults from environment
	defaultMinConfidence := float32(0.5)
	defaultCooldownSeconds := 30

	notificationCfg := &config.NotificationConfig{
		TelegramEnabled:  telegramBot != nil && telegramBot.IsEnabled(),
		TelegramBotToken: ptrString(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   ptrString(os.Getenv("TELEGRAM_CHAT_ID")),
		MinConfidence:    &defaultMinConfidence,
		CooldownSeconds:  &defaultCooldownSeconds,
	}

	// Initialize DINOv3 config with defaults
	defaultMotionThreshold := float32(0.85)
	defaultDinov3ConfThreshold := float32(0.6)
	dinov3Endpoint := os.Getenv("DINOV3_ENDPOINT")
	if dinov3Endpoint == "" {
		dinov3Endpoint = "http://dinov3-service:8000"
	}

	dinov3Cfg := &config.DINOv3Config{
		Enabled:             os.Getenv("DINOV3_ENABLED") == "true",
		ServiceEndpoint:     &dinov3Endpoint,
		MotionThreshold:     defaultMotionThreshold,
		ConfidenceThreshold: defaultDinov3ConfThreshold,
		FallbackToBasic:     true,
		EnableSceneAnalysis: true,
	}

	// Initialize YOLO config with defaults
	defaultYoloConfThreshold := float32(0.5)
	yoloEndpoint := os.Getenv("YOLO_ENDPOINT")
	if yoloEndpoint == "" {
		yoloEndpoint = "http://yolo-service:8081"
	}

	defaultBoxColor := "#0066FF" // Blue
	yoloCfg := &config.YOLOConfig{
		Enabled:             os.Getenv("YOLO_ENABLED") == "true",
		ServiceEndpoint:     &yoloEndpoint,
		ConfidenceThreshold: defaultYoloConfThreshold,
		SecurityMode:        true,
		ClassesFilter:       nil,
		DrawBoxes:           os.Getenv("YOLO_DRAW_BOXES") == "true",
		BoxColor:            &defaultBoxColor,
		BoxThickness:        2,
	}

	// Initialize Recognition config with defaults
	defaultSimilarityThreshold := float32(0.5)
	recognitionEndpoint := os.Getenv("RECOGNITION_ENDPOINT")
	if recognitionEndpoint == "" {
		recognitionEndpoint = "http://recognition-service:8082"
	}
	defaultKnownFaceColor := "#00FF00" // Green
	defaultUnknownFaceColor := "#FF0000" // Red

	recognitionCfg := &config.RecognitionConfig{
		Enabled:            os.Getenv("RECOGNITION_ENABLED") == "true",
		ServiceEndpoint:    &recognitionEndpoint,
		SimilarityThreshold: defaultSimilarityThreshold,
		KnownFaceColor:     &defaultKnownFaceColor,
		UnknownFaceColor:   &defaultUnknownFaceColor,
		BoxThickness:       2,
	}

	// Determine primary detector from environment
	primaryDetector := os.Getenv("PRIMARY_DETECTOR")
	if primaryDetector == "" {
		primaryDetector = "basic"
	}

	detectionCfg := &config.DetectionConfig{
		PrimaryDetector: primaryDetector,
		Yolo:            yoloCfg,
		Dinov3:          dinov3Cfg,
		FallbackEnabled: true,
	}

	// Initialize pipeline config from environment
	detectionMode := os.Getenv("DETECTION_MODE")
	if detectionMode == "" {
		detectionMode = "motion_triggered"
	}
	executionMode := os.Getenv("DETECTION_EXECUTION_MODE")
	if executionMode == "" {
		executionMode = "sequential"
	}
	scheduleInterval := os.Getenv("DETECTION_SCHEDULE_INTERVAL")
	if scheduleInterval == "" {
		scheduleInterval = "5s"
	}
	motionSensitivity := float32(0.1)
	if envVal := os.Getenv("MOTION_SENSITIVITY"); envVal != "" {
		var val float64
		if _, err := fmt.Sscanf(envVal, "%f", &val); err == nil && val >= 0 && val <= 1 {
			motionSensitivity = float32(val)
		}
	}
	motionCooldownSeconds := 2
	if envVal := os.Getenv("MOTION_COOLDOWN_SECONDS"); envVal != "" {
		var val int
		if _, err := fmt.Sscanf(envVal, "%d", &val); err == nil && val >= 0 {
			motionCooldownSeconds = val
		}
	}
	// Parse detectors from comma-separated env var
	var detectors []string
	if envVal := os.Getenv("DETECTION_DETECTORS"); envVal != "" {
		for _, d := range splitTrim(envVal, ",") {
			if d != "" {
				detectors = append(detectors, d)
			}
		}
	}

	pipelineCfg := &config.PipelineConfig{
		Mode:                  detectionMode,
		ExecutionMode:         executionMode,
		Detectors:             detectors,
		ScheduleInterval:      scheduleInterval,
		MotionSensitivity:     motionSensitivity,
		MotionCooldownSeconds: motionCooldownSeconds,
	}

	impl := &ConfigImplementation{
		telegramBot:     telegramBot,
		dinov3Detector:  dinov3Detector,
		yoloDetector:    yoloDetector,
		motionDetector:  motionDetector,
		notificationCfg: notificationCfg,
		dinov3Cfg:       dinov3Cfg,
		yoloCfg:         yoloCfg,
		recognitionCfg:  recognitionCfg,
		detectionCfg:    detectionCfg,
		pipelineCfg:     pipelineCfg,
		db:              db,
	}

	// Load config from database if available
	if db != nil {
		impl.loadConfigFromDB()
	}

	// Wire up YOLO config provider to motion detector
	// This allows the stream detector to use the current configured threshold
	if motionDetector != nil {
		impl.wireYOLOConfigProvider()
	}

	return impl
}

// wireYOLOConfigProvider sets up the motion detector to get YOLO config from this implementation
func (c *ConfigImplementation) wireYOLOConfigProvider() {
	c.motionDetector.SetYOLOConfig(func() float32 {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.yoloCfg.ConfidenceThreshold
	})
}

// loadConfigFromDB loads configuration from the database
func (c *ConfigImplementation) loadConfigFromDB() {
	// Load notification config
	if jsonStr, err := c.db.GetConfig("notification_config"); err == nil && jsonStr != "" {
		var cfg config.NotificationConfig
		if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
			// Only update non-sensitive fields from DB
			c.notificationCfg.TelegramEnabled = cfg.TelegramEnabled
			c.notificationCfg.MinConfidence = cfg.MinConfidence
			c.notificationCfg.CooldownSeconds = cfg.CooldownSeconds
			// Token and ChatID come from env vars for security
			fmt.Println("Loaded notification config from database")

			// Update telegram bot enabled state from database config
			if c.telegramBot != nil {
				c.telegramBot.SetEnabled(cfg.TelegramEnabled)
				fmt.Printf("Telegram notifications %s (from database)\n", map[bool]string{true: "enabled", false: "disabled"}[cfg.TelegramEnabled])
			}
		}
	}

	// Load detection config
	if jsonStr, err := c.db.GetConfig("detection_config"); err == nil && jsonStr != "" {
		var cfg config.DetectionConfig
		if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
			c.detectionCfg.PrimaryDetector = cfg.PrimaryDetector
			c.detectionCfg.FallbackEnabled = cfg.FallbackEnabled
			fmt.Println("Loaded detection config from database")
		}
	}

	// Load YOLO config
	if jsonStr, err := c.db.GetConfig("yolo_config"); err == nil && jsonStr != "" {
		var cfg config.YOLOConfig
		if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
			c.yoloCfg.Enabled = cfg.Enabled
			c.yoloCfg.ConfidenceThreshold = cfg.ConfidenceThreshold
			c.yoloCfg.SecurityMode = cfg.SecurityMode
			c.yoloCfg.ClassesFilter = cfg.ClassesFilter
			c.yoloCfg.DrawBoxes = cfg.DrawBoxes
			if cfg.BoxColor != nil {
				c.yoloCfg.BoxColor = cfg.BoxColor
			}
			if cfg.BoxThickness != 0 {
				c.yoloCfg.BoxThickness = cfg.BoxThickness
			}
			// Apply draw boxes setting to motion detector
			if c.motionDetector != nil {
				c.motionDetector.SetDrawBoxes(cfg.DrawBoxes)
			}
			// Endpoint comes from env var
			fmt.Println("Loaded YOLO config from database")
		}
	}

	// Load DINOv3 config
	if jsonStr, err := c.db.GetConfig("dinov3_config"); err == nil && jsonStr != "" {
		var cfg config.DINOv3Config
		if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
			c.dinov3Cfg.Enabled = cfg.Enabled
			c.dinov3Cfg.MotionThreshold = cfg.MotionThreshold
			c.dinov3Cfg.ConfidenceThreshold = cfg.ConfidenceThreshold
			c.dinov3Cfg.FallbackToBasic = cfg.FallbackToBasic
			c.dinov3Cfg.EnableSceneAnalysis = cfg.EnableSceneAnalysis
			// Endpoint comes from env var
			fmt.Println("Loaded DINOv3 config from database")
		}
	}

	// Load pipeline config
	if jsonStr, err := c.db.GetConfig("pipeline_config"); err == nil && jsonStr != "" {
		var cfg config.PipelineConfig
		if err := json.Unmarshal([]byte(jsonStr), &cfg); err == nil {
			c.pipelineCfg.Mode = cfg.Mode
			c.pipelineCfg.ExecutionMode = cfg.ExecutionMode
			c.pipelineCfg.Detectors = cfg.Detectors
			c.pipelineCfg.ScheduleInterval = cfg.ScheduleInterval
			c.pipelineCfg.MotionSensitivity = cfg.MotionSensitivity
			c.pipelineCfg.MotionCooldownSeconds = cfg.MotionCooldownSeconds
			fmt.Println("Loaded pipeline config from database")
		}
	}
}

// saveNotificationConfigToDB saves notification config to database
func (c *ConfigImplementation) saveNotificationConfigToDB() {
	if c.db == nil {
		return
	}
	// Save only non-sensitive fields
	safeCfg := config.NotificationConfig{
		TelegramEnabled: c.notificationCfg.TelegramEnabled,
		MinConfidence:   c.notificationCfg.MinConfidence,
		CooldownSeconds: c.notificationCfg.CooldownSeconds,
	}
	if jsonBytes, err := json.Marshal(safeCfg); err == nil {
		if err := c.db.SaveConfig("notification_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save notification config to database: %v\n", err)
		}
	}
}

// saveDetectionConfigToDB saves detection config to database
func (c *ConfigImplementation) saveDetectionConfigToDB() {
	if c.db == nil {
		return
	}
	cfg := config.DetectionConfig{
		PrimaryDetector: c.detectionCfg.PrimaryDetector,
		FallbackEnabled: c.detectionCfg.FallbackEnabled,
	}
	if jsonBytes, err := json.Marshal(cfg); err == nil {
		if err := c.db.SaveConfig("detection_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save detection config to database: %v\n", err)
		}
	}
}

// saveYoloConfigToDB saves YOLO config to database
func (c *ConfigImplementation) saveYoloConfigToDB() {
	if c.db == nil {
		return
	}
	cfg := config.YOLOConfig{
		Enabled:             c.yoloCfg.Enabled,
		ConfidenceThreshold: c.yoloCfg.ConfidenceThreshold,
		SecurityMode:        c.yoloCfg.SecurityMode,
		ClassesFilter:       c.yoloCfg.ClassesFilter,
		DrawBoxes:           c.yoloCfg.DrawBoxes,
		BoxColor:            c.yoloCfg.BoxColor,
		BoxThickness:        c.yoloCfg.BoxThickness,
	}
	if jsonBytes, err := json.Marshal(cfg); err == nil {
		if err := c.db.SaveConfig("yolo_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save YOLO config to database: %v\n", err)
		}
	}
}

// saveDinov3ConfigToDB saves DINOv3 config to database
func (c *ConfigImplementation) saveDinov3ConfigToDB() {
	if c.db == nil {
		return
	}
	cfg := config.DINOv3Config{
		Enabled:             c.dinov3Cfg.Enabled,
		MotionThreshold:     c.dinov3Cfg.MotionThreshold,
		ConfidenceThreshold: c.dinov3Cfg.ConfidenceThreshold,
		FallbackToBasic:     c.dinov3Cfg.FallbackToBasic,
		EnableSceneAnalysis: c.dinov3Cfg.EnableSceneAnalysis,
	}
	if jsonBytes, err := json.Marshal(cfg); err == nil {
		if err := c.db.SaveConfig("dinov3_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save DINOv3 config to database: %v\n", err)
		}
	}
}

// savePipelineConfigToDB saves pipeline config to database
func (c *ConfigImplementation) savePipelineConfigToDB() {
	if c.db == nil {
		return
	}
	cfg := config.PipelineConfig{
		Mode:                  c.pipelineCfg.Mode,
		ExecutionMode:         c.pipelineCfg.ExecutionMode,
		Detectors:             c.pipelineCfg.Detectors,
		ScheduleInterval:      c.pipelineCfg.ScheduleInterval,
		MotionSensitivity:     c.pipelineCfg.MotionSensitivity,
		MotionCooldownSeconds: c.pipelineCfg.MotionCooldownSeconds,
	}
	if jsonBytes, err := json.Marshal(cfg); err == nil {
		if err := c.db.SaveConfig("pipeline_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save pipeline config to database: %v\n", err)
		}
	}
}

// Get returns the current notification configuration
func (c *ConfigImplementation) Get(ctx context.Context) (*config.NotificationConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &config.NotificationConfig{
		TelegramEnabled:  c.notificationCfg.TelegramEnabled,
		TelegramBotToken: maskToken(c.notificationCfg.TelegramBotToken),
		TelegramChatID:   c.notificationCfg.TelegramChatID,
		MinConfidence:    c.notificationCfg.MinConfidence,
		CooldownSeconds:  c.notificationCfg.CooldownSeconds,
	}, nil
}

// Update updates the notification configuration
func (c *ConfigImplementation) Update(ctx context.Context, cfg *config.NotificationConfig) (*config.NotificationConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.TelegramEnabled {
		if cfg.TelegramBotToken == nil || *cfg.TelegramBotToken == "" {
			return nil, &config.BadRequestError{
				Message: "Telegram bot token is required when notifications are enabled",
			}
		}
		if cfg.TelegramChatID == nil || *cfg.TelegramChatID == "" {
			return nil, &config.BadRequestError{
				Message: "Telegram chat ID is required when notifications are enabled",
			}
		}
	}

	if cfg.MinConfidence != nil && (*cfg.MinConfidence < 0 || *cfg.MinConfidence > 1) {
		return nil, &config.BadRequestError{
			Message: "Minimum confidence must be between 0 and 1",
		}
	}

	if cfg.CooldownSeconds != nil && *cfg.CooldownSeconds < 0 {
		return nil, &config.BadRequestError{
			Message: "Cooldown seconds cannot be negative",
		}
	}

	c.notificationCfg.TelegramEnabled = cfg.TelegramEnabled

	if cfg.TelegramBotToken != nil && *cfg.TelegramBotToken != "" && !isMaskedToken(*cfg.TelegramBotToken) {
		c.notificationCfg.TelegramBotToken = cfg.TelegramBotToken
	}
	if cfg.TelegramChatID != nil {
		c.notificationCfg.TelegramChatID = cfg.TelegramChatID
	}
	if cfg.MinConfidence != nil {
		c.notificationCfg.MinConfidence = cfg.MinConfidence
	}
	if cfg.CooldownSeconds != nil {
		c.notificationCfg.CooldownSeconds = cfg.CooldownSeconds
	}

	if c.telegramBot != nil {
		cooldown := 30
		if c.notificationCfg.CooldownSeconds != nil {
			cooldown = *c.notificationCfg.CooldownSeconds
		}

		botToken := ""
		if c.notificationCfg.TelegramBotToken != nil {
			botToken = *c.notificationCfg.TelegramBotToken
		}

		chatID := ""
		if c.notificationCfg.TelegramChatID != nil {
			chatID = *c.notificationCfg.TelegramChatID
		}

		c.telegramBot.UpdateConfig(telegram.Config{
			BotToken:        botToken,
			ChatID:          chatID,
			Enabled:         c.notificationCfg.TelegramEnabled,
			CooldownSeconds: cooldown,
		})
	}

	// Save to database
	c.saveNotificationConfigToDB()

	return &config.NotificationConfig{
		TelegramEnabled:  c.notificationCfg.TelegramEnabled,
		TelegramBotToken: maskToken(c.notificationCfg.TelegramBotToken),
		TelegramChatID:   c.notificationCfg.TelegramChatID,
		MinConfidence:    c.notificationCfg.MinConfidence,
		CooldownSeconds:  c.notificationCfg.CooldownSeconds,
	}, nil
}

// TestNotification sends a test notification
func (c *ConfigImplementation) TestNotification(ctx context.Context) (*config.TestNotificationResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.telegramBot == nil {
		return &config.TestNotificationResult{
			Success: false,
			Message: "Telegram bot is not configured",
		}, nil
	}

	if !c.notificationCfg.TelegramEnabled {
		return &config.TestNotificationResult{
			Success: false,
			Message: "Telegram notifications are disabled",
		}, nil
	}

	err := c.telegramBot.SendTestMessage(ctx)
	if err != nil {
		return &config.TestNotificationResult{
			Success: false,
			Message: "Failed to send test notification: " + err.Error(),
		}, nil
	}

	return &config.TestNotificationResult{
		Success: true,
		Message: "Test notification sent successfully",
	}, nil
}

// GetDinov3 returns the current DINOv3 configuration
func (c *ConfigImplementation) GetDinov3(ctx context.Context) (*config.DINOv3Config, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &config.DINOv3Config{
		Enabled:             c.dinov3Cfg.Enabled,
		ServiceEndpoint:     c.dinov3Cfg.ServiceEndpoint,
		MotionThreshold:     c.dinov3Cfg.MotionThreshold,
		ConfidenceThreshold: c.dinov3Cfg.ConfidenceThreshold,
		FallbackToBasic:     c.dinov3Cfg.FallbackToBasic,
		EnableSceneAnalysis: c.dinov3Cfg.EnableSceneAnalysis,
	}, nil
}

// UpdateDinov3 updates the DINOv3 configuration
func (c *ConfigImplementation) UpdateDinov3(ctx context.Context, cfg *config.DINOv3Config) (*config.DINOv3Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.Enabled && (cfg.ServiceEndpoint == nil || *cfg.ServiceEndpoint == "") {
		return nil, &config.BadRequestError{
			Message: "DINOv3 service endpoint is required when enabled",
		}
	}

	if cfg.MotionThreshold < 0 || cfg.MotionThreshold > 1 {
		return nil, &config.BadRequestError{
			Message: "Motion threshold must be between 0 and 1",
		}
	}

	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return nil, &config.BadRequestError{
			Message: "Confidence threshold must be between 0 and 1",
		}
	}

	c.dinov3Cfg.Enabled = cfg.Enabled
	if cfg.ServiceEndpoint != nil {
		c.dinov3Cfg.ServiceEndpoint = cfg.ServiceEndpoint
	}
	c.dinov3Cfg.MotionThreshold = cfg.MotionThreshold
	c.dinov3Cfg.ConfidenceThreshold = cfg.ConfidenceThreshold
	c.dinov3Cfg.FallbackToBasic = cfg.FallbackToBasic
	c.dinov3Cfg.EnableSceneAnalysis = cfg.EnableSceneAnalysis

	// Update detection config
	c.detectionCfg.Dinov3 = c.dinov3Cfg

	// Save to database
	c.saveDinov3ConfigToDB()

	return &config.DINOv3Config{
		Enabled:             c.dinov3Cfg.Enabled,
		ServiceEndpoint:     c.dinov3Cfg.ServiceEndpoint,
		MotionThreshold:     c.dinov3Cfg.MotionThreshold,
		ConfidenceThreshold: c.dinov3Cfg.ConfidenceThreshold,
		FallbackToBasic:     c.dinov3Cfg.FallbackToBasic,
		EnableSceneAnalysis: c.dinov3Cfg.EnableSceneAnalysis,
	}, nil
}

// TestDinov3 tests the DINOv3 service connectivity
func (c *ConfigImplementation) TestDinov3(ctx context.Context) (*config.TestDinov3Result, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.dinov3Detector == nil {
		return &config.TestDinov3Result{
			Healthy: false,
			Message: "DINOv3 detector is not configured",
		}, nil
	}

	if !c.dinov3Cfg.Enabled {
		return &config.TestDinov3Result{
			Healthy: false,
			Message: "DINOv3 detection is disabled",
		}, nil
	}

	startTime := time.Now()
	healthy := c.dinov3Detector.IsHealthy()
	responseTime := float32(time.Since(startTime).Milliseconds())

	endpoint := ""
	if c.dinov3Cfg.ServiceEndpoint != nil {
		endpoint = *c.dinov3Cfg.ServiceEndpoint
	}

	if healthy {
		device := "dinov3"
		return &config.TestDinov3Result{
			Healthy:        true,
			Endpoint:       &endpoint,
			ResponseTimeMs: &responseTime,
			Device:         &device,
			Message:        "DINOv3 service is healthy and responding",
		}, nil
	}

	return &config.TestDinov3Result{
		Healthy:        false,
		Endpoint:       &endpoint,
		ResponseTimeMs: &responseTime,
		Message:        "DINOv3 service is not responding or unhealthy",
	}, nil
}

// GetYolo returns the current YOLO configuration
func (c *ConfigImplementation) GetYolo(ctx context.Context) (*config.YOLOConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &config.YOLOConfig{
		Enabled:             c.yoloCfg.Enabled,
		ServiceEndpoint:     c.yoloCfg.ServiceEndpoint,
		ConfidenceThreshold: c.yoloCfg.ConfidenceThreshold,
		SecurityMode:        c.yoloCfg.SecurityMode,
		ClassesFilter:       c.yoloCfg.ClassesFilter,
		DrawBoxes:           c.yoloCfg.DrawBoxes,
		BoxColor:            c.yoloCfg.BoxColor,
		BoxThickness:        c.yoloCfg.BoxThickness,
	}, nil
}

// UpdateYolo updates the YOLO configuration
func (c *ConfigImplementation) UpdateYolo(ctx context.Context, cfg *config.YOLOConfig) (*config.YOLOConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.Enabled && (cfg.ServiceEndpoint == nil || *cfg.ServiceEndpoint == "") {
		return nil, &config.BadRequestError{
			Message: "YOLO service endpoint is required when enabled",
		}
	}

	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return nil, &config.BadRequestError{
			Message: "Confidence threshold must be between 0 and 1",
		}
	}

	c.yoloCfg.Enabled = cfg.Enabled
	if cfg.ServiceEndpoint != nil {
		c.yoloCfg.ServiceEndpoint = cfg.ServiceEndpoint
	}
	c.yoloCfg.ConfidenceThreshold = cfg.ConfidenceThreshold
	c.yoloCfg.SecurityMode = cfg.SecurityMode
	c.yoloCfg.ClassesFilter = cfg.ClassesFilter
	c.yoloCfg.DrawBoxes = cfg.DrawBoxes
	if cfg.BoxColor != nil {
		c.yoloCfg.BoxColor = cfg.BoxColor
	}
	if cfg.BoxThickness != 0 {
		c.yoloCfg.BoxThickness = cfg.BoxThickness
	}

	// Update the YOLO detector if available
	if c.yoloDetector != nil {
		classesFilter := ""
		if c.yoloCfg.ClassesFilter != nil {
			classesFilter = *c.yoloCfg.ClassesFilter
		}
		endpoint := ""
		if c.yoloCfg.ServiceEndpoint != nil {
			endpoint = *c.yoloCfg.ServiceEndpoint
		}

		c.yoloDetector.UpdateConfig(detection.YOLOConfig{
			Enabled:             c.yoloCfg.Enabled,
			ServiceEndpoint:     endpoint,
			ConfidenceThreshold: c.yoloCfg.ConfidenceThreshold,
			SecurityMode:        c.yoloCfg.SecurityMode,
			ClassesFilter:       classesFilter,
			DrawBoxes:           c.yoloCfg.DrawBoxes,
		})
	}

	// Update motion detector draw boxes setting and gRPC YOLO configuration
	if c.motionDetector != nil {
		c.motionDetector.SetDrawBoxes(c.yoloCfg.DrawBoxes)

		// Send class filter configuration to gRPC YOLO service
		// Parse comma-separated classes filter string into slice
		var classesSlice []string
		if c.yoloCfg.ClassesFilter != nil && *c.yoloCfg.ClassesFilter != "" {
			for _, class := range splitAndTrim(*c.yoloCfg.ClassesFilter, ",") {
				if class != "" {
					classesSlice = append(classesSlice, class)
				}
			}
		}
		if err := c.motionDetector.ConfigureGRPCYOLO(c.yoloCfg.ConfidenceThreshold, classesSlice); err != nil {
			// Log but don't fail - gRPC service might not be available
			fmt.Printf("Warning: failed to configure gRPC YOLO: %v\n", err)
		}
	}

	// Update detection config
	c.detectionCfg.Yolo = c.yoloCfg

	// Save to database
	c.saveYoloConfigToDB()

	return &config.YOLOConfig{
		Enabled:             c.yoloCfg.Enabled,
		ServiceEndpoint:     c.yoloCfg.ServiceEndpoint,
		ConfidenceThreshold: c.yoloCfg.ConfidenceThreshold,
		SecurityMode:        c.yoloCfg.SecurityMode,
		ClassesFilter:       c.yoloCfg.ClassesFilter,
		DrawBoxes:           c.yoloCfg.DrawBoxes,
		BoxColor:            c.yoloCfg.BoxColor,
		BoxThickness:        c.yoloCfg.BoxThickness,
	}, nil
}

// TestYolo tests the YOLO service connectivity
func (c *ConfigImplementation) TestYolo(ctx context.Context) (*config.TestYoloResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.yoloDetector == nil {
		return &config.TestYoloResult{
			Healthy: false,
			Message: "YOLO detector is not configured",
		}, nil
	}

	if !c.yoloCfg.Enabled {
		return &config.TestYoloResult{
			Healthy: false,
			Message: "YOLO detection is disabled",
		}, nil
	}

	startTime := time.Now()
	healthInfo, err := c.yoloDetector.GetHealthInfo()
	responseTime := float32(time.Since(startTime).Milliseconds())

	endpoint := c.yoloDetector.GetEndpoint()

	if err != nil {
		return &config.TestYoloResult{
			Healthy:        false,
			Endpoint:       &endpoint,
			ResponseTimeMs: &responseTime,
			Message:        "YOLO service is not responding: " + err.Error(),
		}, nil
	}

	if healthInfo.ModelLoaded {
		return &config.TestYoloResult{
			Healthy:        true,
			Endpoint:       &endpoint,
			ResponseTimeMs: &responseTime,
			Device:         &healthInfo.Device,
			ModelLoaded:    &healthInfo.ModelLoaded,
			Message:        "YOLO service is healthy and model is loaded",
		}, nil
	}

	modelLoaded := false
	return &config.TestYoloResult{
		Healthy:        false,
		Endpoint:       &endpoint,
		ResponseTimeMs: &responseTime,
		Device:         &healthInfo.Device,
		ModelLoaded:    &modelLoaded,
		Message:        "YOLO service is running but model is not loaded",
	}, nil
}

// GetDetection returns the combined detection configuration
func (c *ConfigImplementation) GetDetection(ctx context.Context) (*config.DetectionConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &config.DetectionConfig{
		PrimaryDetector: c.detectionCfg.PrimaryDetector,
		Yolo:            c.yoloCfg,
		Dinov3:          c.dinov3Cfg,
		FallbackEnabled: c.detectionCfg.FallbackEnabled,
	}, nil
}

// UpdateDetection updates the combined detection configuration
func (c *ConfigImplementation) UpdateDetection(ctx context.Context, cfg *config.DetectionConfig) (*config.DetectionConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate primary detector
	switch cfg.PrimaryDetector {
	case "basic", "yolo", "dinov3":
		// Valid
	default:
		return nil, &config.BadRequestError{
			Message: "Primary detector must be one of: basic, yolo, dinov3",
		}
	}

	// Validate that the selected primary detector is enabled
	if cfg.PrimaryDetector == "yolo" && (cfg.Yolo == nil || !cfg.Yolo.Enabled) {
		return nil, &config.BadRequestError{
			Message: "Cannot set YOLO as primary detector when it is disabled",
		}
	}
	if cfg.PrimaryDetector == "dinov3" && (cfg.Dinov3 == nil || !cfg.Dinov3.Enabled) {
		return nil, &config.BadRequestError{
			Message: "Cannot set DINOv3 as primary detector when it is disabled",
		}
	}

	c.detectionCfg.PrimaryDetector = cfg.PrimaryDetector
	c.detectionCfg.FallbackEnabled = cfg.FallbackEnabled

	// Update individual configs if provided
	if cfg.Yolo != nil {
		c.yoloCfg = cfg.Yolo
		c.detectionCfg.Yolo = cfg.Yolo
	}
	if cfg.Dinov3 != nil {
		c.dinov3Cfg = cfg.Dinov3
		c.detectionCfg.Dinov3 = cfg.Dinov3
	}

	// Save to database
	c.saveDetectionConfigToDB()
	if cfg.Yolo != nil {
		c.saveYoloConfigToDB()
	}
	if cfg.Dinov3 != nil {
		c.saveDinov3ConfigToDB()
	}

	return &config.DetectionConfig{
		PrimaryDetector: c.detectionCfg.PrimaryDetector,
		Yolo:            c.yoloCfg,
		Dinov3:          c.dinov3Cfg,
		FallbackEnabled: c.detectionCfg.FallbackEnabled,
	}, nil
}

// GetMinConfidence returns the minimum confidence threshold for notifications
func (c *ConfigImplementation) GetMinConfidence() float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.notificationCfg.MinConfidence != nil {
		return *c.notificationCfg.MinConfidence
	}
	return 0.5
}

// GetPrimaryDetector returns the primary detector type
func (c *ConfigImplementation) GetPrimaryDetector() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.detectionCfg.PrimaryDetector
}

// GetPipeline returns the detection pipeline configuration
func (c *ConfigImplementation) GetPipeline(ctx context.Context) (*config.PipelineConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to avoid race conditions
	detectors := make([]string, len(c.pipelineCfg.Detectors))
	copy(detectors, c.pipelineCfg.Detectors)

	return &config.PipelineConfig{
		Mode:                  c.pipelineCfg.Mode,
		ExecutionMode:         c.pipelineCfg.ExecutionMode,
		Detectors:             detectors,
		ScheduleInterval:      c.pipelineCfg.ScheduleInterval,
		MotionSensitivity:     c.pipelineCfg.MotionSensitivity,
		MotionCooldownSeconds: c.pipelineCfg.MotionCooldownSeconds,
	}, nil
}

// UpdatePipeline updates the detection pipeline configuration
func (c *ConfigImplementation) UpdatePipeline(ctx context.Context, cfg *config.PipelineConfig) (*config.PipelineConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Validate detection mode
	switch cfg.Mode {
	case "disabled", "visual_only", "continuous", "motion_triggered", "scheduled", "hybrid":
		// Valid
	default:
		return nil, &config.BadRequestError{
			Message: "Detection mode must be one of: disabled, visual_only, continuous, motion_triggered, scheduled, hybrid",
		}
	}

	// Validate execution mode (only sequential is supported)
	// Parallel mode was removed because it causes time-travel effects
	if cfg.ExecutionMode != "" && cfg.ExecutionMode != "sequential" {
		return nil, &config.BadRequestError{
			Message: "Execution mode must be 'sequential' (parallel mode is not supported)",
		}
	}

	// Validate detectors
	validDetectors := map[string]bool{
		"yolo":  true,
		"face":  true,
		"plate": true,
	}
	for _, det := range cfg.Detectors {
		if !validDetectors[det] {
			return nil, &config.BadRequestError{
				Message: fmt.Sprintf("Invalid detector: %s. Must be one of: yolo, face, plate", det),
			}
		}
	}

	// Validate detector ordering in sequential mode (if both YOLO and Face are enabled)
	// Note: Face can work alone - InsightFace has its own face detector
	// But if both are enabled in sequential mode, YOLO should come before Face for efficiency
	if cfg.ExecutionMode == "sequential" {
		hasYOLO := false
		hasFace := false
		yoloIndex := -1
		faceIndex := -1
		for i, det := range cfg.Detectors {
			if det == "yolo" {
				hasYOLO = true
				yoloIndex = i
			}
			if det == "face" {
				hasFace = true
				faceIndex = i
			}
		}
		// If both are enabled, YOLO should come before Face for proper chaining
		if hasFace && hasYOLO && faceIndex < yoloIndex {
			return nil, &config.BadRequestError{
				Message: "In sequential mode, YOLO must be ordered before Face detector (YOLO detects persons first, then faces are recognized within those regions)",
			}
		}
	}

	// Validate motion sensitivity
	if cfg.MotionSensitivity < 0 || cfg.MotionSensitivity > 1 {
		return nil, &config.BadRequestError{
			Message: "Motion sensitivity must be between 0 and 1",
		}
	}

	// Validate motion cooldown
	if cfg.MotionCooldownSeconds < 0 {
		return nil, &config.BadRequestError{
			Message: "Motion cooldown seconds cannot be negative",
		}
	}

	// Validate schedule interval format (simple check)
	if cfg.ScheduleInterval != "" {
		// Basic validation: should end with s, m, or h
		lastChar := cfg.ScheduleInterval[len(cfg.ScheduleInterval)-1]
		if lastChar != 's' && lastChar != 'm' && lastChar != 'h' {
			return nil, &config.BadRequestError{
				Message: "Schedule interval must be a valid Go duration (e.g., '5s', '10s', '1m')",
			}
		}
	}

	// Apply updates
	c.pipelineCfg.Mode = cfg.Mode
	c.pipelineCfg.ExecutionMode = cfg.ExecutionMode
	c.pipelineCfg.Detectors = make([]string, len(cfg.Detectors))
	copy(c.pipelineCfg.Detectors, cfg.Detectors)
	c.pipelineCfg.ScheduleInterval = cfg.ScheduleInterval
	c.pipelineCfg.MotionSensitivity = cfg.MotionSensitivity
	c.pipelineCfg.MotionCooldownSeconds = cfg.MotionCooldownSeconds

	// Save to database
	c.savePipelineConfigToDB()

	// Return a copy of the updated config
	detectors := make([]string, len(c.pipelineCfg.Detectors))
	copy(detectors, c.pipelineCfg.Detectors)

	return &config.PipelineConfig{
		Mode:                  c.pipelineCfg.Mode,
		ExecutionMode:         c.pipelineCfg.ExecutionMode,
		Detectors:             detectors,
		ScheduleInterval:      c.pipelineCfg.ScheduleInterval,
		MotionSensitivity:     c.pipelineCfg.MotionSensitivity,
		MotionCooldownSeconds: c.pipelineCfg.MotionCooldownSeconds,
	}, nil
}

// GetPipelineConfig returns the pipeline config for internal use
func (c *ConfigImplementation) GetPipelineConfig() *config.PipelineConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pipelineCfg
}

// GetRecognition returns the current face recognition configuration
func (c *ConfigImplementation) GetRecognition(ctx context.Context) (*config.RecognitionConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return &config.RecognitionConfig{
		Enabled:            c.recognitionCfg.Enabled,
		ServiceEndpoint:    c.recognitionCfg.ServiceEndpoint,
		SimilarityThreshold: c.recognitionCfg.SimilarityThreshold,
		KnownFaceColor:     c.recognitionCfg.KnownFaceColor,
		UnknownFaceColor:   c.recognitionCfg.UnknownFaceColor,
		BoxThickness:       c.recognitionCfg.BoxThickness,
	}, nil
}

// UpdateRecognition updates the face recognition configuration
func (c *ConfigImplementation) UpdateRecognition(ctx context.Context, cfg *config.RecognitionConfig) (*config.RecognitionConfig, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.Enabled && (cfg.ServiceEndpoint == nil || *cfg.ServiceEndpoint == "") {
		return nil, &config.BadRequestError{
			Message: "Recognition service endpoint is required when enabled",
		}
	}

	if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
		return nil, &config.BadRequestError{
			Message: "Similarity threshold must be between 0 and 1",
		}
	}

	// Validate box thickness (0 means keep current value)
	if cfg.BoxThickness != 0 && (cfg.BoxThickness < 1 || cfg.BoxThickness > 5) {
		return nil, &config.BadRequestError{
			Message: "Box thickness must be between 1 and 5",
		}
	}

	c.recognitionCfg.Enabled = cfg.Enabled
	if cfg.ServiceEndpoint != nil {
		c.recognitionCfg.ServiceEndpoint = cfg.ServiceEndpoint
	}
	c.recognitionCfg.SimilarityThreshold = cfg.SimilarityThreshold
	if cfg.KnownFaceColor != nil {
		c.recognitionCfg.KnownFaceColor = cfg.KnownFaceColor
	}
	if cfg.UnknownFaceColor != nil {
		c.recognitionCfg.UnknownFaceColor = cfg.UnknownFaceColor
	}
	if cfg.BoxThickness != 0 {
		c.recognitionCfg.BoxThickness = cfg.BoxThickness
	}

	// TODO: If we have a recognition detector reference, update its config here
	// For now, color configuration is sent via gRPC Configure call from motion detector

	// Save to database
	c.saveRecognitionConfigToDB()

	return &config.RecognitionConfig{
		Enabled:            c.recognitionCfg.Enabled,
		ServiceEndpoint:    c.recognitionCfg.ServiceEndpoint,
		SimilarityThreshold: c.recognitionCfg.SimilarityThreshold,
		KnownFaceColor:     c.recognitionCfg.KnownFaceColor,
		UnknownFaceColor:   c.recognitionCfg.UnknownFaceColor,
		BoxThickness:       c.recognitionCfg.BoxThickness,
	}, nil
}

// TestRecognition tests the face recognition service connectivity
func (c *ConfigImplementation) TestRecognition(ctx context.Context) (*config.TestRecognitionResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.recognitionCfg.Enabled {
		return &config.TestRecognitionResult{
			Healthy: false,
			Message: "Face recognition is disabled",
		}, nil
	}

	endpoint := ""
	if c.recognitionCfg.ServiceEndpoint != nil {
		endpoint = *c.recognitionCfg.ServiceEndpoint
	}

	// Try to make a health check request to the recognition service
	startTime := time.Now()

	// Simple HTTP health check
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint + "/health")
	responseTime := float32(time.Since(startTime).Milliseconds())

	if err != nil {
		return &config.TestRecognitionResult{
			Healthy:        false,
			Endpoint:       &endpoint,
			ResponseTimeMs: &responseTime,
			Message:        "Recognition service is not responding: " + err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return &config.TestRecognitionResult{
			Healthy:        false,
			Endpoint:       &endpoint,
			ResponseTimeMs: &responseTime,
			Message:        fmt.Sprintf("Recognition service returned status %d", resp.StatusCode),
		}, nil
	}

	// Parse health response
	var healthResp struct {
		Status          string `json:"status"`
		Device          string `json:"device"`
		ModelLoaded     bool   `json:"model_loaded"`
		KnownFacesCount int    `json:"known_faces_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err == nil {
		device := healthResp.Device
		modelLoaded := healthResp.ModelLoaded
		knownFacesCount := healthResp.KnownFacesCount

		if healthResp.Status == "healthy" && modelLoaded {
			return &config.TestRecognitionResult{
				Healthy:         true,
				Endpoint:        &endpoint,
				ResponseTimeMs:  &responseTime,
				Device:          &device,
				ModelLoaded:     &modelLoaded,
				KnownFacesCount: &knownFacesCount,
				Message:         "Recognition service is healthy and model is loaded",
			}, nil
		}
	}

	modelLoaded := false
	return &config.TestRecognitionResult{
		Healthy:        false,
		Endpoint:       &endpoint,
		ResponseTimeMs: &responseTime,
		ModelLoaded:    &modelLoaded,
		Message:        "Recognition service is running but not fully operational",
	}, nil
}

// saveRecognitionConfigToDB saves recognition config to database
func (c *ConfigImplementation) saveRecognitionConfigToDB() {
	if c.db == nil {
		return
	}
	cfg := config.RecognitionConfig{
		Enabled:            c.recognitionCfg.Enabled,
		SimilarityThreshold: c.recognitionCfg.SimilarityThreshold,
		KnownFaceColor:     c.recognitionCfg.KnownFaceColor,
		UnknownFaceColor:   c.recognitionCfg.UnknownFaceColor,
		BoxThickness:       c.recognitionCfg.BoxThickness,
	}
	if jsonBytes, err := json.Marshal(cfg); err == nil {
		if err := c.db.SaveConfig("recognition_config", string(jsonBytes)); err != nil {
			fmt.Printf("Warning: failed to save recognition config to database: %v\n", err)
		}
	}
}

// Helper functions

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func maskToken(token *string) *string {
	if token == nil || *token == "" {
		return nil
	}

	t := *token
	if len(t) <= 8 {
		masked := "****"
		return &masked
	}

	masked := t[:4] + "..." + t[len(t)-4:]
	return &masked
}

func isMaskedToken(token string) bool {
	return len(token) > 0 && (token == "****" || (len(token) >= 11 && token[4:7] == "..."))
}

func splitTrim(s string, sep string) []string {
	parts := make([]string, 0)
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s string, sep string) []string {
	if s == "" {
		return nil
	}
	result := make([]string, 0)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// CreatePipelineConfigProvider creates a function that provides pipeline.EffectiveConfig
// for mode gating in the motion detector. This converts the config service's PipelineConfig
// to the pipeline package's EffectiveConfig format.
func CreatePipelineConfigProvider(configImpl *ConfigImplementation) motion.PipelineConfigProvider {
	return func(cameraID string) *pipeline.EffectiveConfig {
		pipelineCfg := configImpl.GetPipelineConfig()
		if pipelineCfg == nil {
			return nil
		}

		// Convert string mode to pipeline.DetectionMode
		var mode pipeline.DetectionMode
		switch pipelineCfg.Mode {
		case "disabled":
			mode = pipeline.DetectionModeDisabled
		case "continuous":
			mode = pipeline.DetectionModeContinuous
		case "visual_only":
			mode = pipeline.DetectionModeVisualOnly
		case "motion_triggered":
			mode = pipeline.DetectionModeMotionTriggered
		case "scheduled":
			mode = pipeline.DetectionModeScheduled
		case "hybrid":
			mode = pipeline.DetectionModeHybrid
		default:
			mode = pipeline.DetectionModeMotionTriggered
		}

		// Execution mode is always sequential (parallel was removed)
		execMode := pipeline.ExecutionModeSequential

		// Parse schedule interval
		scheduleInterval := 5 * time.Second
		if pipelineCfg.ScheduleInterval != "" {
			if parsed, err := time.ParseDuration(pipelineCfg.ScheduleInterval); err == nil {
				scheduleInterval = parsed
			}
		}

		return &pipeline.EffectiveConfig{
			CameraID:          cameraID,
			Mode:              mode,
			ExecutionMode:     execMode,
			Detectors:         pipelineCfg.Detectors,
			ScheduleInterval:  scheduleInterval,
			MotionSensitivity: pipelineCfg.MotionSensitivity,
			MotionCooldownMs:  pipelineCfg.MotionCooldownSeconds * 1000, // Convert seconds to milliseconds
			YOLOConfidence:    0.5,                                      // Default
			EnableFaceRecog:   true,                                     // Default enabled
			EnablePlateRecog:  false,                                    // Default disabled
		}
	}
}
