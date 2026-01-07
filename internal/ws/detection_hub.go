package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DetectionHub manages WebSocket connections for real-time detection streaming
type DetectionHub struct {
	// clients maps camera_id -> set of connections
	clients map[string]map[*websocket.Conn]bool
	mu      sync.RWMutex
}

// NewDetectionHub creates a new detection hub
func NewDetectionHub() *DetectionHub {
	return &DetectionHub{
		clients: make(map[string]map[*websocket.Conn]bool),
	}
}

// Register adds a connection for a specific camera
func (h *DetectionHub) Register(cameraID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[cameraID] == nil {
		h.clients[cameraID] = make(map[*websocket.Conn]bool)
	}
	h.clients[cameraID][conn] = true
	fmt.Printf("[WS] Client registered for camera %s (total: %d)\n", cameraID, len(h.clients[cameraID]))
}

// Unregister removes a connection for a specific camera
func (h *DetectionHub) Unregister(cameraID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if conns, ok := h.clients[cameraID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.clients, cameraID)
		}
		fmt.Printf("[WS] Client unregistered for camera %s\n", cameraID)
	}
}

// HasClients returns true if there are any clients connected for a camera
func (h *DetectionHub) HasClients(cameraID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conns, ok := h.clients[cameraID]
	return ok && len(conns) > 0
}

// GetRegisteredCameras returns all camera IDs with clients (for debugging)
func (h *DetectionHub) GetRegisteredCameras() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	cameras := make([]string, 0, len(h.clients))
	for cameraID := range h.clients {
		cameras = append(cameras, cameraID)
	}
	return cameras
}

// BroadcastToCamera sends a message to all clients subscribed to a camera
func (h *DetectionHub) BroadcastToCamera(cameraID string, message []byte) {
	h.mu.RLock()
	conns := h.clients[cameraID]
	h.mu.RUnlock()

	if len(conns) == 0 {
		return
	}

	for conn := range conns {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err := conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			fmt.Printf("[WS] Error sending to client: %v\n", err)
			h.Unregister(cameraID, conn)
			conn.Close()
		}
	}
}

// BroadcastDetection sends an object detection message to camera subscribers
func (h *DetectionHub) BroadcastDetection(cameraID string, msg *DetectionMessage) {
	if !h.HasClients(cameraID) {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[WS] Error marshaling detection message: %v\n", err)
		return
	}
	h.BroadcastToCamera(cameraID, data)
}

// BroadcastFaces sends a face recognition message to camera subscribers
func (h *DetectionHub) BroadcastFaces(cameraID string, msg *FaceMessage) {
	if !h.HasClients(cameraID) {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[WS] Error marshaling face message: %v\n", err)
		return
	}
	h.BroadcastToCamera(cameraID, data)
}

// BroadcastFrame sends a video frame to camera subscribers (for live streaming)
func (h *DetectionHub) BroadcastFrame(cameraID string, msg *FrameMessage) {
	if !h.HasClients(cameraID) {
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("[WS] Error marshaling frame message: %v\n", err)
		return
	}
	h.BroadcastToCamera(cameraID, data)
}

// ClientCount returns the total number of connected clients
func (h *DetectionHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, conns := range h.clients {
		count += len(conns)
	}
	return count
}
