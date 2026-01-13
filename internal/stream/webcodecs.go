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
	"orbo/internal/pipeline"
)

// ClientMode defines the streaming mode for a WebSocket client
type ClientMode int

const (
	// ModeAuto - receives raw frames when detection is off, annotated when on
	ModeAuto ClientMode = iota
	// ModeRaw - always receives raw frames only
	ModeRaw
)

// WebCodecsClient represents a connected WebSocket client with its mode
type WebCodecsClient struct {
	conn    *websocket.Conn
	mode    ClientMode
	writeMu sync.Mutex // Protects concurrent writes to this connection
}

// WebCodecsStream handles WebSocket streaming for a single camera
// Receives frames from unified frame provider and/or detection pipeline
type WebCodecsStream struct {
	cameraID           string
	running            bool
	mu                 sync.RWMutex
	clients            map[*websocket.Conn]*WebCodecsClient
	clientsMu          sync.RWMutex
	stopCh             chan struct{}
	lastAnnotatedTime  time.Time     // When we last received an annotated frame
	lastAnnotatedMu    sync.RWMutex
	lastAnnotatedSeq   uint64        // Last annotated frame sequence for ordering
	lastAnnotatedSeqMu sync.RWMutex
	rawFrameSeq        uint64        // Sequence counter for raw frames
	rawFrameSeqMu      sync.Mutex

	// Frame provider subscription
	frameSubscription  *pipeline.FrameSubscription
}

// WebCodecsStreamManager manages WebCodecs streams for all cameras
type WebCodecsStreamManager struct {
	streams       map[string]*WebCodecsStream
	mu            sync.RWMutex
	frameProvider pipeline.FrameProvider // Unified frame source
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

// SetFrameProvider sets the unified frame provider for raw frame capture
// This is the preferred way to get frames - single capture, multiple consumers
func (m *WebCodecsStreamManager) SetFrameProvider(provider pipeline.FrameProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frameProvider = provider
	log.Printf("[WebCodecs] Frame provider set")
}

// CreateStream creates a new WebCodecs stream for a camera
// If a FrameProvider is set, it subscribes to frames from there
func (m *WebCodecsStreamManager) CreateStream(cameraID, device string, fps, width, height int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.streams[cameraID]; exists {
		return fmt.Errorf("WebCodecs stream already exists for camera %s", cameraID)
	}

	stream := &WebCodecsStream{
		cameraID: cameraID,
		running:  true,
		clients:  make(map[*websocket.Conn]*WebCodecsClient),
		stopCh:   make(chan struct{}),
	}

	m.streams[cameraID] = stream

	// Subscribe to frame provider if available
	if m.frameProvider != nil {
		sub, err := m.frameProvider.Subscribe(cameraID, 5)
		if err != nil {
			log.Printf("[WebCodecs] Warning: failed to subscribe to frame provider for camera %s: %v", cameraID, err)
		} else {
			stream.frameSubscription = sub
			// Start goroutine to receive frames
			go stream.receiveFrames(sub)
			log.Printf("[WebCodecs] Created stream for camera %s with frame provider subscription", cameraID)
			return nil
		}
	}

	log.Printf("[WebCodecs] Created stream for camera %s (annotated frames only)", cameraID)
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

	// Unsubscribe from frame provider
	if stream.frameSubscription != nil && m.frameProvider != nil {
		m.frameProvider.Unsubscribe(stream.frameSubscription)
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
// seq is the frame sequence number for ordering validation
func (m *WebCodecsStreamManager) SetAnnotatedFrame(cameraID string, seq uint64, frameData []byte) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.broadcastAnnotatedFrame(seq, frameData)
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

// InjectRawFrame pushes a raw frame directly to WebCodecs (for unified frame provider)
// This bypasses MJPEG and sends frames directly when detection is disabled
func (m *WebCodecsStreamManager) InjectRawFrame(cameraID string, frameData []byte) {
	m.mu.RLock()
	stream := m.streams[cameraID]
	m.mu.RUnlock()

	if stream != nil {
		stream.broadcastRawFrame(frameData)
	}
}

// ServeHTTP handles WebSocket upgrade for WebCodecs streaming (auto mode)
func (m *WebCodecsStreamManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.serveWithMode(w, r, ModeAuto)
}

// ServeHTTPRaw handles WebSocket upgrade for raw-only streaming
func (m *WebCodecsStreamManager) ServeHTTPRaw(w http.ResponseWriter, r *http.Request) {
	m.serveWithMode(w, r, ModeRaw)
}

// serveWithMode handles WebSocket upgrade with specified client mode
func (m *WebCodecsStreamManager) serveWithMode(w http.ResponseWriter, r *http.Request, mode ClientMode) {
	// Extract camera_id from path: /ws/video/{camera_id} or /ws/video/raw/{camera_id}
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

	stream.ServeWebSocketWithMode(w, r, mode)
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
	for conn, client := range s.clients {
		_ = client // suppress unused warning
		conn.Close()
		delete(s.clients, conn)
	}
	s.clientsMu.Unlock()
}

// receiveFrames receives frames from the FrameProvider subscription
// and broadcasts them as raw frames when detection is not active
func (s *WebCodecsStream) receiveFrames(sub *pipeline.FrameSubscription) {
	log.Printf("[WebCodecs] Starting frame receiver for camera %s", s.cameraID)

	for {
		select {
		case <-s.stopCh:
			log.Printf("[WebCodecs] Stopping frame receiver for camera %s", s.cameraID)
			return
		case <-sub.Done:
			log.Printf("[WebCodecs] Frame subscription ended for camera %s", s.cameraID)
			return
		case frame := <-sub.Channel:
			if frame != nil && len(frame.Data) > 0 {
				// Broadcast raw frame (will be suppressed if detection is active)
				s.broadcastRawFrame(frame.Data)
			}
		}
	}
}

// broadcastAnnotatedFrame sends an annotated frame to auto-mode clients only
// Called directly from detection pipeline - no channel buffering (real-time)
// Raw-mode clients don't receive annotated frames
func (s *WebCodecsStream) broadcastAnnotatedFrame(seq uint64, frameData []byte) {
	if len(frameData) == 0 {
		return
	}

	// Update last annotated time (to suppress raw frames for auto-mode clients)
	s.lastAnnotatedMu.Lock()
	s.lastAnnotatedTime = time.Now()
	s.lastAnnotatedMu.Unlock()

	// Update sequence (used by client for ordering, but we don't drop frames server-side
	// since annotated frames from detection are already sequential from the pipeline)
	s.lastAnnotatedSeqMu.Lock()
	s.lastAnnotatedSeq = seq
	s.lastAnnotatedSeqMu.Unlock()

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	if len(s.clients) == 0 {
		return
	}

	// Message format: 1 byte type + 8 bytes sequence + 4 bytes length + frame data
	// Type: 1 = annotated frame (with bounding boxes from YOLO/InsightFace)
	msg := make([]byte, 13+len(frameData))
	msg[0] = 1 // Annotated
	binary.BigEndian.PutUint64(msg[1:9], seq)
	binary.BigEndian.PutUint32(msg[9:13], uint32(len(frameData)))
	copy(msg[13:], frameData)

	// Only send to auto-mode clients (raw-mode clients never get annotated frames)
	for _, client := range s.clients {
		if client.mode != ModeAuto {
			continue
		}
		// Use per-client mutex to prevent concurrent writes
		client.writeMu.Lock()
		client.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		err := client.conn.WriteMessage(websocket.BinaryMessage, msg)
		client.writeMu.Unlock()
		if err != nil {
			// Will be cleaned up by read pump
			log.Printf("[WebCodecs] Write error to client: %v", err)
		}
	}
}

// broadcastRawFrame sends a raw frame to clients
// - Raw-mode clients: always receive raw frames
// - Auto-mode clients: only receive raw frames when detection is not active
// YOLO11-style: When detection is enabled, auto-mode clients get ONLY annotated frames
// to avoid time-travel effect (mixing delayed annotated frames with real-time raw frames)
func (s *WebCodecsStream) broadcastRawFrame(frameData []byte) {
	if len(frameData) == 0 {
		return
	}

	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	if len(s.clients) == 0 {
		return
	}

	// Check if detection is active (for auto-mode client filtering)
	// YOLO11-style: We use a 5-second timeout to determine if detection pipeline is running
	// This must be longer than the worst-case inference time (face detection can take 2+ seconds)
	// to prevent raw frames from being shown during slow inference
	s.lastAnnotatedMu.RLock()
	lastAnnotated := s.lastAnnotatedTime
	s.lastAnnotatedMu.RUnlock()
	detectionActive := time.Since(lastAnnotated) < 5*time.Second

	// Increment raw frame sequence
	s.rawFrameSeqMu.Lock()
	s.rawFrameSeq++
	seq := s.rawFrameSeq
	s.rawFrameSeqMu.Unlock()

	// Message format: 1 byte type + 8 bytes sequence + 4 bytes length + frame data
	// Type: 0 = raw frame (no detection boxes)
	msg := make([]byte, 13+len(frameData))
	msg[0] = 0 // Raw
	binary.BigEndian.PutUint64(msg[1:9], seq)
	binary.BigEndian.PutUint32(msg[9:13], uint32(len(frameData)))
	copy(msg[13:], frameData)

	for _, client := range s.clients {
		// Raw-mode clients always get raw frames
		// Auto-mode clients only get raw frames when detection is not active
		if client.mode == ModeAuto && detectionActive {
			continue
		}
		// Use per-client mutex to prevent concurrent writes
		client.writeMu.Lock()
		client.conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		err := client.conn.WriteMessage(websocket.BinaryMessage, msg)
		client.writeMu.Unlock()
		if err != nil {
			// Will be cleaned up by read pump
		}
	}
}

// ServeWebSocket handles WebSocket connections for video streaming (auto mode)
func (s *WebCodecsStream) ServeWebSocket(w http.ResponseWriter, r *http.Request) {
	s.ServeWebSocketWithMode(w, r, ModeAuto)
}

// ServeWebSocketWithMode handles WebSocket connections with specified mode
func (s *WebCodecsStream) ServeWebSocketWithMode(w http.ResponseWriter, r *http.Request, mode ClientMode) {
	conn, err := webCodecsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebCodecs] Upgrade error: %v", err)
		return
	}

	modeStr := "auto"
	if mode == ModeRaw {
		modeStr = "raw"
	}
	log.Printf("[WebCodecs] Client connected to camera %s from %s (mode: %s)", s.cameraID, r.RemoteAddr, modeStr)

	// Register client with mode
	client := &WebCodecsClient{
		conn: conn,
		mode: mode,
	}

	s.clientsMu.Lock()
	s.clients[conn] = client
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("[WebCodecs] Camera %s now has %d connected clients", s.cameraID, clientCount)

	// Read pump to detect disconnection and handle ping/pong
	s.readPump(conn)
}

// readPump reads from WebSocket to detect disconnection
func (s *WebCodecsStream) readPump(conn *websocket.Conn) {
	// Get client reference for write mutex
	s.clientsMu.RLock()
	client := s.clients[conn]
	s.clientsMu.RUnlock()

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
				if client == nil {
					return
				}
				// Use per-client mutex to prevent concurrent writes
				client.writeMu.Lock()
				client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := client.conn.WriteMessage(websocket.PingMessage, nil)
				client.writeMu.Unlock()
				if err != nil {
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
