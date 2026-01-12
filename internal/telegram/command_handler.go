package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"orbo/internal/camera"
	"orbo/internal/database"
)

// MotionDetectorInterface defines the methods needed from the motion detector
// to avoid circular imports
type MotionDetectorInterface interface {
	StartDetection(cameraID, cameraDevice string) error
	StopDetection(cameraID string)
	IsDetectionRunning(cameraID string) bool
	GetEventsForTelegram(cameraID string, since *time.Time, limit int) []MotionEventInfo
}

// MotionEventInfo represents minimal event info needed for command responses
type MotionEventInfo struct {
	ID          string
	CameraID    string
	Timestamp   time.Time
	ObjectClass string
}

// Update represents a Telegram update
type Update struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// Message represents a Telegram message (extended for command handling)
type TelegramMessage struct {
	MessageID int64            `json:"message_id"`
	From      *TelegramUser    `json:"from,omitempty"`
	Chat      *TelegramChat    `json:"chat,omitempty"`
	Date      int64            `json:"date"`
	Text      string           `json:"text,omitempty"`
	Entities  []MessageEntity  `json:"entities,omitempty"`
}

// TelegramUser represents a Telegram user
type TelegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat represents a Telegram chat
type TelegramChat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// MessageEntity represents a message entity (for detecting commands)
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}

// GetUpdatesResponse represents the response from getUpdates
type GetUpdatesResponse struct {
	OK          bool     `json:"ok"`
	Result      []Update `json:"result,omitempty"`
	ErrorCode   int      `json:"error_code,omitempty"`
	Description string   `json:"description,omitempty"`
}

// CommandHandler handles Telegram bot commands
type CommandHandler struct {
	bot            *TelegramBot
	cameraManager  *camera.CameraManager
	motionDetector MotionDetectorInterface
	db             *database.Database
	lastUpdateID   int64
	startTime      time.Time
	mu             sync.Mutex
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(
	bot *TelegramBot,
	cameraManager *camera.CameraManager,
	motionDetector MotionDetectorInterface,
	db *database.Database,
) *CommandHandler {
	return &CommandHandler{
		bot:            bot,
		cameraManager:  cameraManager,
		motionDetector: motionDetector,
		db:             db,
		lastUpdateID:   0,
		startTime:      time.Now(),
	}
}

// StartPolling starts the polling loop for Telegram updates
func (ch *CommandHandler) StartPolling(ctx context.Context) error {
	if !ch.bot.IsEnabled() {
		return fmt.Errorf("telegram bot is disabled")
	}

	ch.bot.mu.RLock()
	if ch.bot.botToken == "" {
		ch.bot.mu.RUnlock()
		return fmt.Errorf("telegram bot token not configured")
	}
	ch.bot.mu.RUnlock()

	fmt.Println("Starting Telegram command handler polling...")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Telegram command handler stopped")
			return nil
		case <-ticker.C:
			if err := ch.pollUpdates(ctx); err != nil {
				fmt.Printf("Warning: failed to poll Telegram updates: %v\n", err)
			}
		}
	}
}

// pollUpdates fetches and processes updates from Telegram
func (ch *CommandHandler) pollUpdates(ctx context.Context) error {
	ch.bot.mu.RLock()
	botToken := ch.bot.botToken
	authorizedChatID := ch.bot.chatID
	ch.bot.mu.RUnlock()

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=1", botToken, ch.lastUpdateID+1)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := ch.bot.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch updates: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var updatesResp GetUpdatesResponse
	if err := json.Unmarshal(body, &updatesResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !updatesResp.OK {
		return fmt.Errorf("telegram API error %d: %s", updatesResp.ErrorCode, updatesResp.Description)
	}

	for _, update := range updatesResp.Result {
		ch.mu.Lock()
		if update.UpdateID > ch.lastUpdateID {
			ch.lastUpdateID = update.UpdateID
		}
		ch.mu.Unlock()

		// Parse message from update
		if update.Message != nil {
			ch.handleMessage(ctx, update, authorizedChatID)
		}
	}

	return nil
}

// handleMessage processes an incoming message
func (ch *CommandHandler) handleMessage(ctx context.Context, update Update, authorizedChatID string) {
	msg := update.Message
	if msg == nil || msg.Chat == nil {
		return
	}

	// Security: Only respond to authorized chat
	chatIDStr := strconv.FormatInt(msg.Chat.ID, 10)
	if chatIDStr != authorizedChatID {
		fmt.Printf("Ignoring message from unauthorized chat: %s (authorized: %s)\n", chatIDStr, authorizedChatID)
		return
	}

	// Check if it's a command
	if msg.Text == "" || !strings.HasPrefix(msg.Text, "/") {
		return
	}

	fmt.Printf("Processing command: %s\n", msg.Text)

	// Parse command and arguments
	parts := strings.Fields(msg.Text)
	command := strings.ToLower(parts[0])
	args := parts[1:]

	// Remove bot username suffix if present (e.g., /status@mybot)
	if atIndex := strings.Index(command, "@"); atIndex != -1 {
		command = command[:atIndex]
	}

	// Dispatch command
	var response string
	switch command {
	case "/start":
		response = ch.handleStart()
	case "/help":
		response = ch.handleHelp()
	case "/status":
		response = ch.handleStatus()
	case "/cameras":
		response = ch.handleCameras()
	case "/start_detection":
		response = ch.handleStartDetection()
	case "/stop_detection":
		response = ch.handleStopDetection()
	case "/snapshot":
		ch.handleSnapshot(ctx, args)
		return // Snapshot sends photo directly
	case "/events":
		response = ch.handleEvents(args)
	case "/enable":
		response = ch.handleEnableCamera(args)
	case "/disable":
		response = ch.handleDisableCamera(args)
	case "/detect_on":
		response = ch.handleDetectOn(args)
	case "/detect_off":
		response = ch.handleDetectOff(args)
	default:
		response = fmt.Sprintf("Unknown command: %s\nUse /help to see available commands.", command)
	}

	// Send response
	if response != "" {
		if err := ch.sendReply(ctx, response); err != nil {
			fmt.Printf("Failed to send reply: %v\n", err)
		}
	}
}

// sendReply sends a reply message (without cooldown)
func (ch *CommandHandler) sendReply(ctx context.Context, message string) error {
	ch.bot.mu.RLock()
	defer ch.bot.mu.RUnlock()

	if ch.bot.botToken == "" || ch.bot.chatID == "" {
		return fmt.Errorf("telegram bot not configured")
	}

	payload := map[string]interface{}{
		"chat_id":    ch.bot.chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	return ch.bot.sendTelegramRequest(ctx, "sendMessage", payload)
}

// Command handlers

func (ch *CommandHandler) handleStart() string {
	return "🤖 <b>Welcome to Orbo!</b>\n\n" +
		"I'm your video alarm system bot. I'll notify you when objects are detected.\n\n" +
		"Use /help to see available commands."
}

func (ch *CommandHandler) handleHelp() string {
	return "📋 <b>Available Commands</b>\n\n" +
		"<b>System</b>\n" +
		"/status - System status\n" +
		"/cameras - List all cameras\n\n" +
		"<b>Camera Control</b>\n" +
		"/enable &lt;name&gt; - Activate camera (start streaming)\n" +
		"/disable &lt;name&gt; - Deactivate camera\n" +
		"/detect_on &lt;name&gt; - Enable AI detection for camera\n" +
		"/detect_off &lt;name&gt; - Disable AI detection (streaming only)\n\n" +
		"<b>Detection</b>\n" +
		"/start_detection - Start detection on all enabled cameras\n" +
		"/stop_detection - Stop all detection\n" +
		"/snapshot &lt;name&gt; - Capture frame from camera\n" +
		"/events [limit] - Show recent detection events\n\n" +
		"/help - Show this help"
}

func (ch *CommandHandler) handleStatus() string {
	cameras := ch.cameraManager.ListCameras()

	activeCount := 0
	detectingCount := 0
	detectionEnabledCount := 0
	for _, cam := range cameras {
		if cam.Status == "active" {
			activeCount++
			if cam.DetectionEnabled {
				detectionEnabledCount++
			}
		}
		if ch.motionDetector.IsDetectionRunning(cam.ID) {
			detectingCount++
		}
	}

	uptime := time.Since(ch.startTime)
	uptimeStr := formatDuration(uptime)

	telegramStatus := "Enabled"
	if !ch.bot.IsEnabled() {
		telegramStatus = "Disabled"
	}

	return fmt.Sprintf(
		"📊 <b>System Status</b>\n\n"+
			"📹 Cameras: %d total, %d active\n"+
			"🔍 Detection: %d/%d cameras (running/enabled)\n"+
			"📱 Telegram: %s\n"+
			"⏱️ Uptime: %s",
		len(cameras), activeCount,
		detectingCount, detectionEnabledCount,
		telegramStatus,
		uptimeStr,
	)
}

func (ch *CommandHandler) handleCameras() string {
	cameras := ch.cameraManager.ListCameras()

	if len(cameras) == 0 {
		return "📹 <b>Cameras</b>\n\nNo cameras configured."
	}

	var sb strings.Builder
	sb.WriteString("📹 <b>Cameras</b>\n\n")

	for _, cam := range cameras {
		statusIcon := "⚪"
		switch cam.Status {
		case "active":
			statusIcon = "🟢"
		case "error":
			statusIcon = "🔴"
		}

		// Detection status indicators
		detectionStatus := ""
		if cam.Status == "active" {
			if ch.motionDetector.IsDetectionRunning(cam.ID) {
				detectionStatus = " 👁️" // Currently detecting
			} else if cam.DetectionEnabled {
				detectionStatus = " 🔍" // Detection enabled but not running
			} else {
				detectionStatus = " 📺" // Streaming only
			}
		}

		sb.WriteString(fmt.Sprintf("%s <b>%s</b>%s\n", statusIcon, cam.Name, detectionStatus))
		sb.WriteString(fmt.Sprintf("   Device: %s\n", cam.Device))
	}

	sb.WriteString("\n👁️ = detecting | 🔍 = detection enabled | 📺 = streaming only")

	return sb.String()
}

func (ch *CommandHandler) handleStartDetection() string {
	cameras := ch.cameraManager.ListCameras()

	started := 0
	alreadyRunning := 0
	skippedDisabled := 0
	errors := 0

	for _, cam := range cameras {
		if cam.Status != "active" {
			continue
		}

		// Skip cameras with detection disabled
		if !cam.DetectionEnabled {
			skippedDisabled++
			continue
		}

		if ch.motionDetector.IsDetectionRunning(cam.ID) {
			alreadyRunning++
			continue
		}

		if err := ch.motionDetector.StartDetection(cam.ID, cam.Device); err != nil {
			fmt.Printf("Failed to start detection on %s: %v\n", cam.Name, err)
			errors++
		} else {
			started++
		}
	}

	if started == 0 && alreadyRunning == 0 && skippedDisabled == 0 {
		return "⚠️ No active cameras to start detection on."
	}

	response := fmt.Sprintf(
		"🔍 <b>Detection Started</b>\n\n"+
			"✅ Started: %d cameras\n"+
			"⏭️ Already running: %d",
		started, alreadyRunning,
	)

	if skippedDisabled > 0 {
		response += fmt.Sprintf("\n📺 Streaming only: %d", skippedDisabled)
	}

	if errors > 0 {
		response += fmt.Sprintf("\n❌ Errors: %d", errors)
	}

	return response
}

func (ch *CommandHandler) handleStopDetection() string {
	cameras := ch.cameraManager.ListCameras()

	stopped := 0
	for _, cam := range cameras {
		if ch.motionDetector.IsDetectionRunning(cam.ID) {
			ch.motionDetector.StopDetection(cam.ID)
			stopped++
		}
	}

	if stopped == 0 {
		return "ℹ️ No detection was running."
	}

	return fmt.Sprintf("🛑 <b>Detection Stopped</b>\n\nStopped detection on %d cameras.", stopped)
}

func (ch *CommandHandler) handleSnapshot(ctx context.Context, args []string) {
	if len(args) == 0 {
		ch.sendReply(ctx, "⚠️ Usage: /snapshot &lt;camera_name&gt;\n\nUse /cameras to see available cameras.")
		return
	}

	cameraName := strings.Join(args, " ")
	targetCamera, err := ch.findCameraByNameOrID(cameraName)
	if err != nil {
		ch.sendReply(ctx, err.Error())
		return
	}

	if targetCamera.Status != "active" {
		ch.sendReply(ctx, fmt.Sprintf("⚠️ Camera '%s' is not active.", targetCamera.Name))
		return
	}

	// Capture frame
	frameData, err := targetCamera.CaptureFrame()
	if err != nil {
		ch.sendReply(ctx, fmt.Sprintf("❌ Failed to capture frame: %v", err))
		return
	}

	// Format timestamp
	now := time.Now()
	zoneName, _ := now.Zone()
	timestamp := fmt.Sprintf("%s %s", now.Format("Jan 2, 2006, 03:04:05 PM"), zoneName)

	caption := fmt.Sprintf("📸 <b>Snapshot</b>\n\n📹 Camera: %s\n🕐 Time: %s", targetCamera.Name, timestamp)

	// Send photo
	if err := ch.bot.SendPhoto(ctx, frameData, caption); err != nil {
		ch.sendReply(ctx, fmt.Sprintf("❌ Failed to send snapshot: %v", err))
	}
}

func (ch *CommandHandler) handleEvents(args []string) string {
	limit := 5
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}

	events := ch.motionDetector.GetEventsForTelegram("", nil, limit)

	if len(events) == 0 {
		return "📋 <b>Recent Events</b>\n\nNo motion events recorded."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 <b>Recent Events</b> (last %d)\n\n", len(events)))

	for i, event := range events {
		// Get camera name
		cameraName := event.CameraID
		if len(cameraName) > 8 {
			cameraName = cameraName[:8] + "..."
		}
		if cam, err := ch.cameraManager.GetCamera(event.CameraID); err == nil && cam != nil {
			cameraName = cam.Name
		}

		// Format time
		zoneName, _ := event.Timestamp.Zone()
		timeStr := fmt.Sprintf("%s %s", event.Timestamp.Format("Jan 2, 03:04 PM"), zoneName)

		// Detection info
		detectionInfo := ""
		if event.ObjectClass != "" {
			detectionInfo = fmt.Sprintf(" (%s)", event.ObjectClass)
		}

		sb.WriteString(fmt.Sprintf("%d. %s%s\n   📹 %s\n", i+1, timeStr, detectionInfo, cameraName))
	}

	return sb.String()
}

func (ch *CommandHandler) handleEnableCamera(args []string) string {
	if len(args) == 0 {
		return "⚠️ Usage: /enable &lt;camera_name&gt;\n\nUse /cameras to see available cameras."
	}

	cameraName := strings.Join(args, " ")
	targetCamera, err := ch.findCameraByNameOrID(cameraName)
	if err != nil {
		return err.Error()
	}

	if targetCamera.Status == "active" {
		return fmt.Sprintf("ℹ️ Camera '%s' is already enabled.", targetCamera.Name)
	}

	if err := ch.cameraManager.ActivateCamera(targetCamera.ID); err != nil {
		return fmt.Sprintf("❌ Failed to enable camera: %v", err)
	}

	return fmt.Sprintf("✅ Camera '%s' enabled.", targetCamera.Name)
}

func (ch *CommandHandler) handleDisableCamera(args []string) string {
	if len(args) == 0 {
		return "⚠️ Usage: /disable &lt;camera_name&gt;\n\nUse /cameras to see available cameras."
	}

	cameraName := strings.Join(args, " ")
	targetCamera, err := ch.findCameraByNameOrID(cameraName)
	if err != nil {
		return err.Error()
	}

	if targetCamera.Status != "active" {
		return fmt.Sprintf("ℹ️ Camera '%s' is already disabled.", targetCamera.Name)
	}

	// Stop detection if running
	if ch.motionDetector.IsDetectionRunning(targetCamera.ID) {
		ch.motionDetector.StopDetection(targetCamera.ID)
	}

	if err := ch.cameraManager.DeactivateCamera(targetCamera.ID); err != nil {
		return fmt.Sprintf("❌ Failed to disable camera: %v", err)
	}

	return fmt.Sprintf("🛑 Camera '%s' disabled.", targetCamera.Name)
}

func (ch *CommandHandler) handleDetectOn(args []string) string {
	if len(args) == 0 {
		return "⚠️ Usage: /detect_on &lt;camera_name&gt;\n\nUse /cameras to see available cameras."
	}

	cameraName := strings.Join(args, " ")
	targetCamera, err := ch.findCameraByNameOrID(cameraName)
	if err != nil {
		return err.Error()
	}

	if targetCamera.Status != "active" {
		return fmt.Sprintf("⚠️ Camera '%s' must be active first. Use /enable %s", targetCamera.Name, targetCamera.Name)
	}

	if targetCamera.DetectionEnabled {
		return fmt.Sprintf("ℹ️ Detection is already enabled for '%s'.", targetCamera.Name)
	}

	if err := ch.cameraManager.SetDetectionEnabled(targetCamera.ID, true); err != nil {
		return fmt.Sprintf("❌ Failed to enable detection: %v", err)
	}

	// Start detection immediately if any detection is already running
	response := fmt.Sprintf("👁️ Detection enabled for '%s'.", targetCamera.Name)

	// Check if we should auto-start detection (if detection is running on other cameras)
	anyRunning := false
	for _, cam := range ch.cameraManager.ListCameras() {
		if ch.motionDetector.IsDetectionRunning(cam.ID) {
			anyRunning = true
			break
		}
	}

	if anyRunning && !ch.motionDetector.IsDetectionRunning(targetCamera.ID) {
		if err := ch.motionDetector.StartDetection(targetCamera.ID, targetCamera.Device); err != nil {
			response += fmt.Sprintf("\n⚠️ Auto-start failed: %v", err)
		} else {
			response += "\n✅ Detection started automatically."
		}
	}

	return response
}

func (ch *CommandHandler) handleDetectOff(args []string) string {
	if len(args) == 0 {
		return "⚠️ Usage: /detect_off &lt;camera_name&gt;\n\nUse /cameras to see available cameras."
	}

	cameraName := strings.Join(args, " ")
	targetCamera, err := ch.findCameraByNameOrID(cameraName)
	if err != nil {
		return err.Error()
	}

	if !targetCamera.DetectionEnabled {
		return fmt.Sprintf("ℹ️ Detection is already disabled for '%s'.", targetCamera.Name)
	}

	// Stop detection if running
	wasRunning := ch.motionDetector.IsDetectionRunning(targetCamera.ID)
	if wasRunning {
		ch.motionDetector.StopDetection(targetCamera.ID)
	}

	if err := ch.cameraManager.SetDetectionEnabled(targetCamera.ID, false); err != nil {
		return fmt.Sprintf("❌ Failed to disable detection: %v", err)
	}

	response := fmt.Sprintf("📺 Detection disabled for '%s' (streaming only).", targetCamera.Name)
	if wasRunning {
		response += "\n🛑 Detection stopped."
	}

	return response
}

// Helper functions

// findCameraByNameOrID finds a camera by name or ID, handling duplicates
func (ch *CommandHandler) findCameraByNameOrID(nameOrID string) (*camera.Camera, error) {
	cameras := ch.cameraManager.ListCameras()

	// First, try exact ID match
	for _, cam := range cameras {
		if cam.ID == nameOrID {
			return cam, nil
		}
	}

	// Then try name match, checking for duplicates
	var matches []*camera.Camera
	for _, cam := range cameras {
		if strings.EqualFold(cam.Name, nameOrID) {
			matches = append(matches, cam)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("❌ Camera not found: %s\n\nUse /cameras to see available cameras.", nameOrID)
	}

	if len(matches) > 1 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("⚠️ Multiple cameras named '%s'. Use camera ID:\n\n", nameOrID))
		for _, cam := range matches {
			sb.WriteString(fmt.Sprintf("• %s\n", cam.ID))
		}
		return nil, fmt.Errorf(sb.String())
	}

	return matches[0], nil
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
