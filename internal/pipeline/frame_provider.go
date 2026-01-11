package pipeline

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FFmpegFrameProvider captures frames from cameras using FFmpeg
// and broadcasts to multiple subscribers
type FFmpegFrameProvider struct {
	cameras map[string]*cameraCapture
	mu      sync.RWMutex
}

// cameraCapture handles frame capture for a single camera
type cameraCapture struct {
	cameraID    string
	device      string
	fps         int
	width       int
	height      int
	running     atomic.Bool
	stopCh      chan struct{}
	cmd         *exec.Cmd
	subscribers map[*FrameSubscription]bool
	subMu       sync.RWMutex
	frameSeq    atomic.Uint64
	stats       *CaptureStats
	statsMu     sync.RWMutex
}

// NewFFmpegFrameProvider creates a new FFmpeg-based frame provider
func NewFFmpegFrameProvider() *FFmpegFrameProvider {
	return &FFmpegFrameProvider{
		cameras: make(map[string]*cameraCapture),
	}
}

func (p *FFmpegFrameProvider) Start(cameraID string, device string, fps int, width int, height int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.cameras[cameraID]; exists {
		return fmt.Errorf("camera %s already started", cameraID)
	}

	capture := &cameraCapture{
		cameraID:    cameraID,
		device:      device,
		fps:         fps,
		width:       width,
		height:      height,
		stopCh:      make(chan struct{}),
		subscribers: make(map[*FrameSubscription]bool),
		stats: &CaptureStats{
			CameraID: cameraID,
		},
	}

	p.cameras[cameraID] = capture

	go capture.run()

	log.Printf("[FrameProvider] Started capture for camera %s (device: %s, fps: %d)", cameraID, device, fps)
	return nil
}

func (p *FFmpegFrameProvider) Stop(cameraID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	capture, exists := p.cameras[cameraID]
	if !exists {
		return fmt.Errorf("camera %s not found", cameraID)
	}

	capture.stop()
	delete(p.cameras, cameraID)

	log.Printf("[FrameProvider] Stopped capture for camera %s", cameraID)
	return nil
}

func (p *FFmpegFrameProvider) Subscribe(cameraID string, bufferSize int) (*FrameSubscription, error) {
	p.mu.RLock()
	capture, exists := p.cameras[cameraID]
	p.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("camera %s not found", cameraID)
	}

	if bufferSize <= 0 {
		bufferSize = 5
	}

	sub := &FrameSubscription{
		CameraID: cameraID,
		Channel:  make(chan *FrameData, bufferSize),
		Done:     make(chan struct{}),
	}

	capture.subMu.Lock()
	capture.subscribers[sub] = true
	capture.subMu.Unlock()

	log.Printf("[FrameProvider] New subscriber for camera %s (total: %d)", cameraID, len(capture.subscribers))
	return sub, nil
}

func (p *FFmpegFrameProvider) Unsubscribe(sub *FrameSubscription) {
	if sub == nil {
		return
	}

	p.mu.RLock()
	capture, exists := p.cameras[sub.CameraID]
	p.mu.RUnlock()

	if !exists {
		return
	}

	capture.subMu.Lock()
	if _, ok := capture.subscribers[sub]; ok {
		delete(capture.subscribers, sub)
		close(sub.Done)
	}
	capture.subMu.Unlock()

	log.Printf("[FrameProvider] Unsubscribed from camera %s (remaining: %d)", sub.CameraID, len(capture.subscribers))
}

func (p *FFmpegFrameProvider) IsRunning(cameraID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	capture, exists := p.cameras[cameraID]
	if !exists {
		return false
	}
	return capture.running.Load()
}

func (p *FFmpegFrameProvider) GetStats(cameraID string) *CaptureStats {
	p.mu.RLock()
	capture, exists := p.cameras[cameraID]
	p.mu.RUnlock()

	if !exists {
		return nil
	}

	capture.statsMu.RLock()
	defer capture.statsMu.RUnlock()

	// Return a copy
	stats := *capture.stats
	return &stats
}

// run starts the frame capture loop
func (c *cameraCapture) run() {
	c.running.Store(true)
	defer c.running.Store(false)

	log.Printf("[FrameProvider] Starting capture loop for camera %s", c.cameraID)

	// Check if it's an HTTP image endpoint (polling mode)
	if c.isHTTPImageEndpoint() {
		c.captureHTTPImages()
		return
	}

	// Use FFmpeg for streaming sources
	c.captureFFmpeg()
}

func (c *cameraCapture) stop() {
	close(c.stopCh)

	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}

	// Close all subscriber channels
	c.subMu.Lock()
	for sub := range c.subscribers {
		close(sub.Done)
		delete(c.subscribers, sub)
	}
	c.subMu.Unlock()
}

func (c *cameraCapture) isHTTPImageEndpoint() bool {
	return (strings.HasPrefix(c.device, "http://") || strings.HasPrefix(c.device, "https://")) &&
		(strings.Contains(c.device, ".jpg") || strings.Contains(c.device, ".jpeg") || strings.Contains(c.device, "image"))
}

func (c *cameraCapture) captureHTTPImages() {
	client := &http.Client{Timeout: 10 * time.Second}
	interval := time.Second / time.Duration(c.fps)
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			resp, err := client.Get(c.device)
			if err != nil {
				log.Printf("[FrameProvider] Error fetching frame from %s: %v", c.device, err)
				continue
			}

			frame, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("[FrameProvider] Error reading frame: %v", err)
				continue
			}

			c.broadcastFrame(frame)
		}
	}
}

func (c *cameraCapture) captureFFmpeg() {
	var args []string

	if strings.HasPrefix(c.device, "rtsp://") {
		args = []string{
			"-rtsp_transport", "tcp",
			"-i", c.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-r", fmt.Sprintf("%d", c.fps),
			"-q:v", "5",
			"-",
		}
	} else if strings.HasPrefix(c.device, "http://") || strings.HasPrefix(c.device, "https://") {
		args = []string{
			"-i", c.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-r", fmt.Sprintf("%d", c.fps),
			"-q:v", "5",
			"-",
		}
	} else {
		// V4L2 device (USB camera)
		args = []string{
			"-f", "v4l2",
			"-video_size", fmt.Sprintf("%dx%d", c.width, c.height),
			"-framerate", fmt.Sprintf("%d", c.fps),
			"-i", c.device,
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "5",
			"-",
		}
	}

	c.cmd = exec.Command("ffmpeg", args...)

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		log.Printf("[FrameProvider] Error creating stdout pipe: %v", err)
		return
	}

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		log.Printf("[FrameProvider] Error creating stderr pipe: %v", err)
		return
	}

	if err := c.cmd.Start(); err != nil {
		log.Printf("[FrameProvider] Error starting ffmpeg: %v", err)
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
		case <-c.stopCh:
			return
		default:
			n, err := stdout.Read(chunk)
			if err != nil {
				if err != io.EOF {
					log.Printf("[FrameProvider] Error reading frame: %v", err)
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
				c.broadcastFrame(frame)
			}
		}
	}
}

func (c *cameraCapture) broadcastFrame(data []byte) {
	seq := c.frameSeq.Add(1)
	now := time.Now()

	frame := &FrameData{
		CameraID:  c.cameraID,
		Data:      data,
		Seq:       seq,
		Timestamp: now,
		Width:     c.width,
		Height:    c.height,
	}

	// Update stats
	c.statsMu.Lock()
	c.stats.FramesCaptured++
	c.stats.LastFrameTime = now.Unix()
	c.statsMu.Unlock()

	// Broadcast to all subscribers
	c.subMu.RLock()
	for sub := range c.subscribers {
		select {
		case sub.Channel <- frame:
		default:
			// Subscriber is slow, drop frame
			c.statsMu.Lock()
			c.stats.FramesDropped++
			c.statsMu.Unlock()
		}
	}
	c.subMu.RUnlock()

	// Log progress every 100 frames
	if seq%100 == 0 {
		c.subMu.RLock()
		subCount := len(c.subscribers)
		c.subMu.RUnlock()
		log.Printf("[FrameProvider] Camera %s: frame %d, %d subscribers", c.cameraID, seq, subCount)
	}
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

// Ensure FFmpegFrameProvider implements FrameProvider
var _ FrameProvider = (*FFmpegFrameProvider)(nil)
