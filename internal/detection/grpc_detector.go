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
// Supports YOLO11 multi-task analysis: detect, pose, segment, obb, classify
type GRPCDetector struct {
	endpoint   string
	conn       *grpc.ClientConn
	client     pb.DetectionServiceClient
	streamMu   sync.Mutex
	enabled    bool
	healthy    bool
	healthMu   sync.RWMutex
	lastHealth time.Time
	drawBoxes  bool
	tasks      []string // YOLO11 tasks: detect, pose, segment, obb, classify
	tasksMu    sync.RWMutex

	// AnalyzeStream for multi-task detection
	analyzeStream     pb.DetectionService_AnalyzeStreamClient
	analyzeStreamCtx  context.Context
	analyzeCancel     context.CancelFunc
	analyzeResponseCh chan *pb.AnalyzeResponse
	analyzeRequestCh  chan *pb.AnalyzeRequest
	wg                sync.WaitGroup
}

// GRPCDetectorConfig holds configuration for the gRPC detector
type GRPCDetectorConfig struct {
	Endpoint      string
	DrawBoxes     bool
	ConfThreshold float32
	Tasks         []string // YOLO11 tasks: detect, pose, segment, obb, classify
}

// NewGRPCDetector creates a new gRPC-based detector
func NewGRPCDetector(config GRPCDetectorConfig) (*GRPCDetector, error) {
	// Default to detect if no tasks specified
	tasks := config.Tasks
	if len(tasks) == 0 {
		tasks = []string{"detect"}
	}

	gd := &GRPCDetector{
		endpoint:          config.Endpoint,
		enabled:           true,
		drawBoxes:         config.DrawBoxes,
		tasks:             tasks,
		analyzeResponseCh: make(chan *pb.AnalyzeResponse, 10),
		analyzeRequestCh:  make(chan *pb.AnalyzeRequest, 10),
	}

	if err := gd.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to detection service: %w", err)
	}

	log.Printf("[GRPCDetector] Initialized with tasks: %v", tasks)
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

// startStream initializes the bidirectional AnalyzeStream RPC for multi-task detection
func (gd *GRPCDetector) startStream() error {
	gd.streamMu.Lock()
	defer gd.streamMu.Unlock()

	if gd.analyzeStream != nil {
		return nil // Already have a stream
	}

	gd.analyzeStreamCtx, gd.analyzeCancel = context.WithCancel(context.Background())

	stream, err := gd.client.AnalyzeStream(gd.analyzeStreamCtx)
	if err != nil {
		return fmt.Errorf("failed to start analyze stream: %w", err)
	}

	gd.analyzeStream = stream

	// Start goroutines to handle send/receive
	gd.wg.Add(2)
	go gd.sendLoop()
	go gd.recvLoop()

	gd.tasksMu.RLock()
	tasks := gd.tasks
	gd.tasksMu.RUnlock()
	log.Printf("[GRPCDetector] AnalyzeStream started with tasks: %v", tasks)
	return nil
}

// sendLoop sends AnalyzeRequest to the stream
func (gd *GRPCDetector) sendLoop() {
	defer gd.wg.Done()

	for {
		select {
		case <-gd.analyzeStreamCtx.Done():
			return
		case req := <-gd.analyzeRequestCh:
			gd.streamMu.Lock()
			stream := gd.analyzeStream
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

// recvLoop receives AnalyzeResponse from the stream
func (gd *GRPCDetector) recvLoop() {
	defer gd.wg.Done()

	for {
		gd.streamMu.Lock()
		stream := gd.analyzeStream
		gd.streamMu.Unlock()

		if stream == nil {
			select {
			case <-gd.analyzeStreamCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		resp, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[GRPCDetector] AnalyzeStream ended")
			gd.resetStream()
			return
		}
		if err != nil {
			log.Printf("[GRPCDetector] Recv error: %v", err)
			gd.resetStream()
			return
		}

		select {
		case gd.analyzeResponseCh <- resp:
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

	if gd.analyzeCancel != nil {
		gd.analyzeCancel()
	}
	gd.analyzeStream = nil
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

// taskStringToProto converts a task string to its proto enum value
func taskStringToProto(task string) pb.YoloTask {
	switch task {
	case "detect":
		return pb.YoloTask_YOLO_TASK_DETECT
	case "pose":
		return pb.YoloTask_YOLO_TASK_POSE
	case "segment":
		return pb.YoloTask_YOLO_TASK_SEGMENT
	case "obb":
		return pb.YoloTask_YOLO_TASK_OBB
	case "classify":
		return pb.YoloTask_YOLO_TASK_CLASSIFY
	default:
		return pb.YoloTask_YOLO_TASK_DETECT
	}
}

// getTasks returns the current configured tasks as proto enum values
func (gd *GRPCDetector) getTasks() []pb.YoloTask {
	gd.tasksMu.RLock()
	tasks := gd.tasks
	gd.tasksMu.RUnlock()

	protoTasks := make([]pb.YoloTask, 0, len(tasks))
	for _, t := range tasks {
		protoTasks = append(protoTasks, taskStringToProto(t))
	}
	return protoTasks
}

// SetTasks updates the YOLO11 tasks to run
func (gd *GRPCDetector) SetTasks(tasks []string) {
	gd.tasksMu.Lock()
	defer gd.tasksMu.Unlock()
	if len(tasks) == 0 {
		gd.tasks = []string{"detect"}
	} else {
		gd.tasks = tasks
	}
	log.Printf("[GRPCDetector] Tasks updated: %v", gd.tasks)
}

// GetTasks returns the currently configured tasks
func (gd *GRPCDetector) GetTasks() []string {
	gd.tasksMu.RLock()
	defer gd.tasksMu.RUnlock()
	result := make([]string, len(gd.tasks))
	copy(result, gd.tasks)
	return result
}

// DetectSecurityObjects performs multi-task detection on a frame using AnalyzeStream
// This is the high-performance path for real-time detection with YOLO11 tasks
func (gd *GRPCDetector) DetectSecurityObjects(imageData []byte, confThreshold float32) (*SecurityDetectionResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("gRPC detection service unavailable")
	}

	// Ensure stream is started
	if err := gd.startStream(); err != nil {
		return nil, err
	}

	// Create AnalyzeRequest with configured tasks
	req := &pb.AnalyzeRequest{
		CameraId:        "default", // Will be overwritten by caller
		FrameSeq:        0,
		TimestampNs:     time.Now().UnixNano(),
		JpegData:        imageData,
		Tasks:           gd.getTasks(),
		ReturnAnnotated: gd.drawBoxes,
		ConfThreshold:   confThreshold,
	}

	// Send request
	select {
	case gd.analyzeRequestCh <- req:
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("send timeout")
	}

	// Wait for response
	select {
	case resp := <-gd.analyzeResponseCh:
		return gd.convertAnalyzeResponse(resp), nil
	case <-time.After(500 * time.Millisecond):
		return nil, fmt.Errorf("response timeout")
	}
}

// DetectSecurityObjectsAnnotated performs multi-task detection and returns annotated image
func (gd *GRPCDetector) DetectSecurityObjectsAnnotated(imageData []byte, confThreshold float32) (*AnnotatedSecurityResult, error) {
	// For annotated results, use unary call to guarantee we get the image back
	return gd.DetectSecurityObjectsAnnotatedUnary(imageData, confThreshold)
}

// DetectSecurityObjectsAnnotatedUnary performs a single multi-task detection with annotated image
// Uses AnalyzeStream for YOLO11 multi-task support (detect, pose, segment, etc.)
func (gd *GRPCDetector) DetectSecurityObjectsAnnotatedUnary(imageData []byte, confThreshold float32) (*AnnotatedSecurityResult, error) {
	if !gd.IsHealthy() {
		return nil, fmt.Errorf("gRPC detection service unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// For unary call, we create a one-shot AnalyzeStream
	stream, err := gd.client.AnalyzeStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create analyze stream: %w", err)
	}

	req := &pb.AnalyzeRequest{
		CameraId:        "default",
		FrameSeq:        0,
		TimestampNs:     time.Now().UnixNano(),
		JpegData:        imageData,
		Tasks:           gd.getTasks(),
		ReturnAnnotated: true,
		ConfThreshold:   confThreshold,
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

	return gd.convertAnnotatedAnalyzeResponse(resp), nil
}

// convertAnalyzeResponse converts AnalyzeResponse to internal format
// Combines detections from all task results (detect, segment, etc.)
func (gd *GRPCDetector) convertAnalyzeResponse(resp *pb.AnalyzeResponse) *SecurityDetectionResult {
	detections := make([]Detection, 0)

	// Get detections from detect task
	if resp.Detect != nil {
		for _, det := range resp.Detect.Detections {
			if det.Bbox == nil {
				continue
			}
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
	}

	// Get detections from segment task (also provides bounding boxes)
	if resp.Segment != nil {
		for _, seg := range resp.Segment.Segments {
			if seg.Bbox == nil {
				continue
			}
			detections = append(detections, Detection{
				Class:      seg.ClassName,
				ClassID:    int(seg.ClassId),
				Confidence: seg.Confidence,
				BBox: []float32{
					seg.Bbox.X1,
					seg.Bbox.Y1,
					seg.Bbox.X2,
					seg.Bbox.Y2,
				},
				Center: []float32{
					(seg.Bbox.X1 + seg.Bbox.X2) / 2,
					(seg.Bbox.Y1 + seg.Bbox.Y2) / 2,
				},
				Area: (seg.Bbox.X2 - seg.Bbox.X1) * (seg.Bbox.Y2 - seg.Bbox.Y1),
			})
		}
	}

	// Get detections from pose task (person detection with keypoints)
	if resp.Pose != nil {
		for _, pose := range resp.Pose.Poses {
			if pose.Bbox == nil {
				continue
			}
			detections = append(detections, Detection{
				Class:      "person",
				ClassID:    0,
				Confidence: pose.Confidence,
				BBox: []float32{
					pose.Bbox.X1,
					pose.Bbox.Y1,
					pose.Bbox.X2,
					pose.Bbox.Y2,
				},
				Center: []float32{
					(pose.Bbox.X1 + pose.Bbox.X2) / 2,
					(pose.Bbox.Y1 + pose.Bbox.Y2) / 2,
				},
				Area: (pose.Bbox.X2 - pose.Bbox.X1) * (pose.Bbox.Y2 - pose.Bbox.Y1),
			})
		}
	}

	// Categorize by threat level
	threat := gd.categorizeThreat(detections)

	return &SecurityDetectionResult{
		Detections:      detections,
		Count:           len(detections),
		ThreatAnalysis:  threat,
		InferenceTimeMs: resp.TotalInferenceMs,
		Device:          resp.Device,
		SecurityFilter:  []string{"person", "car", "truck", "motorcycle", "bicycle"},
	}
}

// convertAnnotatedAnalyzeResponse converts AnalyzeResponse to annotated result
func (gd *GRPCDetector) convertAnnotatedAnalyzeResponse(resp *pb.AnalyzeResponse) *AnnotatedSecurityResult {
	detections := make([]Detection, 0)

	// Get detections from detect task
	if resp.Detect != nil {
		for _, det := range resp.Detect.Detections {
			if det.Bbox == nil {
				continue
			}
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
	}

	// Get detections from segment task
	if resp.Segment != nil {
		for _, seg := range resp.Segment.Segments {
			if seg.Bbox == nil {
				continue
			}
			detections = append(detections, Detection{
				Class:      seg.ClassName,
				ClassID:    int(seg.ClassId),
				Confidence: seg.Confidence,
				BBox: []float32{
					seg.Bbox.X1,
					seg.Bbox.Y1,
					seg.Bbox.X2,
					seg.Bbox.Y2,
				},
				Center: []float32{
					(seg.Bbox.X1 + seg.Bbox.X2) / 2,
					(seg.Bbox.Y1 + seg.Bbox.Y2) / 2,
				},
				Area: (seg.Bbox.X2 - seg.Bbox.X1) * (seg.Bbox.Y2 - seg.Bbox.Y1),
			})
		}
	}

	// Get detections from pose task
	if resp.Pose != nil {
		for _, pose := range resp.Pose.Poses {
			if pose.Bbox == nil {
				continue
			}
			detections = append(detections, Detection{
				Class:      "person",
				ClassID:    0,
				Confidence: pose.Confidence,
				BBox: []float32{
					pose.Bbox.X1,
					pose.Bbox.Y1,
					pose.Bbox.X2,
					pose.Bbox.Y2,
				},
				Center: []float32{
					(pose.Bbox.X1 + pose.Bbox.X2) / 2,
					(pose.Bbox.Y1 + pose.Bbox.Y2) / 2,
				},
				Area: (pose.Bbox.X2 - pose.Bbox.X1) * (pose.Bbox.Y2 - pose.Bbox.Y1),
			})
		}
	}

	threat := gd.categorizeThreat(detections)

	return &AnnotatedSecurityResult{
		ImageData:       resp.AnnotatedJpeg,
		Detections:      detections,
		Count:           len(detections),
		InferenceTimeMs: resp.TotalInferenceMs,
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
