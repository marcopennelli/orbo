package motion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"orbo/internal/database"
	"orbo/internal/detection"
	"orbo/internal/pipeline"
	"orbo/internal/stream"
	"orbo/internal/telegram"
	"orbo/internal/ws"
)

// PipelineConfigProvider is a function that returns the effective detection config for a camera
// Returns nil if the camera doesn't have a pipeline configuration (use defaults)
type PipelineConfigProvider func(cameraID string) *pipeline.EffectiveConfig

// YOLOConfigProvider is a function that returns the current YOLO confidence threshold
type YOLOConfigProvider func() float32

// StreamDetector handles streaming motion detection
type StreamDetector struct {
	mu               sync.RWMutex
	events           map[string]*MotionEvent
	eventsByCamera   map[string][]*MotionEvent
	isRunning        map[string]bool
	stopChannels     map[string]chan struct{}
	streamProcesses  map[string]*exec.Cmd
	sensitivity      float32
	minMotionArea    int
	maxEvents        int
	frameDir         string
	backgroundFrames map[string]image.Image
	frameBuffers     map[string]chan []byte
	gpuDetector      *detection.GPUDetector        // GPU object detection (HTTP fallback)
	grpcDetector     *detection.GRPCDetector       // gRPC-based YOLO detection (preferred)
	dinov3Detector   *detection.DINOv3Detector     // DINOv3 AI-powered detection
	faceRecognizer   *detection.FaceRecognizer     // Face recognition (HTTP fallback)
	grpcFaceRecognizer *detection.GRPCFaceRecognizer // gRPC face recognition (preferred)
	db               *database.Database            // Database for event persistence
	telegramBot      *telegram.TelegramBot         // Telegram notifications
	wsHub            *ws.DetectionHub              // WebSocket hub for real-time broadcasting
	streamOverlay    stream.StreamOverlayProvider  // Stream overlay for drawing bounding boxes on MJPEG
	frameSeqCounters map[string]*uint64            // Per-camera frame sequence counters
	pipelineConfig   PipelineConfigProvider        // Function to get pipeline config for mode gating
	yoloConfig       YOLOConfigProvider            // Function to get YOLO confidence threshold
	frameProvider    pipeline.FrameProvider        // Unified frame provider (avoids duplicate HTTP polling)
}

// NewStreamDetector creates a new streaming motion detector
func NewStreamDetector(frameDir string, db *database.Database) *StreamDetector {
	// Get YOLO endpoint from environment or use default Kubernetes service name
	yoloEndpoint := os.Getenv("YOLO_SERVICE_ENDPOINT")
	if yoloEndpoint == "" {
		yoloEndpoint = os.Getenv("YOLO_ENDPOINT") // Helm chart uses this name
	}
	if yoloEndpoint == "" {
		yoloEndpoint = "http://orbo-yolo:8081"
	}

	// Get DINOv3 endpoint from environment or use default
	dinov3Endpoint := os.Getenv("DINOV3_SERVICE_ENDPOINT")
	if dinov3Endpoint == "" {
		dinov3Endpoint = os.Getenv("DINOV3_ENDPOINT") // Helm chart uses this name
	}
	if dinov3Endpoint == "" {
		dinov3Endpoint = "http://orbo-dinov3:8001"
	}

	// Get Face Recognition endpoint from environment or use default
	recognitionEndpoint := os.Getenv("RECOGNITION_SERVICE_ENDPOINT")
	if recognitionEndpoint == "" {
		recognitionEndpoint = "http://orbo-recognition:8082"
	}
	recognitionEnabled := os.Getenv("RECOGNITION_ENABLED") == "true"

	sd := &StreamDetector{
		events:           make(map[string]*MotionEvent),
		eventsByCamera:   make(map[string][]*MotionEvent),
		isRunning:        make(map[string]bool),
		stopChannels:     make(map[string]chan struct{}),
		streamProcesses:  make(map[string]*exec.Cmd),
		backgroundFrames: make(map[string]image.Image),
		frameBuffers:     make(map[string]chan []byte),
		frameSeqCounters: make(map[string]*uint64),
		sensitivity:      0.15, // More sensitive for real-time detection
		minMotionArea:    300,  // Smaller minimum area for faster detection
		maxEvents:        1000,
		frameDir:         frameDir,
		gpuDetector:      detection.NewGPUDetector(yoloEndpoint),
		dinov3Detector:   detection.NewDINOv3Detector(dinov3Endpoint),
		faceRecognizer: detection.NewFaceRecognizer(detection.FaceRecognizerConfig{
			Enabled:             recognitionEnabled,
			ServiceEndpoint:     recognitionEndpoint,
			SimilarityThreshold: 0.5,
		}),
		db: db,
	}

	// Load events from database on startup
	if db != nil {
		if err := sd.loadEventsFromDB(); err != nil {
			fmt.Printf("Warning: failed to load events from database: %v\n", err)
		}
	}

	return sd
}

// loadEventsFromDB loads recent events from the database
func (sd *StreamDetector) loadEventsFromDB() error {
	// Load last 24 hours of events
	since := time.Now().Add(-24 * time.Hour)
	records, err := sd.db.ListMotionEvents("", &since, sd.maxEvents)
	if err != nil {
		return err
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	for _, record := range records {
		// Convert database bounding boxes to internal format
		bboxes := make([]BoundingBox, len(record.BoundingBoxes))
		for i, bbox := range record.BoundingBoxes {
			bboxes[i] = BoundingBox{
				X:      bbox.X,
				Y:      bbox.Y,
				Width:  bbox.Width,
				Height: bbox.Height,
			}
		}

		event := &MotionEvent{
			ID:                 record.ID,
			CameraID:           record.CameraID,
			Timestamp:          record.Timestamp,
			Confidence:         float32(record.Confidence),
			BoundingBoxes:      bboxes,
			FramePath:          record.FramePath,
			NotificationSent:   record.NotificationSent,
			ObjectClass:        record.ObjectClass,
			ObjectConfidence:   float32(record.ObjectConfidence),
			ThreatLevel:        record.ThreatLevel,
			InferenceTimeMs:    float32(record.InferenceTimeMs),
			DetectionDevice:    record.DetectionDevice,
			FacesDetected:      record.FacesDetected,
			KnownIdentities:    record.KnownIdentities,
			UnknownFacesCount:  record.UnknownFacesCount,
			ForensicThumbnails: record.ForensicThumbnails,
		}

		sd.events[event.ID] = event
		sd.eventsByCamera[event.CameraID] = append(sd.eventsByCamera[event.CameraID], event)
	}

	fmt.Printf("Loaded %d events from database\n", len(records))
	return nil
}

// StartStreamingDetection starts continuous streaming motion detection for a camera
func (sd *StreamDetector) StartStreamingDetection(cameraID, cameraDevice string) error {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if sd.isRunning[cameraID] {
		return fmt.Errorf("streaming detection already running for camera %s", cameraID)
	}

	stopCh := make(chan struct{})
	sd.stopChannels[cameraID] = stopCh
	sd.isRunning[cameraID] = true
	sd.frameBuffers[cameraID] = make(chan []byte, 10) // Buffer up to 10 frames

	// Initialize empty events slice for this camera
	if sd.eventsByCamera[cameraID] == nil {
		sd.eventsByCamera[cameraID] = make([]*MotionEvent, 0)
	}

	// Start streaming goroutine
	go sd.streamFrames(cameraID, cameraDevice, stopCh)
	
	// Start motion analysis goroutine
	go sd.analyzeFrames(cameraID, stopCh)

	return nil
}

// StopStreamingDetection stops streaming motion detection for a camera
func (sd *StreamDetector) StopStreamingDetection(cameraID string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if !sd.isRunning[cameraID] {
		return
	}

	// Stop the streaming process
	if cmd, exists := sd.streamProcesses[cameraID]; exists {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		delete(sd.streamProcesses, cameraID)
	}

	// Close stop channel to signal goroutines to exit
	// Note: We don't close frameBuffer to avoid "send on closed channel" panics.
	// The producer goroutine will exit when it sees stopCh closed, and the
	// frameBuffer will be garbage collected when no goroutines reference it.
	if stopCh, exists := sd.stopChannels[cameraID]; exists {
		close(stopCh)
		delete(sd.stopChannels, cameraID)
	}

	// Remove frameBuffer reference - it will be GC'd when goroutines exit
	delete(sd.frameBuffers, cameraID)

	sd.isRunning[cameraID] = false
	delete(sd.backgroundFrames, cameraID)
}

// isNetworkSource checks if device is an HTTP/RTSP URL
func isNetworkSource(device string) bool {
	return strings.HasPrefix(device, "http://") ||
		strings.HasPrefix(device, "https://") ||
		strings.HasPrefix(device, "rtsp://")
}

// streamFrames continuously captures frames from the camera
func (sd *StreamDetector) streamFrames(cameraID, cameraDevice string, stopCh chan struct{}) {
	fmt.Printf("Started streaming motion detection for camera %s\n", cameraID)
	defer fmt.Printf("Stopped streaming motion detection for camera %s\n", cameraID)

	// Check if we have a frame provider - use it to avoid duplicate HTTP polling
	sd.mu.RLock()
	fp := sd.frameProvider
	sd.mu.RUnlock()

	// For HTTP image endpoints, prefer FrameProvider to avoid duplicate requests
	if isNetworkSource(cameraDevice) && (strings.Contains(cameraDevice, "image.jpg") || strings.Contains(cameraDevice, ".jpg") || strings.Contains(cameraDevice, ".jpeg")) {
		if fp != nil {
			// Use shared frame provider - subscribe to frames instead of polling
			sd.streamFromProvider(cameraID, fp, stopCh)
			return
		}
		// Fallback to direct polling if no frame provider
		sd.streamHTTPImages(cameraID, cameraDevice, stopCh)
		return
	}

	// Build ffmpeg command based on source type
	var args []string
	if isNetworkSource(cameraDevice) {
		// RTSP or MJPEG stream
		args = []string{
			"-i", cameraDevice,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-r", "5", // 5 FPS output
			"-q:v", "5",
			"-",
		}
	} else {
		// V4L2 device
		args = []string{
			"-f", "v4l2",
			"-video_size", "640x480",
			"-framerate", "10",
			"-i", cameraDevice,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "5",
			"-",
		}
	}

	cmd := exec.Command("ffmpeg", args...)

	// Store the process for cleanup
	sd.mu.Lock()
	sd.streamProcesses[cameraID] = cmd
	sd.mu.Unlock()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Error creating stdout pipe for camera %s: %v\n", cameraID, err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Error creating stderr pipe for camera %s: %v\n", cameraID, err)
		return
	}

	// Start the ffmpeg process
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting ffmpeg for camera %s: %v\n", cameraID, err)
		return
	}

	// Handle stderr in background
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			// Silently consume stderr to prevent blocking
		}
	}()

	// Capture frameBuffer channel reference to avoid accessing deleted map entry
	sd.mu.RLock()
	frameCh := sd.frameBuffers[cameraID]
	sd.mu.RUnlock()

	// Read frames from stdout
	go func() {
		defer stdout.Close()
		frameBuffer := make([]byte, 0, 1024*1024) // 1MB buffer

		for {
			select {
			case <-stopCh:
				return
			default:
				// Read frame data
				chunk := make([]byte, 8192)
				n, err := stdout.Read(chunk)
				if err != nil {
					if err != io.EOF {
						fmt.Printf("Error reading frame data for camera %s: %v\n", cameraID, err)
					}
					return
				}

				frameBuffer = append(frameBuffer, chunk[:n]...)

				// Look for JPEG frame boundaries (FFD8...FFD9)
				if frame := sd.extractJPEGFrame(&frameBuffer); frame != nil {
					select {
					case <-stopCh:
						return
					case frameCh <- frame:
					default:
						// Drop frame if buffer is full
					}
				}
			}
		}
	}()

	// Wait for the process to finish
	cmd.Wait()
}

// extractJPEGFrame extracts a complete JPEG frame from the buffer
func (sd *StreamDetector) extractJPEGFrame(buffer *[]byte) []byte {
	if len(*buffer) < 4 {
		return nil
	}

	// Look for JPEG start marker (FFD8)
	startIdx := -1
	for i := 0; i < len(*buffer)-1; i++ {
		if (*buffer)[i] == 0xFF && (*buffer)[i+1] == 0xD8 {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return nil
	}

	// Look for JPEG end marker (FFD9) after start
	endIdx := -1
	for i := startIdx + 2; i < len(*buffer)-1; i++ {
		if (*buffer)[i] == 0xFF && (*buffer)[i+1] == 0xD9 {
			endIdx = i + 2
			break
		}
	}

	if endIdx == -1 {
		return nil
	}

	// Extract the frame
	frame := make([]byte, endIdx-startIdx)
	copy(frame, (*buffer)[startIdx:endIdx])

	// Remove processed data from buffer
	*buffer = (*buffer)[endIdx:]

	return frame
}

// analyzeFrames continuously analyzes incoming frames for motion
func (sd *StreamDetector) analyzeFrames(cameraID string, stopCh chan struct{}) {
	var backgroundImg image.Image
	var lastBackgroundUpdate time.Time
	frameCount := 0

	sd.mu.RLock()
	frameBuffer := sd.frameBuffers[cameraID]
	sd.mu.RUnlock()

	if frameBuffer == nil {
		fmt.Printf("Error: No frame buffer for camera %s in analyzeFrames\n", cameraID)
		return
	}

	fmt.Printf("analyzeFrames started for camera %s, waiting for initial frame...\n", cameraID)

	// Capture initial background frame
	select {
	case frameData := <-frameBuffer:
		img, err := jpeg.Decode(bytes.NewReader(frameData))
		if err == nil {
			backgroundImg = img
			sd.mu.Lock()
			sd.backgroundFrames[cameraID] = backgroundImg
			sd.mu.Unlock()
			lastBackgroundUpdate = time.Now()
			fmt.Printf("Captured initial background frame for camera %s\n", cameraID)
		} else {
			fmt.Printf("Failed to decode initial frame for camera %s: %v\n", cameraID, err)
		}
	case <-time.After(10 * time.Second):
		fmt.Printf("Timeout waiting for initial background frame for camera %s\n", cameraID)
		return
	case <-stopCh:
		return
	}

	for {
		select {
		case <-stopCh:
			return
		case frameData := <-frameBuffer:
			frameCount++

			// Assign capture sequence number NOW (at capture time, not processing completion)
			// This is critical for frame ordering - YOLO processing is async and variable latency
			captureSeq := frameCount

			// Decode current frame
			currentImg, err := jpeg.Decode(bytes.NewReader(frameData))
			if err != nil {
				fmt.Printf("Failed to decode frame %d for camera %s: %v\n", frameCount, cameraID, err)
				continue
			}

			// Log frame processing periodically
			if frameCount%50 == 0 {
				fmt.Printf("analyzeFrames processed %d frames for camera %s\n", frameCount, cameraID)
			}

			// Broadcast frame via WebSocket for live streaming (if clients connected)
			if sd.wsHub != nil {
				hasClients := sd.wsHub.HasClients(cameraID)
				if frameCount%100 == 1 { // Log periodically
					registeredCameras := sd.wsHub.GetRegisteredCameras()
					fmt.Printf("[WS-DEBUG] Camera %s: hasClients=%v, frameCount=%d, registeredCameras=%v\n", cameraID, hasClients, frameCount, registeredCameras)
				}
				if hasClients {
					bounds := currentImg.Bounds()
					frameMsg := ws.NewFrameMessage(cameraID, bounds.Dx(), bounds.Dy(), base64.StdEncoding.EncodeToString(frameData))
					sd.wsHub.BroadcastFrame(cameraID, frameMsg)
				}
			} else if frameCount%100 == 1 {
				fmt.Printf("[WS-DEBUG] Camera %s: wsHub is nil!\n", cameraID)
			}

			// Update background periodically (every 30 seconds)
			if time.Since(lastBackgroundUpdate) > 30*time.Second {
				backgroundImg = currentImg
				sd.mu.Lock()
				sd.backgroundFrames[cameraID] = backgroundImg
				sd.mu.Unlock()
				lastBackgroundUpdate = time.Now()
				fmt.Printf("Updated background frame for camera %s\n", cameraID)
			}

			// Detection pipeline logic:
			// - disabled: No detection at all (streaming only)
			// - continuous/visual_only: Run YOLO/Face on EVERY frame (YOLO11-style)
			// - motion_triggered/hybrid: Run motion detection first, then YOLO/Face only when motion detected
			// - scheduled: Run YOLO/Face at intervals (not implemented here)

			// Check if detection is enabled for this camera
			if !sd.isDetectionEnabled(cameraID) {
				// Detection is disabled, skip all detectors - streaming only mode
				if frameCount%100 == 0 {
					fmt.Printf("[StreamDetector] Detection disabled for camera %s, streaming only\n", cameraID)
				}
				continue
			}

			// Check which detectors are enabled
			yoloEnabled := sd.isDetectorEnabled(cameraID, "yolo")
			faceEnabled := sd.isDetectorEnabled(cameraID, "face") && sd.hasFaceRecognizer()
			continuousMode := sd.isContinuousMode(cameraID)

			// Log pipeline config periodically
			if frameCount%300 == 1 {
				modeStr := "motion_triggered"
				if continuousMode {
					modeStr = "continuous"
				}
				fmt.Printf("[Pipeline] Camera %s: mode=%s, yolo=%v, face=%v\n",
					cameraID, modeStr, yoloEnabled, faceEnabled)
			}

			if continuousMode {
				// CONTINUOUS/VISUAL_ONLY MODE: Run detection on EVERY frame
				// This creates a smooth annotated stream (uniformly delayed, no time-travel)
				if yoloEnabled {
					isHealthy := sd.gpuDetector.IsHealthy()
					if isHealthy {
						if frameCount%30 == 0 { // Log every 30 frames to reduce noise
							fmt.Printf("Running YOLO detection for camera %s (frame %d, seq %d)\n", cameraID, frameCount, captureSeq)
						}
						sd.runDirectYOLODetectionWithSeq(cameraID, frameData, uint64(captureSeq))
					} else if frameCount%60 == 0 {
						fmt.Printf("YOLO detector not healthy for camera %s (frame %d)\n", cameraID, frameCount)
					}
				} else if faceEnabled {
					// Face-only mode in continuous
					if frameCount%30 == 0 {
						fmt.Printf("Running Face detection for camera %s (frame %d, seq %d)\n", cameraID, frameCount, captureSeq)
					}
					sd.runDirectFaceDetection(cameraID, frameData, uint64(captureSeq))
				} else if frameCount%100 == 0 {
					fmt.Printf("[StreamDetector] No detectors enabled for camera %s (continuous mode)\n", cameraID)
				}
			} else {
				// MOTION-TRIGGERED MODE: Run motion detection first, then YOLO/Face only on motion
				// This is more efficient for motion_triggered, hybrid, scheduled modes
				if backgroundImg != nil {
					hasMotion := sd.detectMotion(cameraID, backgroundImg, currentImg)
					if hasMotion {
						// Motion detected - run AI detection
						if yoloEnabled {
							// Check gRPC detector first, then HTTP fallback
							sd.mu.RLock()
							grpcDet := sd.grpcDetector
							sd.mu.RUnlock()

							grpcHealthy := grpcDet != nil && grpcDet.IsHealthy()
							httpHealthy := sd.gpuDetector.IsHealthy()

							if grpcHealthy {
								fmt.Printf("[Motion-Triggered] Running YOLO (gRPC) for camera %s (frame %d)\n", cameraID, frameCount)
								sd.runDirectYOLODetectionWithSeq(cameraID, frameData, uint64(captureSeq))
							} else if httpHealthy {
								fmt.Printf("[Motion-Triggered] Running YOLO (HTTP) for camera %s (frame %d)\n", cameraID, frameCount)
								sd.runDirectYOLODetectionWithSeq(cameraID, frameData, uint64(captureSeq))
							} else if frameCount%60 == 0 {
								fmt.Printf("[Motion-Triggered] YOLO not healthy (gRPC=%v, HTTP=%v) for camera %s\n",
									grpcHealthy, httpHealthy, cameraID)
							}
						} else if faceEnabled {
							fmt.Printf("[Motion-Triggered] Running Face for camera %s (frame %d)\n", cameraID, frameCount)
							sd.runDirectFaceDetection(cameraID, frameData, uint64(captureSeq))
						} else {
							// Basic motion event (no AI)
							fmt.Printf("[Motion-Triggered] Basic motion event for camera %s (no AI detectors)\n", cameraID)
							sd.createMotionEvent(cameraID, frameData, backgroundImg, currentImg)
						}
					}
				} else if frameCount%100 == 1 {
					fmt.Printf("[Motion-Triggered] Waiting for background frame for camera %s\n", cameraID)
				}
			}
		}
	}
}

// runDirectYOLODetection runs YOLO detection directly on a frame without requiring motion first
func (sd *StreamDetector) runDirectYOLODetection(cameraID string, frameData []byte) {
	// Check which detector to use (prefer gRPC for low latency)
	sd.mu.RLock()
	grpcDet := sd.grpcDetector
	drawBoxes := sd.gpuDetector.DrawBoxesEnabled()
	sd.mu.RUnlock()

	// If draw boxes is enabled, use annotated detection
	if drawBoxes || (grpcDet != nil && grpcDet.DrawBoxesEnabled()) {
		sd.runDirectYOLODetectionAnnotated(cameraID, frameData)
		return
	}

	// Get configured confidence threshold
	confThreshold := sd.getYOLOConfThreshold()

	// Prefer gRPC detector for low-latency streaming
	var securityResult *detection.SecurityDetectionResult
	var err error
	if grpcDet != nil && grpcDet.IsHealthy() {
		securityResult, err = grpcDet.DetectSecurityObjects(frameData, confThreshold)
		if err != nil {
			fmt.Printf("gRPC YOLO detection failed for camera %s: %v, falling back to HTTP\n", cameraID, err)
			// Fall back to HTTP
			securityResult, err = sd.gpuDetector.DetectSecurityObjects(frameData, confThreshold)
		}
	} else {
		// Use HTTP detector
		securityResult, err = sd.gpuDetector.DetectSecurityObjects(frameData, confThreshold)
	}
	if err != nil {
		fmt.Printf("Direct YOLO detection failed for camera %s: %v\n", cameraID, err)
		return
	}

	if len(securityResult.Detections) > 0 {
		fmt.Printf("YOLO detected %d objects on camera %s (inference: %.1fms on %s)\n",
			len(securityResult.Detections), cameraID, securityResult.InferenceTimeMs, securityResult.Device)

		// Update stream overlay for MJPEG stream bounding boxes
		sd.updateStreamOverlay(cameraID, securityResult.Detections, nil)

		// Broadcast detections via WebSocket if hub is available
		if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
			// Get frame dimensions
			img, err := jpeg.Decode(bytes.NewReader(frameData))
			frameWidth, frameHeight := 0, 0
			if err == nil {
				bounds := img.Bounds()
				frameWidth = bounds.Dx()
				frameHeight = bounds.Dy()
			}

			wsMsg := ws.NewDetectionMessage(cameraID, frameWidth, frameHeight)
			for _, det := range securityResult.Detections {
				// Convert bbox from [x1, y1, x2, y2] to [x, y, w, h] format for frontend
				bbox := det.BBox
				if len(bbox) == 4 {
					bbox = []float32{bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]}
				}
				wsMsg.AddObject(det.Class, det.Confidence, bbox, sd.gpuDetector.GetThreatLevel(det))
			}
			// Include the frame for sync with bounding boxes
			wsMsg.SetFrame(base64.StdEncoding.EncodeToString(frameData))
			sd.wsHub.BroadcastDetection(cameraID, wsMsg)
		}

		// Create events for each security-relevant detection
		for _, det := range securityResult.Detections {
			if sd.gpuDetector.ShouldAlert(det) {
				// Save frame for this detection
				framePath := fmt.Sprintf("%s/yolo_direct_%s_%s_%d.jpg",
					sd.frameDir, cameraID, det.Class, time.Now().UnixNano())

				if err := sd.saveFrameBytes(frameData, framePath); err != nil {
					fmt.Printf("Failed to save YOLO detection frame: %v\n", err)
					framePath = ""
				}

				// Convert detection bbox to our format
				bbox := BoundingBox{
					X:      int(det.BBox[0]),
					Y:      int(det.BBox[1]),
					Width:  int(det.BBox[2] - det.BBox[0]),
					Height: int(det.BBox[3] - det.BBox[1]),
				}

				// Create motion event
				event := &MotionEvent{
					ID:               uuid.New().String(),
					CameraID:         cameraID,
					Timestamp:        time.Now(),
					Confidence:       det.Confidence,
					BoundingBoxes:    []BoundingBox{bbox},
					FramePath:        framePath,
					ObjectClass:      det.Class,
					ObjectConfidence: det.Confidence,
					ThreatLevel:      sd.gpuDetector.GetThreatLevel(det),
					InferenceTimeMs:  securityResult.InferenceTimeMs,
					DetectionDevice:  securityResult.Device,
				}

				// If person detected, try face recognition
				if det.Class == "person" {
					sd.performFaceRecognition(cameraID, frameData, event)
				}

				sd.addEvent(event)
				fmt.Printf("YOLO direct detection: %s on camera %s (confidence: %.2f, threat: %s)\n",
					det.Class, cameraID, det.Confidence, event.ThreatLevel)
			}
		}
	}
}

// runDirectYOLODetectionAnnotated runs YOLO detection and saves annotated image with bounding boxes
func (sd *StreamDetector) runDirectYOLODetectionAnnotated(cameraID string, frameData []byte) {
	sd.runDirectYOLODetectionAnnotatedWithSeq(cameraID, frameData, 0)
}

// runDirectYOLODetectionAnnotatedWithSeq runs YOLO detection with capture sequence for frame ordering
func (sd *StreamDetector) runDirectYOLODetectionAnnotatedWithSeq(cameraID string, frameData []byte, captureSeq uint64) {
	// Prefer gRPC detector for low-latency streaming
	sd.mu.RLock()
	grpcDet := sd.grpcDetector
	sd.mu.RUnlock()

	// Get configured confidence threshold
	confThreshold := sd.getYOLOConfThreshold()

	var annotatedResult *detection.AnnotatedSecurityResult
	var err error
	if grpcDet != nil && grpcDet.IsHealthy() {
		annotatedResult, err = grpcDet.DetectSecurityObjectsAnnotated(frameData, confThreshold)
		if err != nil {
			fmt.Printf("gRPC YOLO annotated detection failed for camera %s: %v, falling back to HTTP\n", cameraID, err)
			// Fall back to HTTP
			annotatedResult, err = sd.gpuDetector.DetectSecurityObjectsAnnotated(frameData, confThreshold)
		}
	} else {
		// Use HTTP detector
		annotatedResult, err = sd.gpuDetector.DetectSecurityObjectsAnnotated(frameData, confThreshold)
	}
	if err != nil {
		fmt.Printf("Direct YOLO annotated detection failed for camera %s: %v\n", cameraID, err)
		return
	}

	// Simplified pipeline logic:
	// - If YOLO-only: YOLO sends ALL frames to stream (with or without detections)
	// - If YOLO+Face: YOLO does NOT send to stream, Face sends ALL frames
	// This ensures consistent frame rate and no time-travel effect
	faceDetectorEnabled := sd.isDetectorEnabled(cameraID, "face") && sd.hasFaceRecognizer()

	// Log YOLO result
	fmt.Printf("[YOLO] Camera %s: detected %d objects (%.1fms on %s), face_next=%v\n",
		cameraID, annotatedResult.Count, annotatedResult.InferenceTimeMs, annotatedResult.Device, faceDetectorEnabled)

	// YOLO sends to stream ONLY if Face is NOT enabled (YOLO-only mode)
	// When Face is enabled, Face will send ALL frames (including ones without persons)
	if len(annotatedResult.ImageData) > 0 && !faceDetectorEnabled {
		if captureSeq > 0 {
			sd.sendAnnotatedFrameToStreamWithSeq(cameraID, annotatedResult.ImageData, captureSeq)
		} else {
			sd.sendAnnotatedFrameToStream(cameraID, annotatedResult.ImageData)
		}
	}

	// When Face detector is enabled (YOLO+Face mode), ALWAYS pass frame to Face
	// Face will send ALL frames to stream (maintaining consistent frame rate)
	if faceDetectorEnabled {
		// Pass the YOLO-annotated frame to face recognition so face boxes
		// are drawn ON TOP OF the YOLO boxes (combined visualization)
		frameForFaceRecognition := frameData
		if len(annotatedResult.ImageData) > 0 {
			frameForFaceRecognition = annotatedResult.ImageData
		}
		// performFaceRecognitionForStream always sends to stream (simplified pipeline)
		sd.performFaceRecognitionForStream(cameraID, frameForFaceRecognition, captureSeq, annotatedResult)
		return // Face handles stream output in YOLO+Face mode
	}

	// YOLO-only mode: process detections and create events
	if annotatedResult.Count > 0 {
		fmt.Printf("YOLO detected %d objects on camera %s (inference: %.1fms on %s) - saving annotated image\n",
			annotatedResult.Count, cameraID, annotatedResult.InferenceTimeMs, annotatedResult.Device)

		// Also update stream overlay for metadata (fallback if annotated frame not used)
		sd.updateStreamOverlay(cameraID, annotatedResult.Detections, nil)

		// Broadcast detections via WebSocket if hub is available
		if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
			// Get frame dimensions from original frame
			img, err := jpeg.Decode(bytes.NewReader(frameData))
			frameWidth, frameHeight := 0, 0
			if err == nil {
				bounds := img.Bounds()
				frameWidth = bounds.Dx()
				frameHeight = bounds.Dy()
			}

			wsMsg := ws.NewDetectionMessage(cameraID, frameWidth, frameHeight)
			for _, det := range annotatedResult.Detections {
				// Convert bbox from [x1, y1, x2, y2] to [x, y, w, h] format for frontend
				bbox := det.BBox
				if len(bbox) == 4 {
					bbox = []float32{bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]}
				}
				wsMsg.AddObject(det.Class, det.Confidence, bbox, sd.gpuDetector.GetThreatLevel(det))
			}
			// Include the frame for sync with bounding boxes
			wsMsg.SetFrame(base64.StdEncoding.EncodeToString(frameData))
			sd.wsHub.BroadcastDetection(cameraID, wsMsg)
		}

		// Save the annotated frame (with bounding boxes already drawn)
		framePath := fmt.Sprintf("%s/yolo_annotated_%s_%d.jpg",
			sd.frameDir, cameraID, time.Now().UnixNano())

		if err := sd.saveFrameBytes(annotatedResult.ImageData, framePath); err != nil {
			fmt.Printf("Failed to save YOLO annotated frame: %v\n", err)
			framePath = ""
		}

		// Find the best detection (highest confidence) for event metadata
		var bestClass string
		var bestConfidence float32
		var threatLevel string

		if len(annotatedResult.Detections) > 0 {
			// Use the first (usually highest confidence) detection
			bestDet := annotatedResult.Detections[0]
			bestClass = bestDet.Class
			bestConfidence = bestDet.Confidence

			// Find highest confidence among all detections
			for _, det := range annotatedResult.Detections {
				if det.Confidence > bestConfidence {
					bestConfidence = det.Confidence
					bestClass = det.Class
				}
			}

			// Determine threat level based on detected class
			threatLevel = sd.gpuDetector.GetThreatLevel(bestDet)
		} else {
			bestClass = "unknown"
			bestConfidence = 0.5
			threatLevel = "medium"
		}

		// Create event with actual detection data
		event := &MotionEvent{
			ID:               uuid.New().String(),
			CameraID:         cameraID,
			Timestamp:        time.Now(),
			Confidence:       bestConfidence,
			BoundingBoxes:    []BoundingBox{}, // Boxes are drawn on image, not needed here
			FramePath:        framePath,
			ObjectClass:      bestClass,
			ObjectConfidence: bestConfidence,
			ThreatLevel:      threatLevel,
			InferenceTimeMs:  annotatedResult.InferenceTimeMs,
			DetectionDevice:  annotatedResult.Device,
		}

		sd.addEvent(event)
		fmt.Printf("YOLO annotated detection saved for camera %s: %s (%.0f%% confidence, threat: %s)\n",
			cameraID, bestClass, bestConfidence*100, threatLevel)
	}
}

// processMotionWithDINOv3 processes motion using DINOv3 with GPU and basic fallbacks
func (sd *StreamDetector) processMotionWithDINOv3(cameraID string, frameData []byte, backgroundImg, currentImg image.Image) {
	// Try DINOv3 detection first (most advanced)
	if sd.dinov3Detector.IsHealthy() {
		dinov3Result, err := sd.dinov3Detector.DetectMotion(frameData, cameraID, 0.85)
		if err != nil {
			fmt.Printf("DINOv3 detection failed, falling back to GPU: %v\n", err)
			sd.fallbackToBasicMotion(cameraID, frameData, backgroundImg, currentImg)
			return
		}

		// Process DINOv3 results
		if dinov3Result.MotionDetected {
			sd.processDINOv3Detection(cameraID, frameData, dinov3Result)
		}
		return
	}

	// Fallback to basic motion detection if DINOv3 unavailable
	sd.fallbackToBasicMotion(cameraID, frameData, backgroundImg, currentImg)
}

// fallbackToBasicMotion handles fallback to GPU + basic motion detection
func (sd *StreamDetector) fallbackToBasicMotion(cameraID string, frameData []byte, backgroundImg, currentImg image.Image) {
	// Use traditional motion detection
	if motionDetected, motionConfidence, motionBBox := sd.compareFrames(backgroundImg, currentImg); motionDetected {
		fmt.Printf("Motion detected on camera %s! Confidence: %.3f, Area: %dx%d\n",
			cameraID, motionConfidence, motionBBox.Width, motionBBox.Height)
		// Motion detected! Only use GPU if YOLO detector is enabled
		if sd.isDetectorEnabled(cameraID, "yolo") {
			sd.processMotionWithGPU(cameraID, frameData, motionConfidence, motionBBox)
		}
	}
}

// processDINOv3Detection processes DINOv3 detection results
func (sd *StreamDetector) processDINOv3Detection(cameraID string, frameData []byte, result *detection.DINOv3MotionResult) {
	// Save frame for this detection
	framePath := fmt.Sprintf("%s/dinov3_motion_%s_%s_%d.jpg", 
		sd.frameDir, cameraID, result.SceneAnalysis.SceneType, time.Now().UnixNano())
	
	if err := sd.saveFrameBytes(frameData, framePath); err != nil {
		fmt.Printf("Failed to save DINOv3 detection frame: %v\n", err)
		framePath = ""
	}

	// Determine bounding box from change regions or use full frame
	var bbox BoundingBox
	if len(result.ChangeRegions) > 0 {
		// Use first change region as primary detection
		region := result.ChangeRegions[0]
		if len(region.BBox) >= 4 {
			bbox = BoundingBox{
				X:      int(region.BBox[0]),
				Y:      int(region.BBox[1]),
				Width:  int(region.BBox[2] - region.BBox[0]),
				Height: int(region.BBox[3] - region.BBox[1]),
			}
		} else {
			// Full frame fallback
			bbox = BoundingBox{X: 0, Y: 0, Width: 640, Height: 480}
		}
	} else {
		// Full frame fallback
		bbox = BoundingBox{X: 0, Y: 0, Width: 640, Height: 480}
	}

	// Create enhanced motion event with DINOv3 insights
	event := &MotionEvent{
		ID:               uuid.New().String(),
		CameraID:         cameraID,
		Timestamp:        time.Now(),
		Confidence:       result.Confidence,
		BoundingBoxes:    []BoundingBox{bbox},
		FramePath:        framePath,
		// AI-enhanced fields from DINOv3
		ObjectClass:      sd.dinov3Detector.GetMotionType(result),
		ObjectConfidence: result.Confidence,
		ThreatLevel:      sd.dinov3Detector.GetThreatLevel(result),
		InferenceTimeMs:  result.InferenceTimeMs,
		DetectionDevice:  result.Device,
	}

	sd.addEvent(event)
	fmt.Printf("DINOv3-enhanced detection: %s on camera %s (confidence: %.2f, threat: %s, similarity: %.3f, inference: %.1fms)\n",
		sd.dinov3Detector.GetMotionType(result), cameraID, result.Confidence, event.ThreatLevel, 
		result.FeatureSimilarity, result.InferenceTimeMs)
}

// processMotionWithGPU processes detected motion using GPU-accelerated object detection
func (sd *StreamDetector) processMotionWithGPU(cameraID string, frameData []byte, motionConfidence float32, motionBBox BoundingBox) {
	// Get configured confidence threshold
	confThreshold := sd.getYOLOConfThreshold()

	// Try GPU detection first
	if sd.gpuDetector.IsHealthy() {
		securityResult, err := sd.gpuDetector.DetectSecurityObjects(frameData, confThreshold)
		if err != nil {
			fmt.Printf("GPU detection failed, falling back to basic motion: %v\n", err)
			sd.createBasicMotionEvent(cameraID, frameData, motionConfidence, motionBBox)
			return
		}

		// Process GPU detections
		if len(securityResult.Detections) > 0 {
			sd.processGPUDetections(cameraID, frameData, securityResult, motionBBox)
		} else {
			// Motion detected but no security objects found
			fmt.Printf("Motion detected on camera %s but no security objects identified (inference: %.2fms)\n", 
				cameraID, securityResult.InferenceTimeMs)
		}
	} else {
		// GPU not available, fallback to basic motion detection
		sd.createBasicMotionEvent(cameraID, frameData, motionConfidence, motionBBox)
	}
}

// processGPUDetections creates motion events for each detected object
func (sd *StreamDetector) processGPUDetections(cameraID string, frameData []byte, result *detection.SecurityDetectionResult, motionBBox BoundingBox) {
	// Update stream overlay for MJPEG stream bounding boxes
	sd.updateStreamOverlay(cameraID, result.Detections, nil)

	for _, det := range result.Detections {
		if sd.gpuDetector.ShouldAlert(det) {
			// Save frame for this detection
			framePath := fmt.Sprintf("%s/gpu_motion_%s_%s_%d.jpg", 
				sd.frameDir, cameraID, det.Class, time.Now().UnixNano())
			
			if err := sd.saveFrameBytes(frameData, framePath); err != nil {
				fmt.Printf("Failed to save GPU detection frame: %v\n", err)
				framePath = ""
			}

			// Convert detection bbox to our format
			bbox := BoundingBox{
				X:      int(det.BBox[0]),
				Y:      int(det.BBox[1]),
				Width:  int(det.BBox[2] - det.BBox[0]),
				Height: int(det.BBox[3] - det.BBox[1]),
			}

			// Create enhanced motion event
			event := &MotionEvent{
				ID:               uuid.New().String(),
				CameraID:         cameraID,
				Timestamp:        time.Now(),
				Confidence:       float32(motionBBox.Width * motionBBox.Height) / (640.0 * 480.0), // normalized area
				BoundingBoxes:    []BoundingBox{bbox},
				FramePath:        framePath,
				// AI-enhanced fields
				ObjectClass:      det.Class,
				ObjectConfidence: det.Confidence,
				ThreatLevel:      sd.gpuDetector.GetThreatLevel(det),
				InferenceTimeMs:  result.InferenceTimeMs,
				DetectionDevice:  result.Device,
			}

			sd.addEvent(event)
			fmt.Printf("GPU-enhanced detection: %s on camera %s (confidence: %.2f, threat: %s, inference: %.1fms on %s)\n",
				det.Class, cameraID, det.Confidence, event.ThreatLevel, result.InferenceTimeMs, result.Device)
		}
	}
}

// createBasicMotionEvent creates a basic motion event (fallback when GPU unavailable)
func (sd *StreamDetector) createBasicMotionEvent(cameraID string, frameData []byte, confidence float32, bbox BoundingBox) {
	framePath := fmt.Sprintf("%s/basic_motion_%s_%d.jpg", sd.frameDir, cameraID, time.Now().Unix())
	
	if err := sd.saveFrameBytes(frameData, framePath); err != nil {
		fmt.Printf("Failed to save basic motion frame: %v\n", err)
		framePath = ""
	}

	event := &MotionEvent{
		ID:            uuid.New().String(),
		CameraID:      cameraID,
		Timestamp:     time.Now(),
		Confidence:    confidence,
		BoundingBoxes: []BoundingBox{bbox},
		FramePath:     framePath,
		ObjectClass:   "unknown", // Basic motion detection
		ThreatLevel:   "unknown",
	}

	sd.addEvent(event)
	fmt.Printf("Basic motion detected on camera %s with confidence %.2f\n", cameraID, confidence)
}

// performFaceRecognition runs face recognition on a frame and updates the event with results
func (sd *StreamDetector) performFaceRecognition(cameraID string, frameData []byte, event *MotionEvent) {
	// Prefer gRPC for low-latency streaming
	sd.mu.RLock()
	grpcFace := sd.grpcFaceRecognizer
	httpFace := sd.faceRecognizer
	sd.mu.RUnlock()

	// Check if any face recognition is available
	grpcAvailable := grpcFace != nil && grpcFace.IsEnabled() && grpcFace.IsHealthy()
	httpAvailable := httpFace != nil && httpFace.IsEnabled()

	if !grpcAvailable && !httpAvailable {
		return
	}

	// Try gRPC first for low latency
	var annotatedResult *detection.AnnotatedRecognitionResult
	var err error

	if grpcAvailable {
		annotatedResult, err = grpcFace.RecognizeFacesAnnotatedGRPC(frameData)
		if err != nil {
			fmt.Printf("gRPC face recognition failed for camera %s: %v, falling back to HTTP\n", cameraID, err)
			grpcAvailable = false
		}
	}

	// Fall back to HTTP if gRPC failed or unavailable
	if !grpcAvailable && httpAvailable {
		// Check if HTTP service is healthy
		if !httpFace.IsHealthy() {
			if err := httpFace.CheckHealth(); err != nil {
				fmt.Printf("Face recognition service unhealthy for camera %s: %v\n", cameraID, err)
				return
			}
		}
		annotatedResult, err = httpFace.RecognizeFacesAnnotated(frameData)
		if err != nil {
			fmt.Printf("HTTP face recognition failed for camera %s: %v\n", cameraID, err)
			sd.performFaceRecognitionFallback(cameraID, frameData, event)
			return
		}
	}

	if annotatedResult == nil {
		return
	}

	// Update event with face recognition results
	if annotatedResult.Count > 0 {
		event.FacesDetected = annotatedResult.Count
		event.UnknownFacesCount = annotatedResult.UnknownCount

		// Extract known identities
		for _, face := range annotatedResult.Recognitions {
			if face.IsKnown && face.Identity != nil {
				event.KnownIdentities = append(event.KnownIdentities, *face.Identity)
			}
		}

		// Log the results
		fmt.Printf("Face recognition for camera %s: total=%d, known=%d, unknown=%d (inference: %.1fms)\n",
			cameraID, annotatedResult.Count, annotatedResult.KnownCount, annotatedResult.UnknownCount, annotatedResult.InferenceTimeMs)

		// Update threat level if unknown faces detected
		if annotatedResult.UnknownCount > 0 && event.ThreatLevel != "high" {
			event.ThreatLevel = "high" // Unknown person = high threat
		}

		// Send annotated frame with face boxes to MJPEG stream
		if len(annotatedResult.ImageData) > 0 {
			sd.sendAnnotatedFrameToStream(cameraID, annotatedResult.ImageData)
		}

		// Broadcast face recognition results via WebSocket
		if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
			wsMsg := ws.NewFaceMessage(cameraID)
			for _, face := range annotatedResult.Recognitions {
				var identity *string
				var similarity *float32
				var age *int
				var gender *string

				if face.Identity != nil {
					identity = face.Identity
				}
				if face.Similarity > 0 {
					sim := face.Similarity
					similarity = &sim
				}
				if face.Age > 0 {
					a := face.Age
					age = &a
				}
				if face.Gender != "" {
					g := face.Gender
					gender = &g
				}

				// Convert bbox from [x1, y1, x2, y2] to [x, y, w, h] format for frontend
				bbox := face.BBox
				if len(bbox) == 4 {
					bbox = []float32{bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]}
				}
				wsMsg.AddFace(bbox, face.Confidence, identity, face.IsKnown, similarity, age, gender)
			}
			sd.wsHub.BroadcastFaces(cameraID, wsMsg)
		}

		// Update stream overlay with face detection boxes (fallback for coordinate-based overlay)
		sd.updateStreamOverlayFaces(cameraID, annotatedResult.Recognitions)

		// Run forensic analysis to get face thumbnails with landmarks
		sd.performForensicAnalysis(cameraID, frameData, event)
	}
}

// performFaceRecognitionFallback is the original non-annotated face recognition
func (sd *StreamDetector) performFaceRecognitionFallback(cameraID string, frameData []byte, event *MotionEvent) {
	result, err := sd.faceRecognizer.RecognizeFaces(frameData)
	if err != nil {
		fmt.Printf("Face recognition fallback failed for camera %s: %v\n", cameraID, err)
		return
	}

	if result.Count > 0 {
		event.FacesDetected = result.Count
		event.KnownIdentities = result.GetKnownIdentities()
		event.UnknownFacesCount = result.UnknownCount

		sd.faceRecognizer.LogRecognitionResult(result, cameraID)

		if result.UnknownCount > 0 && event.ThreatLevel != "high" {
			event.ThreatLevel = "high"
		}

		sd.updateStreamOverlayFaces(cameraID, result.Recognitions)
		sd.performForensicAnalysis(cameraID, frameData, event)
	}
}

// performFaceRecognitionForStream runs face recognition and ALWAYS sends to stream
// This is the simplified pipeline for YOLO+Face mode where Face is the final stage
// Face ALWAYS sends frames to maintain consistent frame rate (no time-travel)
func (sd *StreamDetector) performFaceRecognitionForStream(cameraID string, frameData []byte, captureSeq uint64, yoloResult *detection.AnnotatedSecurityResult) {
	// Get face recognizers
	sd.mu.RLock()
	grpcFace := sd.grpcFaceRecognizer
	httpFace := sd.faceRecognizer
	sd.mu.RUnlock()

	// Check if any face recognition is available
	grpcAvailable := grpcFace != nil && grpcFace.IsEnabled() && grpcFace.IsHealthy()
	httpAvailable := httpFace != nil && httpFace.IsEnabled()

	// Frame to send to stream - default to input (YOLO-annotated or raw)
	frameToSend := frameData

	var annotatedResult *detection.AnnotatedRecognitionResult
	var err error

	if grpcAvailable {
		annotatedResult, err = grpcFace.RecognizeFacesAnnotatedGRPC(frameData)
		if err != nil {
			fmt.Printf("gRPC face recognition failed for camera %s: %v, falling back to HTTP\n", cameraID, err)
			grpcAvailable = false
		}
	}

	// Fall back to HTTP if gRPC failed or unavailable
	if !grpcAvailable && httpAvailable {
		if !httpFace.IsHealthy() {
			if err := httpFace.CheckHealth(); err != nil {
				fmt.Printf("Face recognition service unhealthy for camera %s: %v\n", cameraID, err)
				// Still send the YOLO frame to maintain stream
			}
		} else {
			annotatedResult, err = httpFace.RecognizeFacesAnnotated(frameData)
			if err != nil {
				fmt.Printf("HTTP face recognition failed for camera %s: %v\n", cameraID, err)
			}
		}
	}

	// Use Face-annotated frame if available, otherwise keep YOLO frame
	if annotatedResult != nil && len(annotatedResult.ImageData) > 0 {
		frameToSend = annotatedResult.ImageData
	}

	// ALWAYS send frame to stream (Face is final stage in YOLO+Face pipeline)
	if captureSeq > 0 {
		sd.sendAnnotatedFrameToStreamWithSeq(cameraID, frameToSend, captureSeq)
	} else {
		sd.sendAnnotatedFrameToStream(cameraID, frameToSend)
	}

	// Process face recognition results for events/notifications
	if annotatedResult != nil && annotatedResult.Count > 0 {
		fmt.Printf("Face recognition for camera %s: total=%d, known=%d, unknown=%d (inference: %.1fms)\n",
			cameraID, annotatedResult.Count, annotatedResult.KnownCount, annotatedResult.UnknownCount, annotatedResult.InferenceTimeMs)

		// Determine threat level based on YOLO + Face results
		threatLevel := "low"
		objectClass := "unknown"
		var objectConfidence float32

		// Use YOLO detection info if available
		if yoloResult != nil && len(yoloResult.Detections) > 0 {
			bestDet := yoloResult.Detections[0]
			objectClass = bestDet.Class
			objectConfidence = bestDet.Confidence
			threatLevel = sd.gpuDetector.GetThreatLevel(bestDet)
		}

		// Upgrade threat level if unknown faces detected
		if annotatedResult.UnknownCount > 0 {
			threatLevel = "high"
		}

		// Save frame for event
		framePath := fmt.Sprintf("%s/yolo_face_%s_%d.jpg", sd.frameDir, cameraID, time.Now().UnixNano())
		if err := sd.saveFrameBytes(frameToSend, framePath); err != nil {
			fmt.Printf("Failed to save detection frame for camera %s: %v\n", cameraID, err)
			framePath = ""
		}

		// Create event combining YOLO + Face data
		event := &MotionEvent{
			ID:                uuid.New().String(),
			CameraID:          cameraID,
			Timestamp:         time.Now(),
			FramePath:         framePath,
			ObjectClass:       objectClass,
			ObjectConfidence:  objectConfidence,
			ThreatLevel:       threatLevel,
			InferenceTimeMs:   annotatedResult.InferenceTimeMs,
			DetectionDevice:   "yolo+insightface",
			FacesDetected:     annotatedResult.Count,
			UnknownFacesCount: annotatedResult.UnknownCount,
		}

		// Add YOLO inference time if available
		if yoloResult != nil {
			event.InferenceTimeMs += yoloResult.InferenceTimeMs
		}

		// Extract known identities
		for _, face := range annotatedResult.Recognitions {
			if face.IsKnown && face.Identity != nil {
				event.KnownIdentities = append(event.KnownIdentities, *face.Identity)
			}
		}

		// Broadcast via WebSocket
		if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
			wsMsg := ws.NewFaceMessage(cameraID)
			for _, face := range annotatedResult.Recognitions {
				var identity *string
				var similarity *float32
				var age *int
				var gender *string

				if face.Identity != nil {
					identity = face.Identity
				}
				if face.Similarity > 0 {
					sim := face.Similarity
					similarity = &sim
				}
				if face.Age > 0 {
					a := face.Age
					age = &a
				}
				if face.Gender != "" {
					g := face.Gender
					gender = &g
				}

				bbox := face.BBox
				if len(bbox) == 4 {
					bbox = []float32{bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]}
				}
				wsMsg.AddFace(bbox, face.Confidence, identity, face.IsKnown, similarity, age, gender)
			}
			sd.wsHub.BroadcastFaces(cameraID, wsMsg)
		}

		// Update stream overlay
		sd.updateStreamOverlayFaces(cameraID, annotatedResult.Recognitions)

		// Run forensic analysis
		sd.performForensicAnalysis(cameraID, frameData, event)

		// Save event
		sd.addEvent(event)
	} else if yoloResult != nil && len(yoloResult.Detections) > 0 {
		// No faces detected but YOLO found objects - create YOLO-only event
		bestDet := yoloResult.Detections[0]

		framePath := fmt.Sprintf("%s/yolo_%s_%d.jpg", sd.frameDir, cameraID, time.Now().UnixNano())
		if err := sd.saveFrameBytes(frameToSend, framePath); err != nil {
			fmt.Printf("Failed to save YOLO frame for camera %s: %v\n", cameraID, err)
			framePath = ""
		}

		event := &MotionEvent{
			ID:               uuid.New().String(),
			CameraID:         cameraID,
			Timestamp:        time.Now(),
			FramePath:        framePath,
			ObjectClass:      bestDet.Class,
			ObjectConfidence: bestDet.Confidence,
			ThreatLevel:      sd.gpuDetector.GetThreatLevel(bestDet),
			InferenceTimeMs:  yoloResult.InferenceTimeMs,
			DetectionDevice:  yoloResult.Device,
		}

		sd.addEvent(event)
		fmt.Printf("YOLO detection (no faces) saved for camera %s: %s (%.0f%% confidence)\n",
			cameraID, bestDet.Class, bestDet.Confidence*100)
	}
}

// performForensicAnalysis generates NSA-style forensic face thumbnails with landmarks
func (sd *StreamDetector) performForensicAnalysis(cameraID string, frameData []byte, event *MotionEvent) {
	forensicResult, err := sd.faceRecognizer.AnalyzeForensic(frameData)
	if err != nil {
		fmt.Printf("Forensic analysis failed for camera %s: %v\n", cameraID, err)
		return
	}

	if forensicResult.Count == 0 {
		return
	}

	// Save forensic thumbnails to disk
	var thumbnailPaths []string
	for i, face := range forensicResult.Faces {
		// Decode base64 image data
		imageData, err := base64.StdEncoding.DecodeString(face.ImageBase64)
		if err != nil {
			fmt.Printf("Failed to decode forensic thumbnail %d: %v\n", i, err)
			continue
		}

		// Generate filename
		identity := "unknown"
		if face.Identity != nil {
			identity = *face.Identity
		}
		thumbnailPath := fmt.Sprintf("%s/forensic_%s_%s_face%d_%d.jpg",
			sd.frameDir, cameraID, identity, i, time.Now().UnixNano())

		// Save thumbnail
		if err := sd.saveFrameBytes(imageData, thumbnailPath); err != nil {
			fmt.Printf("Failed to save forensic thumbnail: %v\n", err)
			continue
		}

		thumbnailPaths = append(thumbnailPaths, thumbnailPath)
		fmt.Printf("Saved forensic face thumbnail: %s (age: %d, gender: %s, known: %v)\n",
			thumbnailPath, face.Age, face.Gender, face.IsKnown)
	}

	event.ForensicThumbnails = thumbnailPaths
	fmt.Printf("Forensic analysis complete for camera %s: %d face thumbnails saved\n",
		cameraID, len(thumbnailPaths))
}

// saveFrameBytes saves frame data directly to disk
func (sd *StreamDetector) saveFrameBytes(frameData []byte, path string) error {
	return os.WriteFile(path, frameData, 0644)
}

// compareFrames compares two frames and detects motion (same as original implementation)
func (sd *StreamDetector) compareFrames(background, current image.Image) (motionDetected bool, confidence float32, bbox BoundingBox) {
	bgBounds := background.Bounds()
	curBounds := current.Bounds()

	if bgBounds != curBounds {
		return false, 0, BoundingBox{}
	}

	width := bgBounds.Dx()
	height := bgBounds.Dy()
	
	var totalDiff, pixelCount int
	minX, minY := width, height
	maxX, maxY := 0, 0
	
	// Sample every 2nd pixel for better performance while maintaining accuracy
	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x += 2 {
			bgR, bgG, bgB, _ := background.At(x, y).RGBA()
			curR, curG, curB, _ := current.At(x, y).RGBA()
			
			bgBrightness := (bgR + bgG + bgB) / 3
			curBrightness := (curR + curG + curB) / 3
			
			diff := int(bgBrightness) - int(curBrightness)
			if diff < 0 {
				diff = -diff
			}
			
			if diff > 6000 { // Lower threshold for streaming detection
				totalDiff++
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
			
			pixelCount++
		}
	}
	
	if pixelCount == 0 {
		return false, 0, BoundingBox{}
	}
	
	changeRatio := float32(totalDiff) / float32(pixelCount)
	
	if changeRatio < sd.sensitivity {
		return false, changeRatio, BoundingBox{}
	}
	
	bboxWidth := maxX - minX
	bboxHeight := maxY - minY
	area := bboxWidth * bboxHeight
	
	if area < sd.minMotionArea {
		return false, changeRatio, BoundingBox{}
	}
	
	bbox = BoundingBox{
		X:      minX,
		Y:      minY,
		Width:  bboxWidth,
		Height: bboxHeight,
	}
	
	confidence = changeRatio * 3 // Amplify for streaming detection
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return true, confidence, bbox
}

// detectMotion checks if there's motion between background and current frame
// Returns true if motion is detected above threshold
func (sd *StreamDetector) detectMotion(cameraID string, background, current image.Image) bool {
	motionDetected, _, _ := sd.compareFrames(background, current)
	return motionDetected
}

// createMotionEvent creates a basic motion event (no AI detection)
// Used when motion is detected but no AI detectors are enabled
func (sd *StreamDetector) createMotionEvent(cameraID string, frameData []byte, background, current image.Image) {
	motionDetected, confidence, bbox := sd.compareFrames(background, current)
	if !motionDetected {
		return
	}

	// Create event
	eventID := uuid.New().String()
	framePath := fmt.Sprintf("%s/motion_%s.jpg", sd.frameDir, eventID)

	// Save frame
	if err := sd.saveFrameBytes(frameData, framePath); err != nil {
		fmt.Printf("Failed to save motion frame for camera %s: %v\n", cameraID, err)
		return
	}

	event := &MotionEvent{
		ID:            eventID,
		CameraID:      cameraID,
		Timestamp:     time.Now(),
		Confidence:    confidence,
		BoundingBoxes: []BoundingBox{bbox},
		FramePath:     framePath,
		ObjectClass:   "motion",
	}

	sd.addEvent(event)
	fmt.Printf("Basic motion detected on camera %s with confidence %.2f\n", cameraID, confidence)
}

// Reuse methods from original detector
func (sd *StreamDetector) saveFrame(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return jpeg.Encode(file, img, &jpeg.Options{Quality: 85})
}

func (sd *StreamDetector) addEvent(event *MotionEvent) {
	// Check camera settings for events and notifications
	// Pipeline still runs for bounding boxes even if both are disabled
	var eventsEnabled, notificationsEnabled bool = true, true // Default to enabled
	if sd.db != nil {
		if cam, err := sd.db.GetCamera(event.CameraID); err == nil && cam != nil {
			eventsEnabled = cam.EventsEnabled
			notificationsEnabled = cam.NotificationsEnabled
		}
	}

	// If events are disabled, skip event storage (but still check notifications)
	if !eventsEnabled {
		// Only send notification if enabled (even without storing event)
		if notificationsEnabled && sd.telegramBot != nil && sd.telegramBot.IsEnabled() {
			go sd.sendTelegramNotification(event)
		}
		return
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.events[event.ID] = event
	sd.eventsByCamera[event.CameraID] = append(sd.eventsByCamera[event.CameraID], event)

	// Persist to database
	if sd.db != nil {
		// Convert bounding boxes to database format
		dbBBoxes := make([]database.BoundingBoxRecord, len(event.BoundingBoxes))
		for i, bbox := range event.BoundingBoxes {
			dbBBoxes[i] = database.BoundingBoxRecord{
				X:      bbox.X,
				Y:      bbox.Y,
				Width:  bbox.Width,
				Height: bbox.Height,
			}
		}

		record := &database.MotionEventRecord{
			ID:                 event.ID,
			CameraID:           event.CameraID,
			Timestamp:          event.Timestamp,
			Confidence:         float64(event.Confidence),
			BoundingBoxes:      dbBBoxes,
			FramePath:          event.FramePath,
			NotificationSent:   event.NotificationSent,
			ObjectClass:        event.ObjectClass,
			ObjectConfidence:   float64(event.ObjectConfidence),
			ThreatLevel:        event.ThreatLevel,
			InferenceTimeMs:    float64(event.InferenceTimeMs),
			DetectionDevice:    event.DetectionDevice,
			FacesDetected:      event.FacesDetected,
			KnownIdentities:    event.KnownIdentities,
			UnknownFacesCount:  event.UnknownFacesCount,
			ForensicThumbnails: event.ForensicThumbnails,
		}
		if err := sd.db.SaveMotionEvent(record); err != nil {
			fmt.Printf("Warning: failed to persist event to database: %v\n", err)
		}
	}

	// Send Telegram notification if configured and enabled for this camera
	if notificationsEnabled && sd.telegramBot != nil && sd.telegramBot.IsEnabled() && !event.NotificationSent {
		go sd.sendTelegramNotification(event)
	}

	if len(sd.events) > sd.maxEvents {
		sd.cleanupOldEvents()
	}
}

// sendTelegramNotification sends a notification for a motion event
func (sd *StreamDetector) sendTelegramNotification(event *MotionEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Read frame data from file
	var frameData []byte
	if event.FramePath != "" {
		data, err := os.ReadFile(event.FramePath)
		if err != nil {
			fmt.Printf("Warning: failed to read frame for Telegram notification: %v\n", err)
		} else {
			frameData = data
		}
	}

	// Get camera name from database, fall back to ID if not found
	cameraName := event.CameraID
	if sd.db != nil {
		if cam, err := sd.db.GetCamera(event.CameraID); err == nil && cam != nil && cam.Name != "" {
			cameraName = cam.Name
		}
	}

	var err error
	// Use enhanced notification with face info if available
	if event.FacesDetected > 0 {
		faceInfo := &telegram.FaceRecognitionInfo{
			FacesDetected:     event.FacesDetected,
			KnownIdentities:   event.KnownIdentities,
			UnknownFacesCount: event.UnknownFacesCount,
		}

		// Load forensic thumbnails if available
		if len(event.ForensicThumbnails) > 0 {
			for _, thumbnailPath := range event.ForensicThumbnails {
				thumbnailData, err := os.ReadFile(thumbnailPath)
				if err != nil {
					fmt.Printf("Warning: failed to read forensic thumbnail %s: %v\n", thumbnailPath, err)
					continue
				}
				faceInfo.ForensicThumbnails = append(faceInfo.ForensicThumbnails, thumbnailData)
			}
		}

		err = sd.telegramBot.SendMotionAlertWithFaces(ctx, cameraName, event.ObjectClass, event.ThreatLevel, faceInfo, frameData)
	} else if event.ObjectClass != "" && event.ObjectClass != "unknown" {
		// Use enhanced notification with object class but no face info
		err = sd.telegramBot.SendMotionAlertWithFaces(ctx, cameraName, event.ObjectClass, event.ThreatLevel, nil, frameData)
	} else {
		// Fall back to basic notification
		err = sd.telegramBot.SendMotionAlert(ctx, cameraName, event.Confidence, frameData)
	}

	if err != nil {
		fmt.Printf("Warning: failed to send Telegram notification: %v\n", err)
		return
	}

	// Mark notification as sent
	sd.mu.Lock()
	event.NotificationSent = true
	sd.mu.Unlock()

	// Update in database
	if sd.db != nil {
		if err := sd.db.MarkNotificationSent(event.ID); err != nil {
			fmt.Printf("Warning: failed to mark notification as sent: %v\n", err)
		}
	}

	fmt.Printf("Telegram notification sent for event %s on camera %s\n",
		event.ID, cameraName)
}

func (sd *StreamDetector) cleanupOldEvents() {
	// Same cleanup logic as original detector
	oldestTime := time.Now().Add(-24 * time.Hour)
	
	for eventID, event := range sd.events {
		if event.Timestamp.Before(oldestTime) {
			delete(sd.events, eventID)
			
			cameraEvents := sd.eventsByCamera[event.CameraID]
			for i, camEvent := range cameraEvents {
				if camEvent.ID == eventID {
					sd.eventsByCamera[event.CameraID] = append(cameraEvents[:i], cameraEvents[i+1:]...)
					break
				}
			}
		}
	}
}

// Additional methods to match the original detector interface
func (sd *StreamDetector) GetEvents(cameraID string, since *time.Time, limit int) []*MotionEvent {
	return sd.GetEventsWithBefore(cameraID, since, nil, limit)
}

// GetEventsWithBefore returns motion events with optional filtering including before timestamp
// Used for pagination - gets events older than 'before' timestamp
func (sd *StreamDetector) GetEventsWithBefore(cameraID string, since *time.Time, before *time.Time, limit int) []*MotionEvent {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	var events []*MotionEvent

	if cameraID != "" {
		events = make([]*MotionEvent, len(sd.eventsByCamera[cameraID]))
		copy(events, sd.eventsByCamera[cameraID])
	} else {
		events = make([]*MotionEvent, 0, len(sd.events))
		for _, event := range sd.events {
			events = append(events, event)
		}
	}

	// Apply since filter (events after this timestamp)
	if since != nil {
		filtered := make([]*MotionEvent, 0)
		for _, event := range events {
			if event.Timestamp.After(*since) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	// Apply before filter (events before this timestamp, for pagination)
	if before != nil {
		filtered := make([]*MotionEvent, 0)
		for _, event := range events {
			if event.Timestamp.Before(*before) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	// Sort by timestamp descending (newest first) before applying limit
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})

	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return events
}

func (sd *StreamDetector) GetEvent(eventID string) (*MotionEvent, error) {
	sd.mu.RLock()
	event, exists := sd.events[eventID]
	sd.mu.RUnlock()

	if exists {
		return event, nil
	}

	// Try to load from database if not in memory
	if sd.db != nil {
		record, err := sd.db.GetMotionEvent(eventID)
		if err != nil {
			return nil, fmt.Errorf("database error: %w", err)
		}
		if record != nil {
			// Convert database bounding boxes to internal format
			bboxes := make([]BoundingBox, len(record.BoundingBoxes))
			for i, bbox := range record.BoundingBoxes {
				bboxes[i] = BoundingBox{
					X:      bbox.X,
					Y:      bbox.Y,
					Width:  bbox.Width,
					Height: bbox.Height,
				}
			}

			event = &MotionEvent{
				ID:                 record.ID,
				CameraID:           record.CameraID,
				Timestamp:          record.Timestamp,
				Confidence:         float32(record.Confidence),
				BoundingBoxes:      bboxes,
				FramePath:          record.FramePath,
				NotificationSent:   record.NotificationSent,
				ObjectClass:        record.ObjectClass,
				ObjectConfidence:   float32(record.ObjectConfidence),
				ThreatLevel:        record.ThreatLevel,
				InferenceTimeMs:    float32(record.InferenceTimeMs),
				DetectionDevice:    record.DetectionDevice,
				FacesDetected:      record.FacesDetected,
				KnownIdentities:    record.KnownIdentities,
				UnknownFacesCount:  record.UnknownFacesCount,
				ForensicThumbnails: record.ForensicThumbnails,
			}
			return event, nil
		}
	}

	return nil, fmt.Errorf("event not found")
}

func (sd *StreamDetector) GetEventFrame(eventID string) ([]byte, error) {
	event, err := sd.GetEvent(eventID)
	if err != nil {
		return nil, err
	}

	if event.FramePath == "" {
		return nil, fmt.Errorf("no frame saved for this event")
	}

	return os.ReadFile(event.FramePath)
}

func (sd *StreamDetector) IsDetectionRunning(cameraID string) bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.isRunning[cameraID]
}

// HasAnyDetectionRunning returns true if detection is running on any camera
func (sd *StreamDetector) HasAnyDetectionRunning() bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	for _, running := range sd.isRunning {
		if running {
			return true
		}
	}
	return false
}

// SetDrawBoxes enables or disables bounding box drawing on detection images
func (sd *StreamDetector) SetDrawBoxes(enabled bool) {
	sd.gpuDetector.SetDrawBoxes(enabled)
}

// DrawBoxesEnabled returns whether bounding boxes are drawn on detection images
func (sd *StreamDetector) DrawBoxesEnabled() bool {
	return sd.gpuDetector.DrawBoxesEnabled()
}

// SetTelegramBot sets the Telegram bot for notifications
func (sd *StreamDetector) SetTelegramBot(bot *telegram.TelegramBot) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.telegramBot = bot
}

// SetWebSocketHub sets the WebSocket hub for real-time detection broadcasting
func (sd *StreamDetector) SetWebSocketHub(hub *ws.DetectionHub) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.wsHub = hub
}

// SetStreamOverlay sets the stream overlay provider for drawing bounding boxes on MJPEG streams
func (sd *StreamDetector) SetStreamOverlay(overlay stream.StreamOverlayProvider) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.streamOverlay = overlay
}

// SetPipelineConfig sets the function that provides detection configuration for mode gating
// When detection mode is "disabled", YOLO detection will be skipped (streaming only)
func (sd *StreamDetector) SetPipelineConfig(provider PipelineConfigProvider) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.pipelineConfig = provider
}

// SetYOLOConfig sets the function that provides YOLO configuration
func (sd *StreamDetector) SetYOLOConfig(provider YOLOConfigProvider) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.yoloConfig = provider
}

// getYOLOConfThreshold returns the configured YOLO confidence threshold
// Falls back to 0.5 if no config provider is set
func (sd *StreamDetector) getYOLOConfThreshold() float32 {
	sd.mu.RLock()
	provider := sd.yoloConfig
	sd.mu.RUnlock()

	if provider == nil {
		return 0.5 // Default threshold
	}
	return provider()
}

// SetGRPCDetector sets the gRPC-based YOLO detector for low-latency streaming detection
// If set, the StreamDetector will prefer gRPC over HTTP for object detection
func (sd *StreamDetector) SetGRPCDetector(grpcDet *detection.GRPCDetector) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.grpcDetector = grpcDet
}

// SetGRPCFaceRecognizer sets the gRPC-based face recognizer for low-latency streaming
// If set, the StreamDetector will prefer gRPC over HTTP for face recognition
func (sd *StreamDetector) SetGRPCFaceRecognizer(grpcFace *detection.GRPCFaceRecognizer) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.grpcFaceRecognizer = grpcFace
}

// SetFrameProvider sets the unified frame provider for detection
// When set, the detector will subscribe to frames from the provider instead of
// doing its own HTTP polling, avoiding duplicate requests to the same camera endpoint
func (sd *StreamDetector) SetFrameProvider(fp pipeline.FrameProvider) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.frameProvider = fp
	fmt.Println("[StreamDetector] Frame provider set - will use shared frame source")
}

// SetMotionSensitivity updates the motion detection sensitivity threshold
// Values: 0.0 (most sensitive) to 1.0 (least sensitive)
// Typical values: 0.05-0.10 (sensitive), 0.15 (default), 0.20-0.30 (less sensitive)
func (sd *StreamDetector) SetMotionSensitivity(sensitivity float32) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.sensitivity = sensitivity
	fmt.Printf("[StreamDetector] Motion sensitivity updated to %.2f\n", sensitivity)
}

// GetMotionSensitivity returns the current motion detection sensitivity threshold
func (sd *StreamDetector) GetMotionSensitivity() float32 {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.sensitivity
}

// SetMinMotionArea updates the minimum motion area threshold (in pixels)
func (sd *StreamDetector) SetMinMotionArea(area int) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.minMotionArea = area
	fmt.Printf("[StreamDetector] Min motion area updated to %d pixels\n", area)
}

// ConfigureGRPCYOLO sends configuration to the gRPC YOLO service
// This is called when YOLO settings are changed via the config API
func (sd *StreamDetector) ConfigureGRPCYOLO(confThreshold float32, classes []string) error {
	sd.mu.RLock()
	grpcDet := sd.grpcDetector
	sd.mu.RUnlock()

	if grpcDet == nil {
		return nil // No gRPC detector configured, silently ignore
	}

	return grpcDet.ConfigureWithClasses(confThreshold, true, "bytetrack", classes)
}

// isDetectionEnabled checks if detection is enabled for a camera based on pipeline config
// Returns true if:
// - No pipeline config provider is set (default behavior: detection enabled)
// - Pipeline config doesn't exist for camera (default behavior: detection enabled)
// - Pipeline config mode is not "disabled"
func (sd *StreamDetector) isDetectionEnabled(cameraID string) bool {
	sd.mu.RLock()
	provider := sd.pipelineConfig
	sd.mu.RUnlock()

	if provider == nil {
		return true // No config provider, assume detection enabled
	}

	config := provider(cameraID)
	if config == nil {
		return true // No config for this camera, assume detection enabled
	}

	return config.Mode != pipeline.DetectionModeDisabled
}

// isDetectorEnabled checks if a specific detector type is enabled in the pipeline config
// Returns true if:
// - No pipeline config provider is set (default behavior: detector enabled)
// - Pipeline config doesn't exist for camera (default behavior: detector enabled)
// - The detector is in the configured detectors list
func (sd *StreamDetector) isDetectorEnabled(cameraID string, detectorType string) bool {
	sd.mu.RLock()
	provider := sd.pipelineConfig
	sd.mu.RUnlock()

	if provider == nil {
		return true // No config provider, assume detector enabled
	}

	config := provider(cameraID)
	if config == nil {
		return true // No config for this camera, assume detector enabled
	}

	// If no detectors specified, default to all enabled
	if len(config.Detectors) == 0 {
		return true
	}

	// Check if the detector is in the list
	for _, d := range config.Detectors {
		if d == detectorType {
			return true
		}
	}
	return false
}

// isContinuousMode checks if detection should run on every frame (continuous/visual_only)
// vs requiring motion first (motion_triggered/hybrid/scheduled)
// Returns true for: continuous, visual_only
// Returns false for: motion_triggered, hybrid, scheduled, disabled
func (sd *StreamDetector) isContinuousMode(cameraID string) bool {
	sd.mu.RLock()
	provider := sd.pipelineConfig
	sd.mu.RUnlock()

	if provider == nil {
		return true // No config provider, default to continuous for backwards compatibility
	}

	config := provider(cameraID)
	if config == nil {
		return true // No config for this camera, default to continuous
	}

	// Continuous modes run detection on every frame
	switch config.Mode {
	case pipeline.DetectionModeContinuous, pipeline.DetectionModeVisualOnly:
		return true
	default:
		// motion_triggered, hybrid, scheduled, disabled all require motion gating
		return false
	}
}

// updateStreamOverlay sends detection data to the MJPEG stream for drawing bounding boxes
func (sd *StreamDetector) updateStreamOverlay(cameraID string, objectDetections []detection.Detection, faceRecognitions []detection.FaceRecognition) {
	sd.mu.RLock()
	overlay := sd.streamOverlay
	sd.mu.RUnlock()

	if overlay == nil {
		return
	}

	// Convert object detections to stream format
	streamDetections := make([]stream.Detection, 0, len(objectDetections))
	for _, det := range objectDetections {
		// Skip detections with invalid bounding boxes
		if len(det.BBox) < 4 {
			continue
		}

		// Get threat-based color
		threatLevel := sd.gpuDetector.GetThreatLevel(det)
		var detColor color.RGBA
		switch threatLevel {
		case "high":
			detColor = color.RGBA{R: 255, G: 0, B: 0, A: 255} // Red
		case "medium":
			detColor = color.RGBA{R: 255, G: 165, B: 0, A: 255} // Orange
		default:
			detColor = color.RGBA{R: 59, G: 130, B: 246, A: 255} // Blue
		}

		streamDetections = append(streamDetections, stream.Detection{
			Class:      det.Class,
			Confidence: det.Confidence,
			X:          int(det.BBox[0]),
			Y:          int(det.BBox[1]),
			W:          int(det.BBox[2] - det.BBox[0]),
			H:          int(det.BBox[3] - det.BBox[1]),
			Color:      detColor,
		})
	}

	// Convert face recognitions to stream format
	streamFaces := make([]stream.FaceDetection, 0, len(faceRecognitions))
	for _, face := range faceRecognitions {
		// Skip faces with invalid bounding boxes
		if len(face.BBox) < 4 {
			continue
		}

		identity := ""
		if face.IsKnown && face.Identity != nil && *face.Identity != "" {
			identity = *face.Identity
		}
		streamFaces = append(streamFaces, stream.FaceDetection{
			Identity:   identity,
			IsKnown:    face.IsKnown,
			Confidence: face.Similarity,
			X:          int(face.BBox[0]),
			Y:          int(face.BBox[1]),
			W:          int(face.BBox[2] - face.BBox[0]),
			H:          int(face.BBox[3] - face.BBox[1]),
		})
	}

	overlay.UpdateDetections(cameraID, streamDetections, streamFaces)
}

// updateStreamOverlayFaces updates only the face detection overlay (called after face recognition)
func (sd *StreamDetector) updateStreamOverlayFaces(cameraID string, faceRecognitions []detection.FaceRecognition) {
	sd.mu.RLock()
	overlay := sd.streamOverlay
	sd.mu.RUnlock()

	if overlay == nil || len(faceRecognitions) == 0 {
		return
	}

	// Convert face recognitions to stream format
	streamFaces := make([]stream.FaceDetection, 0, len(faceRecognitions))
	for _, face := range faceRecognitions {
		// Skip faces with invalid bounding boxes
		if len(face.BBox) < 4 {
			continue
		}

		identity := ""
		if face.IsKnown && face.Identity != nil {
			identity = *face.Identity
		}
		streamFaces = append(streamFaces, stream.FaceDetection{
			Identity:   identity,
			IsKnown:    face.IsKnown,
			Confidence: face.Similarity,
			X:          int(face.BBox[0]),
			Y:          int(face.BBox[1]),
			W:          int(face.BBox[2] - face.BBox[0]),
			H:          int(face.BBox[3] - face.BBox[1]),
		})
	}

	// Update overlay with empty detections but faces populated
	// This won't clear object detections since we pass nil
	overlay.UpdateDetections(cameraID, nil, streamFaces)
}

// sendAnnotatedFrameToStream sends a pre-annotated frame to the MJPEG streamer
// This is the preferred method - YOLO has already drawn the bounding boxes
func (sd *StreamDetector) sendAnnotatedFrameToStream(cameraID string, frameData []byte) {
	sd.mu.Lock()
	overlay := sd.streamOverlay
	// Get or create sequence counter for this camera
	seqPtr := sd.frameSeqCounters[cameraID]
	if seqPtr == nil {
		var seq uint64
		seqPtr = &seq
		sd.frameSeqCounters[cameraID] = seqPtr
	}
	// Increment and get current sequence
	*seqPtr++
	seq := *seqPtr
	sd.mu.Unlock()

	if overlay == nil || len(frameData) == 0 {
		return
	}

	overlay.SetAnnotatedFrame(cameraID, seq, frameData)
}

// sendAnnotatedFrameToStreamWithSeq sends a pre-annotated frame with a specific capture sequence
// This preserves frame ordering when YOLO processing has variable latency
func (sd *StreamDetector) sendAnnotatedFrameToStreamWithSeq(cameraID string, frameData []byte, captureSeq uint64) {
	sd.mu.RLock()
	overlay := sd.streamOverlay
	sd.mu.RUnlock()

	if overlay == nil || len(frameData) == 0 {
		return
	}

	// Use the capture sequence directly (assigned at frame capture time)
	overlay.SetAnnotatedFrame(cameraID, captureSeq, frameData)
}

// hasFaceRecognizer returns true if any face recognizer (gRPC or HTTP) is available
func (sd *StreamDetector) hasFaceRecognizer() bool {
	sd.mu.RLock()
	grpcFace := sd.grpcFaceRecognizer
	httpFace := sd.faceRecognizer
	sd.mu.RUnlock()

	grpcAvailable := grpcFace != nil && grpcFace.IsEnabled() && grpcFace.IsHealthy()
	httpAvailable := httpFace != nil && httpFace.IsEnabled()

	return grpcAvailable || httpAvailable
}

// runDirectFaceDetection runs face recognition directly on a frame (Face-only mode)
// This is used when Face detector is enabled but YOLO is NOT enabled
// InsightFace has its own built-in face detector, so it doesn't need YOLO
func (sd *StreamDetector) runDirectFaceDetection(cameraID string, frameData []byte, captureSeq uint64) {
	// Get face recognizers
	sd.mu.RLock()
	grpcFace := sd.grpcFaceRecognizer
	httpFace := sd.faceRecognizer
	sd.mu.RUnlock()

	// Check if any face recognition is available
	grpcAvailable := grpcFace != nil && grpcFace.IsEnabled() && grpcFace.IsHealthy()
	httpAvailable := httpFace != nil && httpFace.IsEnabled()

	if !grpcAvailable && !httpAvailable {
		return
	}

	// Try gRPC first for low latency
	var annotatedResult *detection.AnnotatedRecognitionResult
	var err error

	if grpcAvailable {
		annotatedResult, err = grpcFace.RecognizeFacesAnnotatedGRPC(frameData)
		if err != nil {
			fmt.Printf("gRPC face detection failed for camera %s: %v, falling back to HTTP\n", cameraID, err)
			grpcAvailable = false
		}
	}

	// Fall back to HTTP if gRPC failed or unavailable
	if !grpcAvailable && httpAvailable {
		// Check if HTTP service is healthy
		if !httpFace.IsHealthy() {
			if err := httpFace.CheckHealth(); err != nil {
				fmt.Printf("Face recognition service unhealthy for camera %s: %v\n", cameraID, err)
				return
			}
		}
		annotatedResult, err = httpFace.RecognizeFacesAnnotated(frameData)
		if err != nil {
			fmt.Printf("HTTP face detection failed for camera %s: %v\n", cameraID, err)
			return
		}
	}

	// In Face-only mode, ALWAYS send a frame to maintain smooth stream (no time-travel)
	// If face detection returned annotated data, use that; otherwise use original frame
	var frameToSend []byte
	if annotatedResult != nil && len(annotatedResult.ImageData) > 0 {
		frameToSend = annotatedResult.ImageData
	} else {
		// Face detection failed or returned no data - send original frame
		// This prevents raw frames from being shown (time-travel effect)
		frameToSend = frameData
	}

	// Send frame to stream (this is the final output in Face-only mode)
	if captureSeq > 0 {
		sd.sendAnnotatedFrameToStreamWithSeq(cameraID, frameToSend, captureSeq)
	} else {
		sd.sendAnnotatedFrameToStream(cameraID, frameToSend)
	}

	// Only process events if faces were detected
	if annotatedResult == nil {
		return
	}

	// Only process if faces were detected
	if annotatedResult.Count > 0 {
		fmt.Printf("Face detection for camera %s: total=%d, known=%d, unknown=%d (inference: %.1fms)\n",
			cameraID, annotatedResult.Count, annotatedResult.KnownCount, annotatedResult.UnknownCount, annotatedResult.InferenceTimeMs)

		// Determine threat level based on face recognition
		threatLevel := "low"
		if annotatedResult.UnknownCount > 0 {
			threatLevel = "high" // Unknown person = high threat
		} else if annotatedResult.KnownCount > 0 {
			threatLevel = "medium" // Known person = medium (someone is there)
		}

		// Create motion event for the face detection
		framePath := fmt.Sprintf("%s/face_%s_%d.jpg", sd.frameDir, cameraID, time.Now().UnixNano())
		if err := os.WriteFile(framePath, frameData, 0644); err != nil {
			fmt.Printf("Failed to save face detection frame for camera %s: %v\n", cameraID, err)
		}

		event := &MotionEvent{
			ID:                fmt.Sprintf("face_%s_%d", cameraID, time.Now().UnixNano()),
			CameraID:          cameraID,
			Timestamp:         time.Now(),
			FramePath:         framePath,
			ObjectClass:       "face",
			ObjectConfidence:  annotatedResult.Recognitions[0].Confidence,
			ThreatLevel:       threatLevel,
			InferenceTimeMs:   annotatedResult.InferenceTimeMs,
			DetectionDevice:   "insightface",
			FacesDetected:     annotatedResult.Count,
			UnknownFacesCount: annotatedResult.UnknownCount,
		}

		// Extract known identities
		for _, face := range annotatedResult.Recognitions {
			if face.IsKnown && face.Identity != nil {
				event.KnownIdentities = append(event.KnownIdentities, *face.Identity)
			}
		}

		// Broadcast face recognition results via WebSocket
		if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
			wsMsg := ws.NewFaceMessage(cameraID)
			for _, face := range annotatedResult.Recognitions {
				var identity *string
				var similarity *float32
				var age *int
				var gender *string

				if face.Identity != nil {
					identity = face.Identity
				}
				if face.Similarity > 0 {
					sim := face.Similarity
					similarity = &sim
				}
				if face.Age > 0 {
					a := face.Age
					age = &a
				}
				if face.Gender != "" {
					g := face.Gender
					gender = &g
				}

				// Convert bbox from [x1, y1, x2, y2] to [x, y, w, h] format for frontend
				bbox := face.BBox
				if len(bbox) == 4 {
					bbox = []float32{bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]}
				}
				wsMsg.AddFace(bbox, face.Confidence, identity, face.IsKnown, similarity, age, gender)
			}
			sd.wsHub.BroadcastFaces(cameraID, wsMsg)
		}

		// Update stream overlay with face detection boxes
		sd.updateStreamOverlayFaces(cameraID, annotatedResult.Recognitions)

		// Run forensic analysis to get face thumbnails
		sd.performForensicAnalysis(cameraID, frameData, event)

		// Save event (handles database persistence and Telegram notifications)
		sd.addEvent(event)
	}
}

// runDirectYOLODetectionWithSeq runs YOLO detection with capture sequence for frame ordering
func (sd *StreamDetector) runDirectYOLODetectionWithSeq(cameraID string, frameData []byte, captureSeq uint64) {
	// Check which detector to use (prefer gRPC for low latency)
	sd.mu.RLock()
	grpcDet := sd.grpcDetector
	drawBoxes := sd.gpuDetector.DrawBoxesEnabled()
	sd.mu.RUnlock()

	// If draw boxes is enabled, use annotated detection
	if drawBoxes || (grpcDet != nil && grpcDet.DrawBoxesEnabled()) {
		sd.runDirectYOLODetectionAnnotatedWithSeq(cameraID, frameData, captureSeq)
		return
	}

	// For non-annotated detection, call the original function
	sd.runDirectYOLODetection(cameraID, frameData)
}

// streamFromProvider receives frames from the unified FrameProvider
// This avoids duplicate HTTP polling - both streaming and detection share the same frame source
func (sd *StreamDetector) streamFromProvider(cameraID string, fp pipeline.FrameProvider, stopCh chan struct{}) {
	fmt.Printf("[StreamDetector] Subscribing to frame provider for camera %s\n", cameraID)

	// Subscribe to frames from the provider
	sub, err := fp.Subscribe(cameraID, 10)
	if err != nil {
		fmt.Printf("[StreamDetector] Failed to subscribe to frame provider for camera %s: %v\n", cameraID, err)
		return
	}
	defer fp.Unsubscribe(sub)

	// Get the frame buffer for this camera
	sd.mu.RLock()
	frameBuffer := sd.frameBuffers[cameraID]
	sd.mu.RUnlock()

	if frameBuffer == nil {
		fmt.Printf("[StreamDetector] No frame buffer for camera %s\n", cameraID)
		return
	}

	frameCount := 0
	for {
		select {
		case <-stopCh:
			fmt.Printf("[StreamDetector] Frame provider subscription stopped for camera %s after %d frames\n", cameraID, frameCount)
			return
		case <-sub.Done:
			fmt.Printf("[StreamDetector] Frame provider subscription ended for camera %s\n", cameraID)
			return
		case frame := <-sub.Channel:
			if frame != nil && len(frame.Data) > 0 {
				frameCount++
				// Forward frame to the detection buffer
				select {
				case frameBuffer <- frame.Data:
					if frameCount%100 == 0 {
						fmt.Printf("[StreamDetector] Received %d frames from provider for camera %s\n", frameCount, cameraID)
					}
				default:
					// Buffer full, skip frame
				}
			}
		}
	}
}

// streamHTTPImages polls HTTP image endpoints for motion detection
func (sd *StreamDetector) streamHTTPImages(cameraID, imageURL string, stopCh chan struct{}) {
	fmt.Printf("Starting HTTP image polling for camera %s at %s\n", cameraID, imageURL)

	// Use the frame buffer already created in StartStreamingDetection
	sd.mu.RLock()
	frameBuffer := sd.frameBuffers[cameraID]
	sd.mu.RUnlock()

	if frameBuffer == nil {
		fmt.Printf("Error: No frame buffer found for camera %s\n", cameraID)
		return
	}

	// Start background polling - this puts frames into the shared buffer
	// The analyzeFrames goroutine (started by StartStreamingDetection) will process them
	sd.pollHTTPImage(cameraID, imageURL, frameBuffer, stopCh)
}

// pollHTTPImage continuously fetches images from HTTP endpoint
// Uses adaptive polling with backoff on errors to avoid overwhelming rate-limited servers
func (sd *StreamDetector) pollHTTPImage(cameraID, imageURL string, frameBuffer chan []byte, stopCh chan struct{}) {
	baseInterval := 1000 * time.Millisecond // 1 FPS base rate (reduced from 2 FPS)
	currentInterval := baseInterval
	maxInterval := 5 * time.Second
	consecutiveErrors := 0

	ticker := time.NewTicker(currentInterval)
	defer ticker.Stop()

	frameCount := 0
	errorCount := 0
	for {
		select {
		case <-stopCh:
			fmt.Printf("HTTP polling stopped for camera %s after %d frames (%d errors)\n", cameraID, frameCount, errorCount)
			return
		case <-ticker.C:
			frameData, err := sd.fetchHTTPImage(imageURL)
			if err != nil {
				errorCount++
				consecutiveErrors++
				// Only log every 10th error to reduce spam
				if errorCount%10 == 1 {
					fmt.Printf("Failed to fetch image from %s: %v (error %d)\n", imageURL, err, errorCount)
				}
				// Exponential backoff on consecutive errors
				if consecutiveErrors > 3 {
					currentInterval = time.Duration(float64(currentInterval) * 1.5)
					if currentInterval > maxInterval {
						currentInterval = maxInterval
					}
					ticker.Reset(currentInterval)
				}
				continue
			}

			// Success - reset backoff
			if consecutiveErrors > 0 {
				consecutiveErrors = 0
				if currentInterval != baseInterval {
					currentInterval = baseInterval
					ticker.Reset(currentInterval)
				}
			}

			frameCount++

			// Try non-blocking send, check stop channel first
			select {
			case <-stopCh:
				fmt.Printf("HTTP polling stopped for camera %s after %d frames (%d errors)\n", cameraID, frameCount, errorCount)
				return
			case frameBuffer <- frameData:
				if frameCount%20 == 0 { // Log every 20 frames
					fmt.Printf("HTTP polling: sent %d frames for camera %s\n", frameCount, cameraID)
				}
			default:
				// Buffer full - this is fine, skip frame
			}
		}
	}
}

// httpClient is a shared HTTP client with connection pooling for frame fetching
// Reusing connections reduces EOF errors from connection churn
var httpImageClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
	},
}

// fetchHTTPImage fetches a single image from HTTP endpoint using direct HTTP GET
// This is more reliable than ffmpeg for simple JPEG image endpoints
func (sd *StreamDetector) fetchHTTPImage(imageURL string) ([]byte, error) {
	resp, err := httpImageClient.Get(imageURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	frameData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}

	return frameData, nil
}

// processHTTPFrames processes frames from HTTP polling
func (sd *StreamDetector) processHTTPFrames(cameraID string, frameBuffer chan []byte, stopCh chan struct{}) {
	var backgroundImg image.Image
	var lastBackgroundUpdate time.Time

	// Wait for initial background frame
	select {
	case frameData := <-frameBuffer:
		img, err := jpeg.Decode(bytes.NewReader(frameData))
		if err != nil {
			fmt.Printf("Failed to decode initial HTTP frame for camera %s: %v\n", cameraID, err)
			return
		}
		backgroundImg = img
		lastBackgroundUpdate = time.Now()
		sd.mu.Lock()
		sd.backgroundFrames[cameraID] = backgroundImg
		sd.mu.Unlock()
		fmt.Printf("HTTP polling initialized for camera %s\n", cameraID)
	case <-time.After(30 * time.Second):
		fmt.Printf("Timeout waiting for initial HTTP frame for camera %s\n", cameraID)
		return
	case <-stopCh:
		return
	}

	for {
		select {
		case <-stopCh:
			return
		case frameData := <-frameBuffer:
			currentImg, err := jpeg.Decode(bytes.NewReader(frameData))
			if err != nil {
				continue
			}

			// Broadcast frame via WebSocket for live streaming (if clients connected)
			if sd.wsHub != nil && sd.wsHub.HasClients(cameraID) {
				bounds := currentImg.Bounds()
				frameMsg := ws.NewFrameMessage(cameraID, bounds.Dx(), bounds.Dy(), base64.StdEncoding.EncodeToString(frameData))
				sd.wsHub.BroadcastFrame(cameraID, frameMsg)
			}

			// Update background periodically
			if time.Since(lastBackgroundUpdate) > 30*time.Second {
				backgroundImg = currentImg
				sd.mu.Lock()
				sd.backgroundFrames[cameraID] = backgroundImg
				sd.mu.Unlock()
				lastBackgroundUpdate = time.Now()
			}

			// Process motion detection
			if backgroundImg != nil {
				sd.processMotionWithDINOv3(cameraID, frameData, backgroundImg, currentImg)
			}
		}
	}
}