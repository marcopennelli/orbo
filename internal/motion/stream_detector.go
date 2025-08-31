package motion

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"orbo/internal/detection"
)

// StreamDetector handles streaming motion detection
type StreamDetector struct {
	mu              sync.RWMutex
	events          map[string]*MotionEvent
	eventsByCamera  map[string][]*MotionEvent
	isRunning       map[string]bool
	stopChannels    map[string]chan struct{}
	streamProcesses map[string]*exec.Cmd
	sensitivity     float32
	minMotionArea   int
	maxEvents       int
	frameDir        string
	backgroundFrames map[string]image.Image
	frameBuffers    map[string]chan []byte
	gpuDetector     *detection.GPUDetector // NEW: GPU object detection
}

// NewStreamDetector creates a new streaming motion detector
func NewStreamDetector(frameDir string) *StreamDetector {
	return &StreamDetector{
		events:          make(map[string]*MotionEvent),
		eventsByCamera:  make(map[string][]*MotionEvent),
		isRunning:       make(map[string]bool),
		stopChannels:    make(map[string]chan struct{}),
		streamProcesses: make(map[string]*exec.Cmd),
		backgroundFrames: make(map[string]image.Image),
		frameBuffers:    make(map[string]chan []byte),
		sensitivity:     0.15, // More sensitive for real-time detection
		minMotionArea:   300,  // Smaller minimum area for faster detection
		maxEvents:       1000,
		frameDir:        frameDir,
		gpuDetector:     detection.NewGPUDetector("http://localhost:8081"), // Initialize GPU detector
	}
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
		cmd.Process.Kill()
		delete(sd.streamProcesses, cameraID)
	}

	// Close channels
	if stopCh, exists := sd.stopChannels[cameraID]; exists {
		close(stopCh)
		delete(sd.stopChannels, cameraID)
	}

	if frameBuffer, exists := sd.frameBuffers[cameraID]; exists {
		close(frameBuffer)
		delete(sd.frameBuffers, cameraID)
	}

	sd.isRunning[cameraID] = false
	delete(sd.backgroundFrames, cameraID)
}

// streamFrames continuously captures frames from the camera
func (sd *StreamDetector) streamFrames(cameraID, cameraDevice string, stopCh chan struct{}) {
	fmt.Printf("Started streaming motion detection for camera %s\n", cameraID)
	defer fmt.Printf("Stopped streaming motion detection for camera %s\n", cameraID)

	// Start ffmpeg process for continuous streaming
	cmd := exec.Command("ffmpeg",
		"-f", "v4l2",
		"-video_size", "640x480",
		"-framerate", "10", // 10 FPS for good motion detection balance
		"-i", cameraDevice,
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "5", // Lower quality for faster processing
		"-")

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
					case sd.frameBuffers[cameraID] <- frame:
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
	frameBuffer := sd.frameBuffers[cameraID]

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
			// Decode current frame
			currentImg, err := jpeg.Decode(bytes.NewReader(frameData))
			if err != nil {
				continue
			}

			// Update background periodically (every 30 seconds)
			if time.Since(lastBackgroundUpdate) > 30*time.Second {
				backgroundImg = currentImg
				sd.mu.Lock()
				sd.backgroundFrames[cameraID] = backgroundImg
				sd.mu.Unlock()
				lastBackgroundUpdate = time.Now()
			}

			// Perform hybrid motion detection (basic motion + GPU object detection)
			if backgroundImg != nil {
				if motionDetected, motionConfidence, motionBBox := sd.compareFrames(backgroundImg, currentImg); motionDetected {
					// Motion detected! Now use GPU to identify what moved
					sd.processMotionWithGPU(cameraID, frameData, motionConfidence, motionBBox)
				}
			}
		}
	}
}

// processMotionWithGPU processes detected motion using GPU-accelerated object detection
func (sd *StreamDetector) processMotionWithGPU(cameraID string, frameData []byte, motionConfidence float32, motionBBox BoundingBox) {
	// Try GPU detection first
	if sd.gpuDetector.IsHealthy() {
		securityResult, err := sd.gpuDetector.DetectSecurityObjects(frameData, 0.5)
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
	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.events[event.ID] = event
	sd.eventsByCamera[event.CameraID] = append(sd.eventsByCamera[event.CameraID], event)

	if len(sd.events) > sd.maxEvents {
		sd.cleanupOldEvents()
	}
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

	if since != nil {
		filtered := make([]*MotionEvent, 0)
		for _, event := range events {
			if event.Timestamp.After(*since) {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}

	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	return events
}

func (sd *StreamDetector) GetEvent(eventID string) (*MotionEvent, error) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	event, exists := sd.events[eventID]
	if !exists {
		return nil, fmt.Errorf("event not found")
	}

	return event, nil
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