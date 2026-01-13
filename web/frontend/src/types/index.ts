// Camera types - matches CameraInfo from design.go
export interface Camera {
  id: string;
  name: string;
  device: string;  // Camera device path (e.g., /dev/video0)
  status: 'active' | 'inactive' | 'error';
  resolution?: string;
  fps?: number;
  created_at?: string;
  alerts_enabled: boolean;  // Whether events/alerts are enabled (detection still runs for bounding boxes)
}

export interface CameraCreatePayload {
  name: string;
  device: string;
  resolution?: string;
  fps?: number;
  alerts_enabled?: boolean;
}

export interface CameraUpdatePayload {
  name?: string;
  device?: string;  // Can only be changed when camera is inactive
  resolution?: string;
  fps?: number;
  alerts_enabled?: boolean;
}

// Frame response - matches FrameResponse from design.go
export interface FrameResponse {
  data: string;  // Base64 encoded JPEG image data
  content_type: string;  // Image MIME type
}

// Bounding box - matches BoundingBox from design.go
export interface BoundingBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

// Motion event types - matches MotionEvent from design.go
export interface MotionEvent {
  id: string;
  camera_id: string;
  timestamp: string;
  confidence: number;
  bounding_boxes?: BoundingBox[];
  frame_path?: string;
  notification_sent?: boolean;
  // AI-enhanced fields
  object_class?: string;
  object_confidence?: number;
  threat_level?: 'none' | 'low' | 'medium' | 'high';
  inference_time_ms?: number;
  detection_device?: 'cpu' | 'cuda' | 'dinov3';
  // Face recognition fields
  faces_detected?: number;
  known_identities?: string[];
  unknown_faces_count?: number;
  forensic_thumbnails?: string[];  // Paths to forensic face analysis images with landmarks
}

// System status types - matches SystemStatus from design.go
export interface SystemStatus {
  cameras: Camera[];
  motion_detection_active: boolean;
  notifications_active: boolean;
  uptime_seconds: number;
}

// Notification config - matches NotificationConfig from design.go
export interface TelegramConfig {
  telegram_enabled: boolean;
  telegram_bot_token?: string;
  telegram_chat_id?: string;
  min_confidence?: number;
  cooldown_seconds?: number;
}

// YOLO task types - matches YoloTask from Go code
export type YoloTask = 'detect' | 'pose' | 'segment' | 'obb' | 'classify';

// YOLO config - matches YOLOConfig from design.go
export interface YoloConfig {
  enabled: boolean;
  service_endpoint?: string;
  confidence_threshold?: number;
  security_mode?: boolean;
  classes_filter?: string;  // Comma-separated class names
  draw_boxes?: boolean;
  box_color?: string;  // Hex color for bounding boxes (e.g., '#0066FF')
  box_thickness?: number;  // Bounding box line thickness (1-5)
  tasks?: YoloTask[];  // YOLO11 tasks: detect, pose, segment, obb, classify
}

// Recognition config - matches RecognitionConfig from design.go
export interface RecognitionConfig {
  enabled: boolean;
  service_endpoint?: string;
  similarity_threshold?: number;  // Face matching threshold (0-1)
  known_face_color?: string;  // Hex color for known faces (e.g., '#00FF00')
  unknown_face_color?: string;  // Hex color for unknown faces (e.g., '#FF0000')
  box_thickness?: number;  // Bounding box line thickness (1-5)
}

// DINOv3 config - matches DINOv3Config from design.go
export interface Dinov3Config {
  enabled: boolean;
  service_endpoint?: string;
  motion_threshold?: number;
  confidence_threshold?: number;
  fallback_to_basic?: boolean;
  enable_scene_analysis?: boolean;
}

// Detection config - matches DetectionConfig from design.go
export interface DetectionConfig {
  primary_detector: 'basic' | 'yolo' | 'dinov3';
  yolo?: YoloConfig;
  dinov3?: Dinov3Config;
  fallback_enabled?: boolean;
}

// Pipeline config - matches PipelineConfig from design.go
export type DetectionMode = 'disabled' | 'visual_only' | 'continuous' | 'motion_triggered' | 'scheduled' | 'hybrid';
export type ExecutionMode = 'sequential' | 'parallel';
export type DetectorType = 'yolo' | 'face' | 'plate';

export interface PipelineConfig {
  mode: DetectionMode;
  execution_mode: ExecutionMode;
  detectors: DetectorType[];
  schedule_interval: string;  // Go duration string, e.g., '5s', '10s', '1m'
  motion_sensitivity: number;  // 0.0-1.0
  motion_cooldown_seconds: number;
}

// Test notification response
export interface TestNotificationResult {
  success: boolean;
  message: string;
}

// Test YOLO/DINOv3 response
export interface TestServiceResult {
  healthy: boolean;
  endpoint?: string;
  response_time_ms?: number;
  device?: string;
  model_loaded?: boolean;
  message: string;
}

// Test Recognition response
export interface TestRecognitionResult {
  healthy: boolean;
  endpoint?: string;
  response_time_ms?: number;
  device?: string;
  model_loaded?: boolean;
  known_faces_count?: number;
  message: string;
}

// Layout types (frontend-only)
export type LayoutMode = 'single' | 'dual' | 'quad' | 'six' | 'nine';

export interface LayoutSlot {
  position: number;
  cameraId: string | null;
}

export interface LayoutConfig {
  mode: LayoutMode;
  slots: LayoutSlot[];
}

// API response types
export interface ApiResponse<T> {
  data: T;
  error?: string;
}

export interface ApiError {
  message: string;
  code?: number;
}
