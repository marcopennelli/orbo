package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TelegramBot handles Telegram bot operations
type TelegramBot struct {
	botToken   string
	chatID     string
	httpClient *http.Client
	mu         sync.RWMutex
	enabled    bool
	cooldownTracker map[string]time.Time
	cooldownPeriod  time.Duration
}

// Config holds Telegram bot configuration
type Config struct {
	BotToken        string
	ChatID          string
	Enabled         bool
	CooldownSeconds int
}

// Message represents a Telegram message
type Message struct {
	Text     string
	PhotoURL string
	PhotoData []byte
}

// TelegramResponse represents the response from Telegram API
type TelegramResponse struct {
	OK     bool   `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	ErrorCode int   `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewTelegramBot creates a new Telegram bot instance
func NewTelegramBot(config Config) *TelegramBot {
	cooldownPeriod := time.Duration(config.CooldownSeconds) * time.Second
	if cooldownPeriod == 0 {
		cooldownPeriod = 30 * time.Second // Default 30 seconds cooldown
	}

	return &TelegramBot{
		botToken:   config.BotToken,
		chatID:     config.ChatID,
		enabled:    config.Enabled,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cooldownTracker: make(map[string]time.Time),
		cooldownPeriod: cooldownPeriod,
	}
}

// IsEnabled returns whether the bot is enabled
func (tb *TelegramBot) IsEnabled() bool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.enabled
}

// SetEnabled enables or disables the bot
func (tb *TelegramBot) SetEnabled(enabled bool) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.enabled = enabled
}

// UpdateConfig updates the bot configuration
func (tb *TelegramBot) UpdateConfig(config Config) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	
	tb.botToken = config.BotToken
	tb.chatID = config.ChatID
	tb.enabled = config.Enabled
	
	if config.CooldownSeconds > 0 {
		tb.cooldownPeriod = time.Duration(config.CooldownSeconds) * time.Second
	}
}

// SendMessage sends a text message
func (tb *TelegramBot) SendMessage(ctx context.Context, message string) error {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if !tb.enabled {
		return fmt.Errorf("telegram bot is disabled")
	}

	if tb.botToken == "" || tb.chatID == "" {
		return fmt.Errorf("telegram bot token or chat ID not configured")
	}

	// Check cooldown
	if !tb.checkCooldown("message") {
		return fmt.Errorf("message cooldown period not yet elapsed")
	}

	payload := map[string]interface{}{
		"chat_id": tb.chatID,
		"text":    message,
		"parse_mode": "HTML",
	}

	err := tb.sendTelegramRequest(ctx, "sendMessage", payload)
	if err == nil {
		tb.updateCooldown("message")
	}
	
	return err
}

// SendPhoto sends a photo with optional caption
func (tb *TelegramBot) SendPhoto(ctx context.Context, photoData []byte, caption string) error {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if !tb.enabled {
		return fmt.Errorf("telegram bot is disabled")
	}

	if tb.botToken == "" || tb.chatID == "" {
		return fmt.Errorf("telegram bot token or chat ID not configured")
	}

	// Check cooldown
	if !tb.checkCooldown("photo") {
		return fmt.Errorf("photo cooldown period not yet elapsed")
	}

	err := tb.sendPhoto(ctx, photoData, caption)
	if err == nil {
		tb.updateCooldown("photo")
	}
	
	return err
}

// SendMotionAlert sends a motion detection alert with frame
func (tb *TelegramBot) SendMotionAlert(ctx context.Context, cameraName string, confidence float32, frameData []byte) error {
	// Format time with timezone to match UI display
	now := time.Now()
	zoneName, _ := now.Zone()
	timestamp := fmt.Sprintf("%s %s", now.Format("2 Jan 2006, 15:04:05"), zoneName)

	message := fmt.Sprintf(
		"🚨 <b>Detection Alert!</b>\n\n"+
			"📹 Camera: %s\n"+
			"🕐 Time: %s",
		cameraName,
		timestamp,
	)

	if frameData != nil && len(frameData) > 0 {
		return tb.SendPhoto(ctx, frameData, message)
	}

	return tb.SendMessage(ctx, message)
}

// FaceRecognitionInfo contains face recognition results for notifications
type FaceRecognitionInfo struct {
	FacesDetected       int
	KnownIdentities     []string
	UnknownFacesCount   int
	ForensicThumbnails  [][]byte // Forensic face analysis thumbnails
}

// SendMotionAlertWithFaces sends a motion detection alert with face recognition info
func (tb *TelegramBot) SendMotionAlertWithFaces(ctx context.Context, cameraName string, objectClass string, threatLevel string, faceInfo *FaceRecognitionInfo, frameData []byte) error {
	// Format time with timezone to match UI display
	now := time.Now()
	zoneName, _ := now.Zone()
	timestamp := fmt.Sprintf("%s %s", now.Format("2 Jan 2006, 15:04:05"), zoneName)

	// Build message with detection info
	var threatEmoji string
	switch threatLevel {
	case "high":
		threatEmoji = "🔴"
	case "medium":
		threatEmoji = "🟡"
	case "low":
		threatEmoji = "🟢"
	default:
		threatEmoji = "⚪"
	}

	message := fmt.Sprintf(
		"🚨 <b>Detection Alert!</b>\n\n"+
			"📹 Camera: %s\n"+
			"🎯 Detected: %s\n"+
			"%s Threat: %s\n"+
			"🕐 Time: %s",
		cameraName,
		objectClass,
		threatEmoji,
		threatLevel,
		timestamp,
	)

	// Add face recognition info if available
	if faceInfo != nil && faceInfo.FacesDetected > 0 {
		message += fmt.Sprintf("\n\n👤 <b>Face Recognition:</b>")

		if len(faceInfo.KnownIdentities) > 0 {
			message += fmt.Sprintf("\n✅ Identified: %s", strings.Join(faceInfo.KnownIdentities, ", "))
		}

		if faceInfo.UnknownFacesCount > 0 {
			message += fmt.Sprintf("\n❓ Unknown faces: %d", faceInfo.UnknownFacesCount)
		}
	}

	// Send main frame with caption first
	if frameData != nil && len(frameData) > 0 {
		err := tb.SendPhoto(ctx, frameData, message)
		if err != nil {
			return err
		}
	} else {
		err := tb.SendMessage(ctx, message)
		if err != nil {
			return err
		}
	}

	// Send forensic thumbnails as separate photos if available
	if faceInfo != nil && len(faceInfo.ForensicThumbnails) > 0 {
		for i, thumbnailData := range faceInfo.ForensicThumbnails {
			if len(thumbnailData) > 0 {
				caption := fmt.Sprintf("🔬 Face Analysis #%d", i+1)
				// Skip cooldown check for follow-up forensic photos
				err := tb.sendPhoto(ctx, thumbnailData, caption)
				if err != nil {
					fmt.Printf("Warning: failed to send forensic thumbnail %d: %v\n", i+1, err)
					// Continue with other thumbnails even if one fails
				}
			}
		}
	}

	return nil
}

// SendTestMessage sends a test message to verify the bot configuration
func (tb *TelegramBot) SendTestMessage(ctx context.Context) error {
	// Format time with timezone to match UI display
	now := time.Now()
	zoneName, _ := now.Zone()
	timestamp := fmt.Sprintf("%s %s", now.Format("2 Jan 2006, 15:04:05"), zoneName)

	message := fmt.Sprintf(
		"🤖 <b>Orbo Test Message</b>\n\n"+
			"✅ Telegram bot is working correctly!\n"+
			"🕐 Test sent at: %s",
		timestamp,
	)

	return tb.SendMessage(ctx, message)
}

// sendPhoto sends a photo using multipart form data
func (tb *TelegramBot) sendPhoto(ctx context.Context, photoData []byte, caption string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", tb.botToken)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add chat_id field
	err := writer.WriteField("chat_id", tb.chatID)
	if err != nil {
		return fmt.Errorf("failed to write chat_id field: %w", err)
	}

	// Add caption if provided
	if caption != "" {
		err = writer.WriteField("caption", caption)
		if err != nil {
			return fmt.Errorf("failed to write caption field: %w", err)
		}
		
		err = writer.WriteField("parse_mode", "HTML")
		if err != nil {
			return fmt.Errorf("failed to write parse_mode field: %w", err)
		}
	}

	// Add photo data
	part, err := writer.CreateFormFile("photo", "motion_frame.jpg")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = part.Write(photoData)
	if err != nil {
		return fmt.Errorf("failed to write photo data: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := tb.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}
	defer resp.Body.Close()

	return tb.handleResponse(resp)
}

// sendTelegramRequest sends a generic request to Telegram API
func (tb *TelegramBot) sendTelegramRequest(ctx context.Context, method string, payload map[string]interface{}) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", tb.botToken, method)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := tb.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	return tb.handleResponse(resp)
}

// handleResponse processes the Telegram API response
func (tb *TelegramBot) handleResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var telegramResp TelegramResponse
	err = json.Unmarshal(body, &telegramResp)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !telegramResp.OK {
		return fmt.Errorf("telegram API error %d: %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	return nil
}

// checkCooldown checks if the cooldown period has elapsed for a specific action type
func (tb *TelegramBot) checkCooldown(actionType string) bool {
	lastTime, exists := tb.cooldownTracker[actionType]
	if !exists {
		return true
	}

	return time.Since(lastTime) >= tb.cooldownPeriod
}

// updateCooldown updates the last action time for cooldown tracking
func (tb *TelegramBot) updateCooldown(actionType string) {
	tb.cooldownTracker[actionType] = time.Now()
}

// ValidateConfig validates the Telegram bot configuration
func ValidateConfig(config Config) error {
	if config.Enabled {
		if config.BotToken == "" {
			return fmt.Errorf("telegram bot token is required when enabled")
		}
		
		if config.ChatID == "" {
			return fmt.Errorf("telegram chat ID is required when enabled")
		}
	}

	if config.CooldownSeconds < 0 {
		return fmt.Errorf("cooldown seconds cannot be negative")
	}

	return nil
}

// GetBotInfo retrieves information about the bot
func (tb *TelegramBot) GetBotInfo(ctx context.Context) (map[string]interface{}, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	if tb.botToken == "" {
		return nil, fmt.Errorf("bot token not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", tb.botToken)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := tb.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get bot info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var telegramResp TelegramResponse
	err = json.Unmarshal(body, &telegramResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !telegramResp.OK {
		return nil, fmt.Errorf("telegram API error %d: %s", telegramResp.ErrorCode, telegramResp.Description)
	}

	if result, ok := telegramResp.Result.(map[string]interface{}); ok {
		return result, nil
	}

	return nil, fmt.Errorf("unexpected response format")
}

// CleanupCooldownTracking removes old cooldown entries to prevent memory leaks
func (tb *TelegramBot) CleanupCooldownTracking() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	for actionType, lastTime := range tb.cooldownTracker {
		if now.Sub(lastTime) > tb.cooldownPeriod*2 {
			delete(tb.cooldownTracker, actionType)
		}
	}
}