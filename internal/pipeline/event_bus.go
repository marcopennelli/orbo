package pipeline

import (
	"sync"
)

// EventBus provides pub/sub for detection results
// Subscribers receive detection results from the pipeline
type EventBus struct {
	subscribers map[*eventSubscription]bool
	mu          sync.RWMutex
}

type eventSubscription struct {
	cameraFilter string // Empty string means receive all cameras
	channel      chan *MergedDetectionResult
	handler      DetectionResultHandler
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[*eventSubscription]bool),
	}
}

// Subscribe registers a handler for detection results from all cameras
// Returns an unsubscribe function
func (b *EventBus) Subscribe(handler DetectionResultHandler) func() {
	sub := &eventSubscription{
		handler: handler,
	}

	b.mu.Lock()
	b.subscribers[sub] = true
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
	}
}

// SubscribeCamera registers a handler for detection results from a specific camera
// Returns an unsubscribe function
func (b *EventBus) SubscribeCamera(cameraID string, handler DetectionResultHandler) func() {
	sub := &eventSubscription{
		cameraFilter: cameraID,
		handler:      handler,
	}

	b.mu.Lock()
	b.subscribers[sub] = true
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		delete(b.subscribers, sub)
		b.mu.Unlock()
	}
}

// SubscribeChannel returns a channel that receives detection results
// The channel has the specified buffer size
// Returns the channel and an unsubscribe function
func (b *EventBus) SubscribeChannel(bufferSize int) (<-chan *MergedDetectionResult, func()) {
	if bufferSize <= 0 {
		bufferSize = 10
	}

	ch := make(chan *MergedDetectionResult, bufferSize)
	sub := &eventSubscription{
		channel: ch,
	}

	b.mu.Lock()
	b.subscribers[sub] = true
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[sub]; ok {
			delete(b.subscribers, sub)
			close(ch)
		}
		b.mu.Unlock()
	}

	return ch, unsubscribe
}

// SubscribeCameraChannel returns a channel that receives detection results for a specific camera
func (b *EventBus) SubscribeCameraChannel(cameraID string, bufferSize int) (<-chan *MergedDetectionResult, func()) {
	if bufferSize <= 0 {
		bufferSize = 10
	}

	ch := make(chan *MergedDetectionResult, bufferSize)
	sub := &eventSubscription{
		cameraFilter: cameraID,
		channel:      ch,
	}

	b.mu.Lock()
	b.subscribers[sub] = true
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if _, ok := b.subscribers[sub]; ok {
			delete(b.subscribers, sub)
			close(ch)
		}
		b.mu.Unlock()
	}

	return ch, unsubscribe
}

// Publish sends a detection result to all subscribers
func (b *EventBus) Publish(result *MergedDetectionResult) {
	if result == nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		// Check camera filter
		if sub.cameraFilter != "" && sub.cameraFilter != result.CameraID {
			continue
		}

		// Deliver via handler or channel
		// IMPORTANT: Handlers are called synchronously to preserve frame ordering.
		// Detection results must be delivered in sequence to prevent old frames
		// from appearing after newer ones in the stream.
		if sub.handler != nil {
			sub.handler.OnDetectionResult(result)
		} else if sub.channel != nil {
			select {
			case sub.channel <- result:
			default:
				// Channel full, skip this result
			}
		}
	}
}

// SubscriberCount returns the number of active subscribers
func (b *EventBus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// Close unsubscribes all subscribers and closes channels
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for sub := range b.subscribers {
		if sub.channel != nil {
			close(sub.channel)
		}
		delete(b.subscribers, sub)
	}
}

// StreamOverlayAdapter adapts EventBus to StreamOverlayProvider interface
// This allows the detection pipeline to feed annotated frames to streaming
type StreamOverlayAdapter struct {
	provider StreamOverlayProvider
}

// NewStreamOverlayAdapter creates an adapter that forwards detection results to a stream provider
func NewStreamOverlayAdapter(provider StreamOverlayProvider) *StreamOverlayAdapter {
	return &StreamOverlayAdapter{
		provider: provider,
	}
}

// OnDetectionResult implements DetectionResultHandler
func (a *StreamOverlayAdapter) OnDetectionResult(result *MergedDetectionResult) {
	if a.provider == nil || result == nil {
		return
	}

	// Forward annotated image to stream with sequence number for ordering
	if len(result.ImageData) > 0 {
		a.provider.SetAnnotatedFrame(result.CameraID, result.FrameSeq, result.ImageData)
	}

	// Convert and forward detection metadata
	detections := make([]Detection, 0, len(result.Detections))
	for _, d := range result.Detections {
		detections = append(detections, d)
	}

	faces := make([]FaceDetection, 0, len(result.Faces))
	for _, f := range result.Faces {
		faces = append(faces, f)
	}

	a.provider.UpdateDetections(result.CameraID, detections, faces)
}

// Ensure StreamOverlayAdapter implements DetectionResultHandler
var _ DetectionResultHandler = (*StreamOverlayAdapter)(nil)
