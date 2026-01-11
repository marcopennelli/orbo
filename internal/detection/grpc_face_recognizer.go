package detection

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	pb "orbo/api/proto/recognition/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// GRPCFaceRecognizer provides gRPC-based face recognition using InsightFace
// Uses bidirectional streaming for low-latency real-time recognition
type GRPCFaceRecognizer struct {
	endpoint   string
	conn       *grpc.ClientConn
	client     pb.FaceRecognitionServiceClient
	stream     pb.FaceRecognitionService_RecognizeStreamClient
	streamMu   sync.Mutex
	enabled    bool
	healthy    bool
	healthMu   sync.RWMutex
	lastHealth time.Time
	threshold  float32

	// Stream management
	streamCtx    context.Context
	streamCancel context.CancelFunc
	responsesCh  chan *pb.FaceResponse
	requestsCh   chan *pb.FaceRequest
	wg           sync.WaitGroup
}

// GRPCFaceRecognizerConfig holds configuration for the gRPC face recognizer
type GRPCFaceRecognizerConfig struct {
	Endpoint            string
	SimilarityThreshold float32
}

// FaceRecognitionWithTrack extends FaceRecognition with track association
type FaceRecognitionWithTrack struct {
	FaceRecognition
	AssociatedTrackID int32
}

// FaceRecognitionResultWithTracks extends FaceRecognitionResult with track info
type FaceRecognitionResultWithTracks struct {
	Recognitions    []FaceRecognitionWithTrack
	Count           int
	KnownCount      int
	UnknownCount    int
	InferenceTimeMs float32
	Device          string
}

// AnnotatedFaceResultWithTracks contains recognition results with annotated image and tracks
type AnnotatedFaceResultWithTracks struct {
	ImageData       []byte
	Recognitions    []FaceRecognitionWithTrack
	Count           int
	KnownCount      int
	UnknownCount    int
	InferenceTimeMs float32
	Device          string
}

// NewGRPCFaceRecognizer creates a new gRPC-based face recognizer
func NewGRPCFaceRecognizer(config GRPCFaceRecognizerConfig) (*GRPCFaceRecognizer, error) {
	fr := &GRPCFaceRecognizer{
		endpoint:    config.Endpoint,
		enabled:     true,
		threshold:   config.SimilarityThreshold,
		responsesCh: make(chan *pb.FaceResponse, 10),
		requestsCh:  make(chan *pb.FaceRequest, 10),
	}

	if fr.threshold <= 0 {
		fr.threshold = 0.5 // Default similarity threshold
	}

	if err := fr.connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to face recognition service: %w", err)
	}

	return fr, nil
}

// connect establishes the gRPC connection
func (fr *GRPCFaceRecognizer) connect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	kacp := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             5 * time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.DialContext(ctx, fr.endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	fr.conn = conn
	fr.client = pb.NewFaceRecognitionServiceClient(conn)

	log.Printf("[GRPCFaceRecognizer] Connected to %s", fr.endpoint)
	return nil
}

// startStream initializes the bidirectional streaming RPC
func (fr *GRPCFaceRecognizer) startStream() error {
	fr.streamMu.Lock()
	defer fr.streamMu.Unlock()

	if fr.stream != nil {
		return nil
	}

	fr.streamCtx, fr.streamCancel = context.WithCancel(context.Background())

	stream, err := fr.client.RecognizeStream(fr.streamCtx)
	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	fr.stream = stream

	fr.wg.Add(2)
	go fr.sendLoop()
	go fr.recvLoop()

	log.Printf("[GRPCFaceRecognizer] Stream started")
	return nil
}

// sendLoop sends face requests to the stream
func (fr *GRPCFaceRecognizer) sendLoop() {
	defer fr.wg.Done()

	for {
		select {
		case <-fr.streamCtx.Done():
			return
		case req := <-fr.requestsCh:
			fr.streamMu.Lock()
			stream := fr.stream
			fr.streamMu.Unlock()

			if stream == nil {
				continue
			}

			if err := stream.Send(req); err != nil {
				log.Printf("[GRPCFaceRecognizer] Send error: %v", err)
				fr.resetStream()
				return
			}
		}
	}
}

// recvLoop receives face recognition responses from the stream
func (fr *GRPCFaceRecognizer) recvLoop() {
	defer fr.wg.Done()

	for {
		fr.streamMu.Lock()
		stream := fr.stream
		fr.streamMu.Unlock()

		if stream == nil {
			select {
			case <-fr.streamCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		resp, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[GRPCFaceRecognizer] Stream ended")
			fr.resetStream()
			return
		}
		if err != nil {
			log.Printf("[GRPCFaceRecognizer] Recv error: %v", err)
			fr.resetStream()
			return
		}

		select {
		case fr.responsesCh <- resp:
		default:
			log.Printf("[GRPCFaceRecognizer] Dropping response, channel full")
		}
	}
}

// resetStream cleans up and prepares for reconnection
func (fr *GRPCFaceRecognizer) resetStream() {
	fr.streamMu.Lock()
	defer fr.streamMu.Unlock()

	if fr.streamCancel != nil {
		fr.streamCancel()
	}
	fr.stream = nil
}

// IsHealthy checks if the face recognition service is available
func (fr *GRPCFaceRecognizer) IsHealthy() bool {
	fr.healthMu.RLock()
	if time.Since(fr.lastHealth) < 30*time.Second && fr.healthy {
		fr.healthMu.RUnlock()
		return true
	}
	fr.healthMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := fr.client.HealthCheck(ctx, &pb.HealthRequest{})
	if err != nil {
		log.Printf("[GRPCFaceRecognizer] Health check failed: %v", err)
		fr.healthMu.Lock()
		fr.healthy = false
		fr.healthMu.Unlock()
		return false
	}

	fr.healthMu.Lock()
	fr.healthy = resp.Status == "healthy" && resp.ModelLoaded
	fr.lastHealth = time.Now()
	fr.healthMu.Unlock()

	return fr.healthy
}

// RecognizeFacesWithTracking performs face recognition with person region hints
// personRegions are bounding boxes from YOLO person detections
// trackIDs are the corresponding YOLO track IDs for face-track association
func (fr *GRPCFaceRecognizer) RecognizeFacesWithTracking(imageData []byte, personRegions [][]float32, trackIDs []int32) (*FaceRecognitionResultWithTracks, error) {
	if !fr.IsHealthy() {
		return nil, fmt.Errorf("face recognition service unavailable")
	}

	if err := fr.startStream(); err != nil {
		return nil, err
	}

	// Convert bounding boxes to proto
	protoRegions := make([]*pb.BBox, len(personRegions))
	for i, bbox := range personRegions {
		if len(bbox) >= 4 {
			protoRegions[i] = &pb.BBox{
				X1: bbox[0],
				Y1: bbox[1],
				X2: bbox[2],
				Y2: bbox[3],
			}
		}
	}

	req := &pb.FaceRequest{
		CameraId:            "default",
		FrameSeq:            0,
		TimestampNs:         time.Now().UnixNano(),
		JpegData:            imageData,
		ReturnAnnotated:     false,
		SimilarityThreshold: fr.threshold,
		PersonRegions:       protoRegions,
		PersonTrackIds:      trackIDs,
	}

	select {
	case fr.requestsCh <- req:
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("send timeout")
	}

	select {
	case resp := <-fr.responsesCh:
		return fr.convertResponseWithTracks(resp), nil
	case <-time.After(500 * time.Millisecond):
		return nil, fmt.Errorf("response timeout")
	}
}

// RecognizeFacesGRPC performs face recognition via gRPC (compatible with HTTP client interface)
func (fr *GRPCFaceRecognizer) RecognizeFacesGRPC(imageData []byte) (*FaceRecognitionResult, error) {
	result, err := fr.RecognizeFacesWithTracking(imageData, nil, nil)
	if err != nil {
		return nil, err
	}

	// Convert to standard FaceRecognitionResult
	recognitions := make([]FaceRecognition, len(result.Recognitions))
	for i, rec := range result.Recognitions {
		recognitions[i] = rec.FaceRecognition
	}

	return &FaceRecognitionResult{
		Recognitions:    recognitions,
		Count:           result.Count,
		KnownCount:      result.KnownCount,
		UnknownCount:    result.UnknownCount,
		InferenceTimeMs: result.InferenceTimeMs,
		Device:          result.Device,
	}, nil
}

// RecognizeFacesAnnotatedGRPC performs face recognition and returns annotated image via gRPC
func (fr *GRPCFaceRecognizer) RecognizeFacesAnnotatedGRPC(imageData []byte) (*AnnotatedRecognitionResult, error) {
	result, err := fr.RecognizeFacesAnnotatedUnary(imageData)
	if err != nil {
		return nil, err
	}

	// Convert to standard AnnotatedRecognitionResult
	recognitions := make([]FaceRecognition, len(result.Recognitions))
	for i, rec := range result.Recognitions {
		recognitions[i] = rec.FaceRecognition
	}

	return &AnnotatedRecognitionResult{
		ImageData:       result.ImageData,
		Recognitions:    recognitions,
		Count:           result.Count,
		KnownCount:      result.KnownCount,
		UnknownCount:    result.UnknownCount,
		InferenceTimeMs: result.InferenceTimeMs,
		Device:          result.Device,
	}, nil
}

// RecognizeFacesAnnotatedUnary performs a single recognition with annotated image
func (fr *GRPCFaceRecognizer) RecognizeFacesAnnotatedUnary(imageData []byte) (*AnnotatedFaceResultWithTracks, error) {
	if !fr.IsHealthy() {
		return nil, fmt.Errorf("face recognition service unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := fr.client.RecognizeStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	req := &pb.FaceRequest{
		CameraId:            "default",
		FrameSeq:            0,
		TimestampNs:         time.Now().UnixNano(),
		JpegData:            imageData,
		ReturnAnnotated:     true,
		SimilarityThreshold: fr.threshold,
	}

	if err := stream.Send(req); err != nil {
		return nil, fmt.Errorf("send failed: %w", err)
	}

	if err := stream.CloseSend(); err != nil {
		log.Printf("[GRPCFaceRecognizer] CloseSend error: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("recv failed: %w", err)
	}

	return fr.convertAnnotatedResponseWithTracks(resp), nil
}

// convertResponseWithTracks converts gRPC response to internal format with tracks
func (fr *GRPCFaceRecognizer) convertResponseWithTracks(resp *pb.FaceResponse) *FaceRecognitionResultWithTracks {
	recognitions := make([]FaceRecognitionWithTrack, 0, len(resp.Faces))

	for _, f := range resp.Faces {
		var identity *string
		if f.Identity != "" {
			identity = &f.Identity
		}

		rec := FaceRecognitionWithTrack{
			FaceRecognition: FaceRecognition{
				BBox: []float32{
					f.Bbox.X1,
					f.Bbox.Y1,
					f.Bbox.X2,
					f.Bbox.Y2,
				},
				Confidence: f.Confidence,
				Identity:   identity,
				Similarity: f.Similarity,
				IsKnown:    f.IsKnown,
				Age:        int(f.Age),
				Gender:     f.Gender,
			},
			AssociatedTrackID: f.AssociatedTrackId,
		}
		recognitions = append(recognitions, rec)
	}

	return &FaceRecognitionResultWithTracks{
		Recognitions:    recognitions,
		Count:           int(resp.TotalCount),
		KnownCount:      int(resp.KnownCount),
		UnknownCount:    int(resp.UnknownCount),
		InferenceTimeMs: resp.InferenceMs,
		Device:          resp.Device,
	}
}

// convertAnnotatedResponseWithTracks converts gRPC response to annotated result with tracks
func (fr *GRPCFaceRecognizer) convertAnnotatedResponseWithTracks(resp *pb.FaceResponse) *AnnotatedFaceResultWithTracks {
	recognitions := make([]FaceRecognitionWithTrack, 0, len(resp.Faces))

	for _, f := range resp.Faces {
		var identity *string
		if f.Identity != "" {
			identity = &f.Identity
		}

		rec := FaceRecognitionWithTrack{
			FaceRecognition: FaceRecognition{
				BBox: []float32{
					f.Bbox.X1,
					f.Bbox.Y1,
					f.Bbox.X2,
					f.Bbox.Y2,
				},
				Confidence: f.Confidence,
				Identity:   identity,
				Similarity: f.Similarity,
				IsKnown:    f.IsKnown,
				Age:        int(f.Age),
				Gender:     f.Gender,
			},
			AssociatedTrackID: f.AssociatedTrackId,
		}
		recognitions = append(recognitions, rec)
	}

	return &AnnotatedFaceResultWithTracks{
		ImageData:       resp.AnnotatedJpeg,
		Recognitions:    recognitions,
		Count:           int(resp.TotalCount),
		KnownCount:      int(resp.KnownCount),
		UnknownCount:    int(resp.UnknownCount),
		InferenceTimeMs: resp.InferenceMs,
		Device:          resp.Device,
	}
}

// RegisterFace adds a new face identity to the database
func (fr *GRPCFaceRecognizer) RegisterFace(name string, imageData []byte, metadata map[string]string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := &pb.RegisterFaceRequest{
		Name:      name,
		ImageData: imageData,
		Metadata:  metadata,
	}

	resp, err := fr.client.RegisterFace(ctx, req)
	if err != nil {
		return fmt.Errorf("register face failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("register face failed: %s", resp.Message)
	}

	log.Printf("[GRPCFaceRecognizer] Registered face '%s', total faces: %d", name, resp.FaceCount)
	return nil
}

// DeleteFace removes a face identity from the database
func (fr *GRPCFaceRecognizer) DeleteFace(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.DeleteFaceRequest{
		Name: name,
	}

	resp, err := fr.client.DeleteFace(ctx, req)
	if err != nil {
		return fmt.Errorf("delete face failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("delete face failed: %s", resp.Message)
	}

	log.Printf("[GRPCFaceRecognizer] Deleted face '%s', remaining faces: %d", name, resp.FaceCount)
	return nil
}

// ListFaces returns all registered face identities
func (fr *GRPCFaceRecognizer) ListFaces() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := fr.client.ListFaces(ctx, &pb.ListFacesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list faces failed: %w", err)
	}

	names := make([]string, 0, len(resp.Faces))
	for _, face := range resp.Faces {
		names = append(names, face.Name)
	}

	return names, nil
}

// SetThreshold updates the similarity threshold for face matching
func (fr *GRPCFaceRecognizer) SetThreshold(threshold float32) {
	fr.threshold = threshold
}

// GetThreshold returns the current similarity threshold
func (fr *GRPCFaceRecognizer) GetThreshold() float32 {
	return fr.threshold
}

// IsEnabled returns whether the recognizer is enabled
func (fr *GRPCFaceRecognizer) IsEnabled() bool {
	return fr.enabled
}

// SetEnabled enables or disables the recognizer
func (fr *GRPCFaceRecognizer) SetEnabled(enabled bool) {
	fr.enabled = enabled
}

// Close shuts down the gRPC connection
func (fr *GRPCFaceRecognizer) Close() error {
	fr.resetStream()
	fr.wg.Wait()

	if fr.conn != nil {
		return fr.conn.Close()
	}
	return nil
}
