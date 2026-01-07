package stream

import (
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebCodecsStream handles WebSocket streaming for a single camera
// Prioritizes annotated frames (from detection) but falls back to raw frames
// from MJPEG capture when detection is disabled
type WebCodecsStream struct {
	cameraID           string
	running            bool
	mu                 sync.RWMutex
	clients            map[*websocket.Conn]bool
	clientsMu          sync.RWMutex
	stopCh             chan struct{}
	lastAnnotatedTime  time.Time     // When we last received an annotated frame
	lastAnnotatedMu    sync.RWMutex
}

// WebCodecsStreamManager manages WebCodecs streams for all cameras
type WebCodecsStreamManager struct {
	streams       map[string]*WebCodecsStream
	mu            sync.RWMutex
	mjpegManager  *MJPEGStreamManager // Reference to MJPEG for raw frame fallback
}

var webCodecsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 256 * 1024, // 256KB for video frames
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// NewWebCodecsStreamManager creates a new WebCodecs stream manager
func NewWebCodecsStreamManager() *WebCodecsStreamManager {
	return &WebCodecsStreamManager{
		streams: make(map[string]*WebCodecsStream),
	}
}

// SetMJPEGManager sets the MJPEG manager for raw frame fallback
// This allows WebCodecs to receive frames even when detection is disabled
func (m *WebCodecsStreamManager) SetMJPEGManager(mjpeg *MJPEGStreamManager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mjpegManager = mjpeg
}

// CreateStream creates a new WebCodecs stream for a camera
func (m *WebCodecsStreamManager) CreateStream(cameraID, device string, fps, width, height int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.streams[cameraID]; exists {
		return fmt.Errorf("WebCodecs stream already exists for camera %s", cameraID)
	}

	stream := &WebCodecsStream{
		cameraID: cameraID,
		running:  true,
		clients:  make(map[*websocket.Conn]bool),
		stopCh:   make(chan struct{}),
	}

	m.streams[cameraID] = stream

	// Register frame listener with MJPEG stream for raw frame fallback
	// This ensures WebCodecs receives frames even when detection is disabled
	if m.mjpegManager != nil {
		m.mjpegManager.SetFrameListener(cameraID, func(camID string, frame []byte) {
			// Only send raw frames if we have connected clients
			stream.broadcastRawFrame(frame)
		})
		log.Printf("[WebCodecs] Created stream for camera %s with MJPEG raw frame fallback", cameraID)
	} else {
		log.Printf("[WebCodecs] Created stream for camera %s (annotated frames only, no MJPEG fallback)", cameraID)
	}
	return nil
}

// DeleteStream stops and removes a stream
func (m *WebCodecsStreamManager) DeleteStream(cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stream, exists := m.streams[cameraID]
	if !exists {
		return fmt.Errorf("WebCodecs stream not found for camera %s", cameraID)
	}

	stream.Stop()
	delete(m.streams, cameraID)

	log.Printf("[WebCodecs] Deleted stream for camera %s", cameraID)
	return nil
}

// GetStream returns a stream by camera ID
func (m *WebCodecsStreamManager) GetStream(cameraID string) *WebCodecsStream {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.streams[cameraID]
}

// SetAnnotatedFrame broadcasts an annotated JPEG frame to all connected clients
// This is called from the detection pipeline after YOLO/InsightFace processing
func (m *WebCodecsStreamManager) SetAnnotatedFrame(cameraID string, frameData []byte) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.broadcastAnnotatedFrame(frameData)
	}
}

// UpdateDetections - compatibility method (not used for WebCodecs, boxes are pre-drawn)
func (m *WebCodecsStreamManager) UpdateDetections(cameraID string, detections []Detection, faces []FaceDetection) {
	// No-op for WebCodecs - we use pre-annotated frames
}

// GetCurrentFrameSeq - compatibility method
func (m *WebCodecsStreamManager) GetCurrentFrameSeq(cameraID string) uint64 {
	return 0 // Not used for WebCodecs
}

// ServeHTTP handles WebSocket upgrade for WebCodecs streaming
func (m *WebCodecsStreamManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract camera_id from path: /ws/video/{camera_id}
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

	stream.ServeWebSocket(w, r)
}

// Stop stops the stream
func (s *WebCodecsStream) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopCh)
	s.running = false

	// Close all client connections
	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
		delete(s.clients, conn)
	}
	s.clientsMu.Unlock()
}

// broadcastAnnotatedFrame sends an annotated frame to all connected clients
// Called directly from detection pipeline - no channel buffering (real-time)
func (s *WebCodecsStream) broadcastAnnotatedFrame(frameData []byte) {
	if len(frameData) == 0 {
		return
	}

	// Update last annotated time (to suppress raw frames while detection is active)
	s.lastAnnotatedMu.Lock()
	s.lastAnnotatedTime = time.Now()
	s.lastAnnotatedMu.Unlock()

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	if len(s.clients) == 0 {
		return
	}

	// Message format: 1 byte type + 4 bytes length + frame data
	// Type: 1 = annotated frame (with bounding boxes from YOLO/InsightFace)
	msg := make([]byte, 5+len(frameData))
	msg[0] = 1 // Annotated
	binary.BigEndian.PutUint32(msg[1:5], uint32(len(frameData)))
	copy(msg[5:], frameData)

	for conn := range s.clients {
		conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			// Will be cleaned up by read pump
			log.Printf("[WebCodecs] Write error to client: %v", err)
		}
	}
}

// broadcastRawFrame sends a raw frame (from MJPEG capture) when detection is disabled
// Only sends if we haven't received an annotated frame recently (detection is off)
func (s *WebCodecsStream) broadcastRawFrame(frameData []byte) {
	if len(frameData) == 0 {
		return
	}

	// Check if we received an annotated frame recently (within 500ms)
	// If yes, detection is active and we should skip raw frames
	s.lastAnnotatedMu.RLock()
	lastAnnotated := s.lastAnnotatedTime
	s.lastAnnotatedMu.RUnlock()

	if time.Since(lastAnnotated) < 500*time.Millisecond {
		// Detection is active, skip raw frame
		return
	}

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	if len(s.clients) == 0 {
		return
	}

	// Message format: 1 byte type + 4 bytes length + frame data
	// Type: 0 = raw frame (no detection boxes)
	msg := make([]byte, 5+len(frameData))
	msg[0] = 0 // Raw
	binary.BigEndian.PutUint32(msg[1:5], uint32(len(frameData)))
	copy(msg[5:], frameData)

	for conn := range s.clients {
		conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			// Will be cleaned up by read pump
		}
	}
}

// ServeWebSocket handles WebSocket connections for video streaming
func (s *WebCodecsStream) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := webCodecsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebCodecs] Upgrade error: %v", err)
		return
	}

	log.Printf("[WebCodecs] Client connected to camera %s from %s", s.cameraID, r.RemoteAddr)

	// Register client
	s.clientsMu.Lock()
	s.clients[conn] = true
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("[WebCodecs] Camera %s now has %d connected clients", s.cameraID, clientCount)

	// Read pump to detect disconnection and handle ping/pong
	s.readPump(conn)
}

// readPump reads from WebSocket to detect disconnection
func (s *WebCodecsStream) readPump(conn *websocket.Conn) {
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		clientCount := len(s.clients)
		s.clientsMu.Unlock()
		conn.Close()
		log.Printf("[WebCodecs] Client disconnected from camera %s (%d clients remaining)", s.cameraID, clientCount)
	}()

	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Ping ticker to keep connection alive
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
