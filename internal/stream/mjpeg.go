package stream

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Detection represents a detection result for overlay drawing
type Detection struct {
	Class      string
	Confidence float32
	X, Y, W, H int
	Color      color.RGBA
}

// FaceDetection represents a face detection for overlay drawing
type FaceDetection struct {
	Identity   string
	IsKnown    bool
	Confidence float32
	X, Y, W, H int
}

// StreamOverlayProvider is an interface for updating detection overlays on streams
// This allows the motion detector to send detection data to the stream manager
type StreamOverlayProvider interface {
	UpdateDetections(cameraID string, detections []Detection, faces []FaceDetection)
	// SetAnnotatedFrame injects a pre-annotated frame (with bounding boxes already drawn)
	// This frame will be used for streaming until a newer annotated frame arrives or it expires
	SetAnnotatedFrame(cameraID string, frameData []byte)
	// GetCurrentFrameSeq returns the current frame sequence number for a camera
	GetCurrentFrameSeq(cameraID string) uint64
}

// FrameListener is called when a new frame is captured (for WebCodecs streaming)
type FrameListener func(cameraID string, frame []byte)

// MJPEGStream handles MJPEG streaming for a single camera
type MJPEGStream struct {
	cameraID     string
	device       string
	fps          int
	width        int
	height       int
	running      bool
	mu           sync.RWMutex
	clients      map[chan []byte]bool
	clientsMu    sync.RWMutex
	currentFrame []byte
	frameMu      sync.RWMutex
	stopCh       chan struct{}
	cmd          *exec.Cmd

	// Detection overlay data (legacy - kept for compatibility)
	detections     []Detection
	faces          []FaceDetection
	detectionsMu   sync.RWMutex
	overlayEnabled bool

	// Frame sequence tracking
	frameSeq   uint64 // Current frame sequence number (incremented on each new frame)
	frameSeqMu sync.RWMutex

	// Frame listener for WebCodecs (raw frame passthrough)
	frameListener   FrameListener
	frameListenerMu sync.RWMutex
}

// MJPEGStreamManager manages MJPEG streams for all cameras
type MJPEGStreamManager struct {
	streams map[string]*MJPEGStream
	mu      sync.RWMutex
}

// NewMJPEGStreamManager creates a new stream manager
func NewMJPEGStreamManager() *MJPEGStreamManager {
	return &MJPEGStreamManager{
		streams: make(map[string]*MJPEGStream),
	}
}

// CreateStream creates a new MJPEG stream for a camera
func (m *MJPEGStreamManager) CreateStream(cameraID, device string, fps, width, height int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.streams[cameraID]; exists {
		return fmt.Errorf("stream already exists for camera %s", cameraID)
	}

	stream := &MJPEGStream{
		cameraID:       cameraID,
		device:         device,
		fps:            fps,
		width:          width,
		height:         height,
		clients:        make(map[chan []byte]bool),
		stopCh:         make(chan struct{}),
		overlayEnabled: true,
	}

	m.streams[cameraID] = stream

	// Start frame capture
	go stream.captureFrames()

	log.Printf("[MJPEGStream] Created stream for camera %s (device: %s, fps: %d)", cameraID, device, fps)
	return nil
}

// DeleteStream stops and removes a stream
func (m *MJPEGStreamManager) DeleteStream(cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, exists := m.streams[cameraID]
	if !exists {
		return fmt.Errorf("stream not found for camera %s", cameraID)
	}

	stream.Stop()
	delete(m.streams, cameraID)

	log.Printf("[MJPEGStream] Deleted stream for camera %s", cameraID)
	return nil
}

// GetStream returns a stream by camera ID
func (m *MJPEGStreamManager) GetStream(cameraID string) *MJPEGStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streams[cameraID]
}

// UpdateDetections updates the detection overlay data for a camera
func (m *MJPEGStreamManager) UpdateDetections(cameraID string, detections []Detection, faces []FaceDetection) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.UpdateDetections(detections, faces)
	}
}

// SetAnnotatedFrame sets a pre-annotated frame for a camera
func (m *MJPEGStreamManager) SetAnnotatedFrame(cameraID string, frameData []byte) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.SetAnnotatedFrame(frameData)
	}
}

// GetCurrentFrameSeq returns the current frame sequence number for a camera
func (m *MJPEGStreamManager) GetCurrentFrameSeq(cameraID string) uint64 {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		return stream.GetCurrentFrameSeq()
	}
	return 0
}

// SetFrameListener sets a callback that receives all captured frames (for WebCodecs)
func (m *MJPEGStreamManager) SetFrameListener(cameraID string, listener FrameListener) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.SetFrameListener(listener)
	}
}

// SetGlobalFrameListener sets a callback for all streams (for WebCodecs manager)
func (m *MJPEGStreamManager) SetGlobalFrameListener(listener FrameListener) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, stream := range m.streams {
		stream.SetFrameListener(listener)
	}
}

// ServeHTTP handles MJPEG stream requests
func (m *MJPEGStreamManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract camera_id from path: /video/stream/{camera_id}
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	cameraID := pathParts[len(pathParts)-1]

	stream := m.GetStream(cameraID)
	if stream == nil {
		http.Error(w, fmt.Sprintf("Stream not found for camera %s", cameraID), http.StatusNotFound)
		return
	}

	stream.ServeHTTP(w, r)
}

// Stop stops the stream
func (s *MJPEGStream) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopCh)
	s.running = false

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}

	// Close all client channels
	s.clientsMu.Lock()
	for ch := range s.clients {
		close(ch)
		delete(s.clients, ch)
	}
	s.clientsMu.Unlock()
}

// SetFrameListener sets a callback that receives all captured frames
func (s *MJPEGStream) SetFrameListener(listener FrameListener) {
	s.frameListenerMu.Lock()
	defer s.frameListenerMu.Unlock()
	s.frameListener = listener
}

// UpdateDetections updates the detection overlay data
// Pass nil to keep the existing value for that type
func (s *MJPEGStream) UpdateDetections(detections []Detection, faces []FaceDetection) {
	s.detectionsMu.Lock()
	defer s.detectionsMu.Unlock()
	if detections != nil {
		s.detections = detections
	}
	if faces != nil {
		s.faces = faces
	}
}

// SetAnnotatedFrame broadcasts a pre-annotated frame (with bounding boxes) directly to all clients
// This is called from the detection pipeline after YOLO/InsightFace processing
// The frame is sent immediately - no storage, no staleness checks
func (s *MJPEGStream) SetAnnotatedFrame(frameData []byte) {
	if len(frameData) == 0 {
		return
	}

	// Broadcast directly to all connected clients - this IS the real-time frame
	s.clientsMu.RLock()
	for ch := range s.clients {
		select {
		case ch <- frameData:
		default:
			// Client is slow, skip frame (don't block detection pipeline)
		}
	}
	s.clientsMu.RUnlock()
}

// GetCurrentFrame returns the current frame (with optional overlay)
func (s *MJPEGStream) GetCurrentFrame() []byte {
	s.frameMu.RLock()
	frame := s.currentFrame
	s.frameMu.RUnlock()

	if frame == nil || !s.overlayEnabled {
		return frame
	}

	// Check if we have detections to draw
	s.detectionsMu.RLock()
	hasDetections := len(s.detections) > 0 || len(s.faces) > 0
	detections := s.detections
	faces := s.faces
	s.detectionsMu.RUnlock()

	if !hasDetections {
		return frame
	}

	// Draw overlays on the frame
	return s.drawOverlays(frame, detections, faces)
}

// captureFrames continuously captures frames from the camera
func (s *MJPEGStream) captureFrames() {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	log.Printf("[MJPEGStream] Starting frame capture for camera %s", s.cameraID)

	// Check if it's an HTTP image endpoint (polling mode)
	if isHTTPImageEndpoint(s.device) {
		s.captureHTTPImages()
		return
	}

	// Use ffmpeg for streaming sources
	s.captureFFmpeg()
}

// isHTTPImageEndpoint checks if the device is an HTTP image endpoint
func isHTTPImageEndpoint(device string) bool {
	return (strings.HasPrefix(device, "http://") || strings.HasPrefix(device, "https://")) &&
		(strings.Contains(device, ".jpg") || strings.Contains(device, ".jpeg") || strings.Contains(device, "image"))
}

// captureHTTPImages polls an HTTP image endpoint
func (s *MJPEGStream) captureHTTPImages() {
	client := &http.Client{Timeout: 10 * time.Second}
	interval := time.Second / time.Duration(s.fps)
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			resp, err := client.Get(s.device)
			if err != nil {
				log.Printf("[MJPEGStream] Error fetching frame from %s: %v", s.device, err)
				continue
			}

			frame, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("[MJPEGStream] Error reading frame: %v", err)
				continue
			}

			s.updateFrame(frame)
		}
	}
}

// captureFFmpeg captures frames using ffmpeg
func (s *MJPEGStream) captureFFmpeg() {
	var args []string

	if strings.HasPrefix(s.device, "rtsp://") {
		// RTSP stream
		args = []string{
			"-rtsp_transport", "tcp",
			"-i", s.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-r", fmt.Sprintf("%d", s.fps),
			"-q:v", "5",
			"-",
		}
	} else if strings.HasPrefix(s.device, "http://") || strings.HasPrefix(s.device, "https://") {
		// HTTP/MJPEG stream
		args = []string{
			"-i", s.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-r", fmt.Sprintf("%d", s.fps),
			"-q:v", "5",
			"-",
		}
	} else {
		// V4L2 device (USB camera)
		args = []string{
			"-f", "v4l2",
			"-video_size", fmt.Sprintf("%dx%d", s.width, s.height),
			"-framerate", fmt.Sprintf("%d", s.fps),
			"-i", s.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "5",
			"-",
		}
	}

	s.cmd = exec.Command("ffmpeg", args...)

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		log.Printf("[MJPEGStream] Error creating stdout pipe: %v", err)
		return
	}

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		log.Printf("[MJPEGStream] Error creating stderr pipe: %v", err)
		return
	}

	if err := s.cmd.Start(); err != nil {
		log.Printf("[MJPEGStream] Error starting ffmpeg: %v", err)
		return
	}

	// Consume stderr silently
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			// Silently consume
		}
	}()

	// Read frames
	frameBuffer := make([]byte, 0, 1024*1024)
	chunk := make([]byte, 8192)

	for {
		select {
		case <-s.stopCh:
			return
		default:
			n, err := stdout.Read(chunk)
			if err != nil {
				if err != io.EOF {
					log.Printf("[MJPEGStream] Error reading frame: %v", err)
				}
				return
			}

			frameBuffer = append(frameBuffer, chunk[:n]...)

			// Extract complete JPEG frames
			for {
				frame := extractJPEGFrame(&frameBuffer)
				if frame == nil {
					break
				}
				s.updateFrame(frame)
			}
		}
	}
}

// updateFrame updates the current frame and broadcasts to clients
func (s *MJPEGStream) updateFrame(frame []byte) {
	// Increment frame sequence number
	s.frameSeqMu.Lock()
	s.frameSeq++
	currentSeq := s.frameSeq
	s.frameSeqMu.Unlock()

	s.frameMu.Lock()
	s.currentFrame = frame
	s.frameMu.Unlock()

	// Broadcast to all connected clients with current sequence
	s.clientsMu.RLock()
	for ch := range s.clients {
		select {
		case ch <- frame:
		default:
			// Client is slow, skip frame
		}
	}
	s.clientsMu.RUnlock()

	// Notify frame listener (for WebCodecs streaming)
	s.frameListenerMu.RLock()
	listener := s.frameListener
	s.frameListenerMu.RUnlock()
	if listener != nil {
		listener(s.cameraID, frame)
	}

	// Log sequence for debugging (every 100 frames)
	if currentSeq%100 == 0 {
		log.Printf("[MJPEGStream] Camera %s frame seq: %d", s.cameraID, currentSeq)
	}
}

// GetCurrentFrameSeq returns the current frame sequence number
func (s *MJPEGStream) GetCurrentFrameSeq() uint64 {
	s.frameSeqMu.RLock()
	defer s.frameSeqMu.RUnlock()
	return s.frameSeq
}

// extractJPEGFrame extracts a complete JPEG frame from buffer
func extractJPEGFrame(buffer *[]byte) []byte {
	if len(*buffer) < 4 {
		return nil
	}

	// Find JPEG start marker (FFD8)
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

	// Find JPEG end marker (FFD9)
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

	// Extract frame
	frame := make([]byte, endIdx-startIdx)
	copy(frame, (*buffer)[startIdx:endIdx])
	*buffer = (*buffer)[endIdx:]

	return frame
}

// ServeHTTP serves the MJPEG stream to a client
func (s *MJPEGStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set MJPEG headers
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Create client channel
	clientCh := make(chan []byte, 5)
	s.clientsMu.Lock()
	s.clients[clientCh] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, clientCh)
		s.clientsMu.Unlock()
	}()

	// Get flusher for streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Printf("[MJPEGStream] Client connected to camera %s", s.cameraID)

	for {
		select {
		case <-r.Context().Done():
			log.Printf("[MJPEGStream] Client disconnected from camera %s", s.cameraID)
			return
		case frame, ok := <-clientCh:
			if !ok {
				return
			}

			// Direct pass-through - frames come from either:
			// 1. Raw camera feed (updateFrame) when no detection is active
			// 2. Detection pipeline (SetAnnotatedFrame) with bounding boxes already drawn
			// No mixing, no staleness checks - what arrives is what gets displayed

			// Write MJPEG frame
			fmt.Fprintf(w, "--frame\r\n")
			fmt.Fprintf(w, "Content-Type: image/jpeg\r\n")
			fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(frame))
			w.Write(frame)
			fmt.Fprintf(w, "\r\n")
			flusher.Flush()
		}
	}
}

// drawOverlays draws detection boxes on a JPEG frame
func (s *MJPEGStream) drawOverlays(jpegData []byte, detections []Detection, faces []FaceDetection) []byte {
	// Decode JPEG
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return jpegData
	}

	// Convert to RGBA for drawing
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// Draw object detections
	for _, det := range detections {
		s.drawBox(rgba, det.X, det.Y, det.W, det.H, det.Color, 2)
		label := fmt.Sprintf("%s %.0f%%", det.Class, det.Confidence*100)
		s.drawLabel(rgba, det.X, det.Y-5, label, det.Color)
	}

	// Draw face detections
	for _, face := range faces {
		var boxColor color.RGBA
		if face.IsKnown {
			boxColor = color.RGBA{0, 255, 0, 255} // Green for known
		} else {
			boxColor = color.RGBA{255, 165, 0, 255} // Orange for unknown
		}
		s.drawBox(rgba, face.X, face.Y, face.W, face.H, boxColor, 2)

		label := face.Identity
		if label == "" {
			label = "Unknown"
		}
		if face.Confidence > 0 {
			label = fmt.Sprintf("%s %.0f%%", label, face.Confidence*100)
		}
		s.drawLabel(rgba, face.X, face.Y-5, label, boxColor)
	}

	// Encode back to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 85}); err != nil {
		return jpegData
	}

	return buf.Bytes()
}

// drawBox draws a rectangle on the image
func (s *MJPEGStream) drawBox(img *image.RGBA, x, y, w, h int, c color.RGBA, thickness int) {
	bounds := img.Bounds()

	for t := 0; t < thickness; t++ {
		// Top edge
		for i := x; i < x+w && i < bounds.Max.X; i++ {
			if y+t >= 0 && y+t < bounds.Max.Y && i >= 0 {
				img.Set(i, y+t, c)
			}
		}
		// Bottom edge
		for i := x; i < x+w && i < bounds.Max.X; i++ {
			if y+h-t >= 0 && y+h-t < bounds.Max.Y && i >= 0 {
				img.Set(i, y+h-t, c)
			}
		}
		// Left edge
		for j := y; j < y+h && j < bounds.Max.Y; j++ {
			if x+t >= 0 && x+t < bounds.Max.X && j >= 0 {
				img.Set(x+t, j, c)
			}
		}
		// Right edge
		for j := y; j < y+h && j < bounds.Max.Y; j++ {
			if x+w-t >= 0 && x+w-t < bounds.Max.X && j >= 0 {
				img.Set(x+w-t, j, c)
			}
		}
	}
}

// drawLabel draws text on the image
func (s *MJPEGStream) drawLabel(img *image.RGBA, x, y int, label string, c color.RGBA) {
	if y < 10 {
		y = 10
	}
	if x < 0 {
		x = 0
	}

	// Draw background rectangle for text
	bgColor := color.RGBA{0, 0, 0, 180}
	textWidth := len(label) * 7
	for dy := -2; dy < 12; dy++ {
		for dx := -2; dx < textWidth+2; dx++ {
			px, py := x+dx, y+dy
			if px >= 0 && px < img.Bounds().Max.X && py >= 0 && py < img.Bounds().Max.Y {
				img.Set(px, py, bgColor)
			}
		}
	}

	// Draw text
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y + 10)},
	}
	d.DrawString(label)
}

// SnapshotHandler serves single frame snapshots
type SnapshotHandler struct {
	manager *MJPEGStreamManager
}

// NewSnapshotHandler creates a new snapshot handler
func NewSnapshotHandler(manager *MJPEGStreamManager) *SnapshotHandler {
	return &SnapshotHandler{manager: manager}
}

// ServeHTTP serves a single JPEG snapshot
func (h *SnapshotHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract camera_id from path: /video/snapshot/{camera_id}
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	cameraID := pathParts[len(pathParts)-1]

	stream := h.manager.GetStream(cameraID)
	if stream == nil {
		http.Error(w, fmt.Sprintf("Stream not found for camera %s", cameraID), http.StatusNotFound)
		return
	}

	frame := stream.GetCurrentFrame()
	if frame == nil {
		http.Error(w, "No frame available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(frame)))
	w.Write(frame)
}
