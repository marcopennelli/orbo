// Camera types - matches CameraInfo from design.go
export interface Camera {
  id: string;
  name: string;
  device: string;  // Camera device path (e.g., /dev/video0)
  status: 'active' | 'inactive' | 'error';
  resolution?: string;
  fps?: number;
  created_at?: string;
}

export interface CameraCreatePayload {
  name: string;
  device: string;
  resolution?: string;
  fps?: number;
}

export interface CameraUpdatePayload {
  name?: string;
  device?: string;  // Can only be changed when camera is inactive
  resolution?: string;
  fps?: number;
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

// YOLO config - matches YOLOConfig from design.go
export interface YoloConfig {
  enabled: boolean;
  service_endpoint?: string;
  confidence_threshold?: number;
  security_mode?: boolean;
  classes_filter?: string;  // Comma-separated class names
  draw_boxes?: boolean;
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
