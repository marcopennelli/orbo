package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	config "orbo/gen/config"
	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/motion"
	"orbo/internal/telegram"
)

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
	detectionCfg    *config.DetectionConfig
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

	yoloCfg := &config.YOLOConfig{
		Enabled:             os.Getenv("YOLO_ENABLED") == "true",
		ServiceEndpoint:     &yoloEndpoint,
		ConfidenceThreshold: defaultYoloConfThreshold,
		SecurityMode:        true,
		ClassesFilter:       nil,
		DrawBoxes:           os.Getenv("YOLO_DRAW_BOXES") == "true",
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

	impl := &ConfigImplementation{
		telegramBot:     telegramBot,
		dinov3Detector:  dinov3Detector,
		yoloDetector:    yoloDetector,
		motionDetector:  motionDetector,
		notificationCfg: notificationCfg,
		dinov3Cfg:       dinov3Cfg,
		yoloCfg:         yoloCfg,
		detectionCfg:    detectionCfg,
		db:              db,
	}

	// Load config from database if available
	if db != nil {
		impl.loadConfigFromDB()
	}

	return impl
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

	// Update motion detector draw boxes setting
	if c.motionDetector != nil {
		c.motionDetector.SetDrawBoxes(c.yoloCfg.DrawBoxes)
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
