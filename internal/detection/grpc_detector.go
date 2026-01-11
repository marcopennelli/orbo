package detection

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	pb "orbo/api/proto/detection/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// GRPCDetector provides gRPC-based object detection using YOLO
// Uses bidirectional streaming for low-latency real-time detection
type GRPCDetector struct {
	endpoint   string
	conn       *grpc.ClientConn
	client     pb.DetectionServiceClient
	stream     pb.DetectionService_DetectStreamClient
	streamMu   sync.Mutex
	enabled    bool
	healthy    bool
	healthMu   sync.RWMutex
	lastHealth time.Time
	drawBoxes  bool

	// Stream management
	streamCtx    context.Context
	streamCancel context.CancelFunc
	responsesCh  chan *pb.DetectionResponse
	requestsCh   chan *pb.FrameRequest
	wg           sync.WaitGroup
}

// GRPCDetectorConfig holds configuration for the gRPC detector
type GRPCDetectorConfig struct {
	Endpoint      string
	DrawBoxes     bool
	ConfThreshold float32
}

// NewGRPCDetector creates a new gRPC-based detector
func NewGRPCDetector(config GRPCDetectorConfig) (*GRPCDetector, error) {
	gd := &GRPCDetector{
		endpoint:    config.Endpoint,
		enabled:     true,
		drawBoxes:   config.DrawBoxes,
		responsesCh: make(chan *pb.DetectionResponse, 10),
		requestsCh:  make(chan *pb.FrameRequest, 10),
	}

	if err := gd.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to detection service: %w", err)
	}

	return gd, nil
}

// connect establishes the gRPC connection
func (gd *GRPCDetector) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Configure keepalive to detect dead connections quickly
	kacp := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.DialContext(ctx, gd.endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	gd.conn = conn
	gd.client = pb.NewDetectionServiceClient(conn)

	log.Printf("[GRPCDetector] Connected to %s", gd.endpoint)
	return nil
}

// startStream initializes the bidirectional streaming RPC
func (gd *GRPCDetector) startStream() error {
	gd.streamMu.Lock()
	defer gd.streamMu.Unlock()

	if gd.stream != nil {
		return nil // Already have a stream
	}

	gd.streamCtx, gd.streamCancel = context.WithCancel(context.Background())

	stream, err := gd.client.DetectStream(gd.streamCtx)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	gd.stream = stream

	// Start goroutines to handle send/receive
	gd.wg.Add(2)
	go gd.sendLoop()
	go gd.recvLoop()

	log.Printf("[GRPCDetector] Stream started")
	return nil
}

// sendLoop sends frame requests to the stream
func (gd *GRPCDetector) sendLoop() {
	defer gd.wg.Done()

	for {
		select {
		case <-gd.streamCtx.Done():
			return
		case req := <-gd.requestsCh:
			gd.streamMu.Lock()
			stream := gd.stream
			gd.streamMu.Unlock()

			if stream == nil {
				continue
			}

			if err := stream.Send(req); err != nil {
				log.Printf("[GRPCDetector] Send error: %v", err)
				gd.resetStream()
				return
			}
		}
	}
}

// recvLoop receives detection responses from the stream
func (gd *GRPCDetector) recvLoop() {
	defer gd.wg.Done()

	for {
		gd.streamMu.Lock()
		stream := gd.stream
		gd.streamMu.Unlock()

		if stream == nil {
			select {
			case <-gd.streamCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		resp, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[GRPCDetector] Stream ended")
			gd.resetStream()
			return
		}
		if err != nil {
			log.Printf("[GRPCDetector] Recv error: %v", err)
			gd.resetStream()
			return
		}

		select {
		case gd.responsesCh <- resp:
		default:
			// Drop response if channel full (consumer too slow)
			log.Printf("[GRPCDetector] Dropping response, channel full")
		}
	}
}

// resetStream cleans up and prepares for reconnection
func (gd *GRPCDetector) resetStream() {
	gd.streamMu.Lock()
	defer gd.streamMu.Unlock()

	if gd.streamCancel != nil {
		gd.streamCancel()
	}
	gd.stream = nil
}

// IsHealthy checks if the gRPC detection service is available
func (gd *GRPCDetector) IsHealthy() bool {
	gd.healthMu.RLock()
	if time.Since(gd.lastHealth) < 30*time.Second && gd.healthy {
		gd.healthMu.RUnlock()
		return true
	}
	gd.healthMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := gd.client.HealthCheck(ctx, &pb.HealthRequest{})
	if err != nil {
		log.Printf("[GRPCDetector] Health check failed: %v", err)
		gd.healthMu.Lock()
		gd.healthy = false
		gd.healthMu.Unlock()
		return false
	}

	gd.healthMu.Lock()
	gd.healthy = resp.Status == "healthy" && resp.ModelLoaded
	gd.lastHealth = time.Now()
	gd.healthMu.Unlock()

	return gd.healthy
}

// DetectSecurityObjects performs object detection on a frame using the streaming RPC
// This is the high-performance path for real-time detection
func (gd *GRPCDetector) DetectSecurityObjects(imageData []byte, confThreshold float32) (*SecurityDetectionResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("gRPC detection service unavailable")
	}

	// Ensure stream is started
	if err := gd.startStream(); err != nil {
		return nil, err
	}

	// Create request
	req := &pb.FrameRequest{
		CameraId:       "default", // Will be overwritten by caller
		FrameSeq:       0,
		TimestampNs:    time.Now().UnixNano(),
		JpegData:       imageData,
		ReturnAnnotated: gd.drawBoxes,
		ConfThreshold:  confThreshold,
		EnableTracking: true,
	}

	// Send request
	select {
	case gd.requestsCh <- req:
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("send timeout")
	}

	// Wait for response
	select {
	case resp := <-gd.responsesCh:
		return gd.convertResponse(resp), nil
	case <-time.After(500 * time.Millisecond):
		return nil, fmt.Errorf("response timeout")
	}
}

// DetectSecurityObjectsAnnotated performs detection and returns annotated image
func (gd *GRPCDetector) DetectSecurityObjectsAnnotated(imageData []byte, confThreshold float32) (*AnnotatedSecurityResult, error) {
	// For annotated results, use unary call to guarantee we get the image back
	return gd.DetectSecurityObjectsAnnotatedUnary(imageData, confThreshold)
}

// DetectSecurityObjectsAnnotatedUnary performs a single detection with annotated image
// This is slower than streaming but guarantees we get the annotated image back
func (gd *GRPCDetector) DetectSecurityObjectsAnnotatedUnary(imageData []byte, confThreshold float32) (*AnnotatedSecurityResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("gRPC detection service unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// For unary call, we create a one-shot stream
	stream, err := gd.client.DetectStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	req := &pb.FrameRequest{
		CameraId:       "default",
		FrameSeq:       0,
		TimestampNs:    time.Now().UnixNano(),
		JpegData:       imageData,
		ReturnAnnotated: true,
		ConfThreshold:  confThreshold,
		EnableTracking: true,
	}

	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("send failed: %w", err)
	}

	// Signal we're done sending
	if err := stream.CloseSend(); err != nil {
		log.Printf("[GRPCDetector] CloseSend error: %v", err)
	}

	// Receive response
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("recv failed: %w", err)
	}

	return gd.convertAnnotatedResponse(resp), nil
}

// convertResponse converts gRPC response to internal format
func (gd *GRPCDetector) convertResponse(resp *pb.DetectionResponse) *SecurityDetectionResult {
	detections := make([]Detection, 0, len(resp.Detections))

	for _, det := range resp.Detections {
		detections = append(detections, Detection{
			Class:      det.ClassName,
			ClassID:    int(det.ClassId),
			Confidence: det.Confidence,
			BBox: []float32{
				det.Bbox.X1,
				det.Bbox.Y1,
				det.Bbox.X2,
				det.Bbox.Y2,
			},
			Center: []float32{
				(det.Bbox.X1 + det.Bbox.X2) / 2,
				(det.Bbox.Y1 + det.Bbox.Y2) / 2,
			},
			Area: (det.Bbox.X2 - det.Bbox.X1) * (det.Bbox.Y2 - det.Bbox.Y1),
		})
	}

	// Categorize by threat level
	threat := gd.categorizeThreat(detections)

	return &SecurityDetectionResult{
		Detections:      detections,
		Count:           len(detections),
		ThreatAnalysis:  threat,
		InferenceTimeMs: resp.InferenceMs,
		Device:          resp.Device,
		SecurityFilter:  []string{"person", "car", "truck", "motorcycle", "bicycle"},
	}
}

// convertAnnotatedResponse converts gRPC response to annotated result
func (gd *GRPCDetector) convertAnnotatedResponse(resp *pb.DetectionResponse) *AnnotatedSecurityResult {
	detections := make([]Detection, 0, len(resp.Detections))

	for _, det := range resp.Detections {
		detections = append(detections, Detection{
			Class:      det.ClassName,
			ClassID:    int(det.ClassId),
			Confidence: det.Confidence,
			BBox: []float32{
				det.Bbox.X1,
				det.Bbox.Y1,
				det.Bbox.X2,
				det.Bbox.Y2,
			},
			Center: []float32{
				(det.Bbox.X1 + det.Bbox.X2) / 2,
				(det.Bbox.Y1 + det.Bbox.Y2) / 2,
			},
			Area: (det.Bbox.X2 - det.Bbox.X1) * (det.Bbox.Y2 - det.Bbox.Y1),
		})
	}

	threat := gd.categorizeThreat(detections)

	return &AnnotatedSecurityResult{
		ImageData:       resp.AnnotatedJpeg,
		Detections:      detections,
		Count:           len(detections),
		InferenceTimeMs: resp.InferenceMs,
		Device:          resp.Device,
		ThreatAnalysis:  &threat,
	}
}

// categorizeThreat categorizes detections by threat level
func (gd *GRPCDetector) categorizeThreat(detections []Detection) ThreatAnalysis {
	threat := ThreatAnalysis{
		HighPriority:   make([]Detection, 0),
		MediumPriority: make([]Detection, 0),
		LowPriority:    make([]Detection, 0),
	}

	for _, det := range detections {
		switch det.Class {
		case "person":
			threat.HighPriority = append(threat.HighPriority, det)
		case "car", "truck", "bus":
			threat.MediumPriority = append(threat.MediumPriority, det)
		case "bicycle", "motorcycle":
			threat.LowPriority = append(threat.LowPriority, det)
		default:
			threat.LowPriority = append(threat.LowPriority, det)
		}
	}

	return threat
}

// SetDrawBoxes enables or disables bounding box drawing
func (gd *GRPCDetector) SetDrawBoxes(enabled bool) {
	gd.drawBoxes = enabled
}

// DrawBoxesEnabled returns whether bounding boxes are enabled
func (gd *GRPCDetector) DrawBoxesEnabled() bool {
	return gd.drawBoxes
}

// GetThreatLevel returns the threat level for a detection
func (gd *GRPCDetector) GetThreatLevel(det Detection) string {
	switch det.Class {
	case "person":
		return "high"
	case "car", "truck", "bus":
		return "medium"
	default:
		return "low"
	}
}

// ShouldAlert returns whether a detection should trigger an alert
func (gd *GRPCDetector) ShouldAlert(det Detection) bool {
	// Alert on persons and vehicles
	switch det.Class {
	case "person", "car", "truck", "bus", "motorcycle":
		return det.Confidence > 0.5
	default:
		return false
	}
}

// Close shuts down the gRPC connection
func (gd *GRPCDetector) Close() error {
	gd.resetStream()
	gd.wg.Wait()

	if gd.conn != nil {
		return gd.conn.Close()
	}
	return nil
}

// Configure updates detection parameters
func (gd *GRPCDetector) Configure(confThreshold float32, enableTracking bool, trackerType string) error {
	return gd.ConfigureWithClasses(confThreshold, enableTracking, trackerType, nil)
}

// ConfigureWithClasses updates detection parameters including class filter
func (gd *GRPCDetector) ConfigureWithClasses(confThreshold float32, enableTracking bool, trackerType string, classes []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.ConfigureRequest{
		ConfThreshold:  &confThreshold,
		EnableTracking: &enableTracking,
		TrackerType:    &trackerType,
		Classes:        classes, // Pass class filter to YOLO service
	}

	resp, err := gd.client.Configure(ctx, req)
	if err != nil {
		return fmt.Errorf("configure failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("configure failed: %s", resp.Message)
	}

	log.Printf("[GRPCDetector] Configuration updated: conf=%.2f tracking=%v tracker=%s classes=%v",
		resp.ConfThreshold, resp.TrackingEnabled, resp.TrackerType, classes)
	return nil
}
