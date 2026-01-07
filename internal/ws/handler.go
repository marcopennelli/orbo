package ws

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 256 * 1024, // 256KB for base64 encoded JPEG frames
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for development
		// In production, this should be more restrictive
		return true
	},
}

// Handler handles WebSocket connections for real-time detections
type Handler struct {
	hub *DetectionHub
}

// NewHandler creates a new WebSocket handler
func NewHandler(hub *DetectionHub) *Handler {
	return &Handler{hub: hub}
}

// ServeHTTP handles WebSocket upgrade requests
// Expected URL format: /ws/detections/{camera_id}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract camera_id from URL path
	// Path: /ws/detections/{camera_id}
	path := strings.TrimPrefix(r.URL.Path, "/ws/detections/")
	cameraID := strings.TrimSuffix(path, "/")

	if cameraID == "" {
		http.Error(w, "camera_id required", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("[WS] Upgrade error: %v\n", err)
		return
	}

	fmt.Printf("[WS] New connection for camera %s from %s\n", cameraID, r.RemoteAddr)

	// Register the connection
	h.hub.Register(cameraID, conn)

	// Start goroutine to handle incoming messages and keep connection alive
	go h.readPump(cameraID, conn)
}

// readPump reads messages from the WebSocket connection
// This keeps the connection alive and handles client disconnection
func (h *Handler) readPump(cameraID string, conn *websocket.Conn) {
	defer func() {
		h.hub.Unregister(cameraID, conn)
		conn.Close()
	}()

	// Configure connection
	conn.SetReadLimit(512) // Small limit since client shouldn't send much
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Ping goroutine
	go func() {
		for range ticker.C {
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	// Read loop - mainly to detect disconnection
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				fmt.Printf("[WS] Read error for camera %s: %v\n", cameraID, err)
			}
			break
		}
	}
}
