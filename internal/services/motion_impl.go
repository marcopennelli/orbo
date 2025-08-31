package services

import (
	"context"
	"fmt"
	"time"

	motion_service "orbo/gen/motion"
	"orbo/internal/motion"
	"orbo/internal/camera"
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

// Frame gets the captured frame for a motion event
func (m *MotionImplementation) Frame(ctx context.Context, p *motion_service.FramePayload) ([]byte, error) {
	frameData, err := m.motionDetector.GetEventFrame(p.ID)
	if err != nil {
		return nil, &motion_service.NotFoundError{
			Message: "Event or frame not found",
			ID:      p.ID,
		}
	}

	return frameData, nil
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

	return &motion_service.MotionEvent{
		ID:               event.ID,
		CameraID:         event.CameraID,
		Timestamp:        event.Timestamp.Format(time.RFC3339),
		Confidence:       event.Confidence,
		BoundingBoxes:    boundingBoxes,
		FramePath:        &event.FramePath,
		NotificationSent: &event.NotificationSent,
	}
}