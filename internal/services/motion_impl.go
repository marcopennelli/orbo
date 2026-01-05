package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	motion_service "orbo/gen/motion"
	"orbo/internal/camera"
	"orbo/internal/motion"
)

// MotionImplementation implements the motion service
type MotionImplementation struct {
	motionDetector *motion.MotionDetector
	cameraManager  *camera.CameraManager
}

// NewMotionService creates a new motion service implementation
func NewMotionService(motionDetector *motion.MotionDetector, cameraManager *camera.CameraManager) *MotionImplementation {
	return &MotionImplementation{
		motionDetector: motionDetector,
		cameraManager:  cameraManager,
	}
}

// Events lists motion detection events
func (m *MotionImplementation) Events(ctx context.Context, p *motion_service.EventsPayload) ([]*motion_service.MotionEvent, error) {
	var since *time.Time
	if p.Since != nil {
		t, err := time.Parse(time.RFC3339, *p.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid timestamp format: %w", err)
		}
		since = &t
	}

	limit := p.Limit
	if limit == 0 {
		limit = 50 // default
	}

	cameraID := ""
	if p.CameraID != nil {
		cameraID = *p.CameraID
	}

	events := m.motionDetector.GetEvents(cameraID, since, limit)
	
	// Convert internal events to service events
	result := make([]*motion_service.MotionEvent, len(events))
	for i, event := range events {
		result[i] = m.convertMotionEvent(event)
	}

	return result, nil
}

// Event gets a specific motion event by ID
func (m *MotionImplementation) Event(ctx context.Context, p *motion_service.EventPayload) (*motion_service.MotionEvent, error) {
	event, err := m.motionDetector.GetEvent(p.ID)
	if err != nil {
		return nil, &motion_service.NotFoundError{
			Message: "Event not found",
			ID:      p.ID,
		}
	}

	return m.convertMotionEvent(event), nil
}

// Frame gets the captured frame for a motion event as base64
func (m *MotionImplementation) Frame(ctx context.Context, p *motion_service.FramePayload) (*motion_service.FrameResponse, error) {
	fmt.Printf("Frame request for event ID: %s\n", p.ID)

	frameData, err := m.motionDetector.GetEventFrame(p.ID)
	if err != nil {
		fmt.Printf("Frame request failed for event %s: %v\n", p.ID, err)
		return nil, &motion_service.NotFoundError{
			Message: "Event or frame not found",
			ID:      p.ID,
		}
	}

	// Encode frame as base64
	base64Data := base64.StdEncoding.EncodeToString(frameData)
	fmt.Printf("Frame request succeeded for event %s: %d bytes -> %d base64 chars\n", p.ID, len(frameData), len(base64Data))

	return &motion_service.FrameResponse{
		Data:        base64Data,
		ContentType: "image/jpeg",
	}, nil
}

// ForensicThumbnail gets a forensic face analysis thumbnail for a motion event
func (m *MotionImplementation) ForensicThumbnail(ctx context.Context, p *motion_service.ForensicThumbnailPayload) (*motion_service.FrameResponse, error) {
	fmt.Printf("Forensic thumbnail request for event ID: %s, index: %d\n", p.ID, p.Index)

	// Get the event to access forensic thumbnails
	event, err := m.motionDetector.GetEvent(p.ID)
	if err != nil {
		fmt.Printf("Forensic thumbnail request failed - event not found: %s\n", p.ID)
		return nil, &motion_service.NotFoundError{
			Message: "Event not found",
			ID:      p.ID,
		}
	}

	// Check if index is valid
	if p.Index < 0 || p.Index >= len(event.ForensicThumbnails) {
		fmt.Printf("Forensic thumbnail request failed - invalid index %d (have %d thumbnails)\n", p.Index, len(event.ForensicThumbnails))
		return nil, &motion_service.NotFoundError{
			Message: fmt.Sprintf("Thumbnail index %d not found (event has %d face thumbnails)", p.Index, len(event.ForensicThumbnails)),
			ID:      p.ID,
		}
	}

	// Read the thumbnail file
	thumbnailPath := event.ForensicThumbnails[p.Index]
	thumbnailData, err := os.ReadFile(thumbnailPath)
	if err != nil {
		fmt.Printf("Forensic thumbnail request failed - file read error: %v\n", err)
		return nil, &motion_service.NotFoundError{
			Message: "Thumbnail file not found",
			ID:      p.ID,
		}
	}

	// Encode as base64
	base64Data := base64.StdEncoding.EncodeToString(thumbnailData)
	fmt.Printf("Forensic thumbnail request succeeded for event %s index %d: %d bytes\n", p.ID, p.Index, len(thumbnailData))

	return &motion_service.FrameResponse{
		Data:        base64Data,
		ContentType: "image/jpeg",
	}, nil
}

// convertMotionEvent converts internal motion event to service event
func (m *MotionImplementation) convertMotionEvent(event *motion.MotionEvent) *motion_service.MotionEvent {
	// Convert bounding boxes
	boundingBoxes := make([]*motion_service.BoundingBox, len(event.BoundingBoxes))
	for i, bbox := range event.BoundingBoxes {
		boundingBoxes[i] = &motion_service.BoundingBox{
			X:      bbox.X,
			Y:      bbox.Y,
			Width:  bbox.Width,
			Height: bbox.Height,
		}
	}

	result := &motion_service.MotionEvent{
		ID:               event.ID,
		CameraID:         event.CameraID,
		Timestamp:        event.Timestamp.Format(time.RFC3339),
		Confidence:       event.Confidence,
		BoundingBoxes:    boundingBoxes,
		FramePath:        &event.FramePath,
		NotificationSent: &event.NotificationSent,
	}

	// Add AI detection fields if present
	if event.ObjectClass != "" {
		result.ObjectClass = &event.ObjectClass
	}
	if event.ObjectConfidence > 0 {
		result.ObjectConfidence = &event.ObjectConfidence
	}
	if event.ThreatLevel != "" {
		result.ThreatLevel = &event.ThreatLevel
	}
	if event.InferenceTimeMs > 0 {
		result.InferenceTimeMs = &event.InferenceTimeMs
	}
	if event.DetectionDevice != "" {
		result.DetectionDevice = &event.DetectionDevice
	}

	// Add face recognition fields if present
	if event.FacesDetected > 0 {
		result.FacesDetected = &event.FacesDetected
		result.KnownIdentities = event.KnownIdentities
		result.UnknownFacesCount = &event.UnknownFacesCount
	}

	// Add forensic thumbnails if present
	if len(event.ForensicThumbnails) > 0 {
		result.ForensicThumbnails = event.ForensicThumbnails
	}

	return result
}