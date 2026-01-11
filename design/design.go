package design

import (
    . "goa.design/goa/v3/dsl"
)

// API definition
var _ = API("orbo", func() {
    Title("Orbo Video Alarm System")
    Description("Open source video alarm system with motion detection and Telegram integration")
    Version("1.0")
    Server("orbo", func() {
        Host("localhost", func() {
            URI("http://localhost:8080")
        })
        Host("production", func() {
            URI("https://orbo.example.com")
        })
    })
})

// Error types
var NotFoundError = Type("NotFoundError", func() {
    Description("Resource not found error")
    Field(1, "message", String, "Error message")
    Field(2, "id", String, "Resource ID")
    Required("message", "id")
})

var BadRequestError = Type("BadRequestError", func() {
    Description("Bad request error")
    Field(1, "message", String, "Error message")
    Field(2, "details", String, "Error details")
    Required("message")
})

var InternalError = Type("InternalError", func() {
    Description("Internal server error")
    Field(1, "message", String, "Error message")
    Required("message")
})

var NotReadyError = Type("NotReadyError", func() {
    Description("Service is not ready to serve traffic")
    Field(1, "message", String, "Error message")
    Field(2, "details", String, "Additional error details")
    Required("message")
})

var UnauthorizedError = Type("UnauthorizedError", func() {
    Description("Authentication error")
    Field(1, "message", String, "Error message")
    Required("message")
})

// Data types
var CameraInfo = Type("CameraInfo", func() {
    Description("Camera information")
    Field(1, "id", String, "Camera unique identifier", func() {
        Format(FormatUUID)
    })
    Field(2, "name", String, "Camera name")
    Field(3, "device", String, "Camera device path (e.g., /dev/video0)")
    Field(4, "status", String, "Camera status", func() {
        Enum("active", "inactive", "error")
    })
    Field(5, "resolution", String, "Camera resolution")
    Field(6, "fps", Int, "Frames per second")
    Field(7, "created_at", String, "Creation timestamp", func() {
        Format(FormatDateTime)
    })
    Required("id", "name", "device", "status")
})

var MotionEvent = Type("MotionEvent", func() {
    Description("Motion detection event")
    Field(1, "id", String, "Event unique identifier", func() {
        Format(FormatUUID)
    })
    Field(2, "camera_id", String, "Camera ID", func() {
        Format(FormatUUID)
    })
    Field(3, "timestamp", String, "Event timestamp", func() {
        Format(FormatDateTime)
    })
    Field(4, "confidence", Float32, "Motion confidence score (0-1)")
    Field(5, "bounding_boxes", ArrayOf(BoundingBox), "Detected motion areas")
    Field(6, "frame_path", String, "Path to captured frame")
    Field(7, "notification_sent", Boolean, "Whether notification was sent")
    // DINOv3 AI-enhanced fields
    Field(8, "object_class", String, "Detected object class")
    Field(9, "object_confidence", Float32, "AI detection confidence")
    Field(10, "threat_level", String, "Threat level assessment", func() {
        Enum("none", "low", "medium", "high")
    })
    Field(11, "inference_time_ms", Float32, "AI inference time in milliseconds")
    Field(12, "detection_device", String, "Device used for detection", func() {
        Enum("cpu", "cuda", "dinov3")
    })
    // Face recognition fields
    Field(13, "faces_detected", Int, "Number of faces detected in frame")
    Field(14, "known_identities", ArrayOf(String), "Names of recognized known faces")
    Field(15, "unknown_faces_count", Int, "Number of unknown/unrecognized faces")
    Field(16, "forensic_thumbnails", ArrayOf(String), "Paths to forensic face analysis images with landmarks")
    Required("id", "camera_id", "timestamp", "confidence")
})

var FrameResponse = Type("FrameResponse", func() {
    Description("Base64 encoded image frame response")
    Field(1, "data", String, "Base64 encoded JPEG image data")
    Field(2, "content_type", String, "Image MIME type", func() {
        Default("image/jpeg")
    })
    Required("data", "content_type")
})

var BoundingBox = Type("BoundingBox", func() {
    Description("Bounding box coordinates")
    Field(1, "x", Int, "X coordinate")
    Field(2, "y", Int, "Y coordinate")  
    Field(3, "width", Int, "Width")
    Field(4, "height", Int, "Height")
    Required("x", "y", "width", "height")
})

var NotificationConfig = Type("NotificationConfig", func() {
    Description("Notification configuration")
    Field(1, "telegram_enabled", Boolean, "Enable Telegram notifications")
    Field(2, "telegram_bot_token", String, "Telegram bot token")
    Field(3, "telegram_chat_id", String, "Telegram chat ID")
    Field(4, "min_confidence", Float32, "Minimum confidence threshold for notifications")
    Field(5, "cooldown_seconds", Int, "Cooldown period between notifications")
    Required("telegram_enabled")
})

var DINOv3Config = Type("DINOv3Config", func() {
    Description("DINOv3 AI detection configuration")
    Field(1, "enabled", Boolean, "Enable DINOv3 AI detection")
    Field(2, "service_endpoint", String, "DINOv3 service endpoint URL")
    Field(3, "motion_threshold", Float32, "Motion detection threshold (0-1)", func() {
        Default(0.85)
    })
    Field(4, "confidence_threshold", Float32, "Minimum confidence for alerts (0-1)", func() {
        Default(0.6)
    })
    Field(5, "fallback_to_basic", Boolean, "Fallback to basic detection when DINOv3 unavailable", func() {
        Default(true)
    })
    Field(6, "enable_scene_analysis", Boolean, "Enable advanced scene analysis", func() {
        Default(true)
    })
    Required("enabled")
})

var YOLOConfig = Type("YOLOConfig", func() {
    Description("YOLO object detection configuration")
    Field(1, "enabled", Boolean, "Enable YOLO object detection")
    Field(2, "service_endpoint", String, "YOLO service endpoint URL")
    Field(3, "confidence_threshold", Float32, "Minimum confidence for detections (0-1)", func() {
        Default(0.5)
    })
    Field(4, "security_mode", Boolean, "Use security-focused detection (person, car, etc.)", func() {
        Default(true)
    })
    Field(5, "classes_filter", String, "Comma-separated class names to filter (empty = all)")
    Field(6, "draw_boxes", Boolean, "Draw bounding boxes on images (for Telegram, API)", func() {
        Default(false)
    })
    Required("enabled")
})

var DetectionConfig = Type("DetectionConfig", func() {
    Description("Combined detection services configuration")
    Field(1, "primary_detector", String, "Primary detector to use", func() {
        Enum("basic", "yolo", "dinov3")
        Default("basic")
    })
    Field(2, "yolo", YOLOConfig, "YOLO configuration")
    Field(3, "dinov3", DINOv3Config, "DINOv3 configuration")
    Field(4, "fallback_enabled", Boolean, "Enable fallback to basic detection", func() {
        Default(true)
    })
    Required("primary_detector")
})

var PipelineConfig = Type("PipelineConfig", func() {
    Description("Detection pipeline configuration for modular video processing")
    Field(1, "mode", String, "Detection mode", func() {
        Description("Controls when detection runs")
        Enum("disabled", "continuous", "motion_triggered", "scheduled", "hybrid")
        Default("motion_triggered")
    })
    Field(2, "execution_mode", String, "Detector execution mode", func() {
        Description("How detectors are run. Only 'sequential' is supported (chains YOLO → Face → Plate).")
        Enum("sequential")
        Default("sequential")
    })
    Field(3, "detectors", ArrayOf(String), "Enabled detectors in execution order", func() {
        Description("List of detector names: yolo, face, plate. Order matters for sequential mode.")
        Example([]string{"yolo", "face"})
    })
    Field(4, "schedule_interval", String, "Detection interval for scheduled/hybrid modes", func() {
        Description("Go duration string, e.g., '5s', '10s', '1m'")
        Default("5s")
    })
    Field(5, "motion_sensitivity", Float32, "Motion detection sensitivity (0.0-1.0)", func() {
        Description("Lower values = more sensitive. Used in motion_triggered and hybrid modes.")
        Minimum(0)
        Maximum(1)
        Default(0.1)
    })
    Field(6, "motion_cooldown_seconds", Int, "Cooldown after motion stops", func() {
        Description("Seconds to wait after motion ends before stopping detection")
        Minimum(0)
        Default(2)
    })
    Required("mode", "execution_mode")
})

var SystemStatus = Type("SystemStatus", func() {
    Description("System status information")
    Field(1, "cameras", ArrayOf(CameraInfo), "Camera status list")
    Field(2, "motion_detection_active", Boolean, "Motion detection status")
    Field(3, "notifications_active", Boolean, "Notification status")
    Field(4, "uptime_seconds", Int, "System uptime in seconds")
    // Extended pipeline status fields
    Field(5, "pipeline_mode", String, "Current pipeline detection mode (disabled, continuous, motion_triggered, scheduled, hybrid)")
    Field(6, "pipeline_execution_mode", String, "Pipeline execution mode (always sequential)")
    Field(7, "pipeline_detectors", ArrayOf(String), "Enabled detectors in the pipeline")
    Field(8, "pipeline_detection_enabled", Boolean, "Whether AI detection is enabled (false when mode=disabled)")
    Field(9, "detecting_cameras", Int, "Number of cameras currently running detection")
    Required("cameras", "motion_detection_active", "notifications_active", "uptime_seconds")
})

// Health check service
var _ = Service("health", func() {
    Description("Health check endpoints for Kubernetes probes")

    Method("healthz", func() {
        Description("Liveness probe endpoint - indicates if the service is alive")
        Result(Empty)
        HTTP(func() {
            GET("/healthz")
            Response(StatusOK)
        })
    })

    Method("readyz", func() {
        Description("Readiness probe endpoint - indicates if the service is ready to serve traffic")
        Result(Empty)
        Error("not_ready", NotReadyError, "Service is not ready")
        HTTP(func() {
            GET("/readyz")
            Response(StatusOK)
            Response("not_ready", StatusServiceUnavailable)
        })
    })
})

// Authentication service
var _ = Service("auth", func() {
    Description("Authentication service for JWT token management")

    Method("login", func() {
        Description("Authenticate with username and password to receive a JWT token")
        Payload(func() {
            Field(1, "username", String, "Username")
            Field(2, "password", String, "Password")
            Required("username", "password")
        })
        Result(func() {
            Field(1, "token", String, "JWT access token")
            Field(2, "expires_at", Int64, "Token expiration timestamp (Unix)")
            Required("token", "expires_at")
        })
        Error("unauthorized", UnauthorizedError, "Invalid credentials")
        HTTP(func() {
            POST("/api/v1/auth/login")
            Response(StatusOK)
            Response("unauthorized", StatusUnauthorized)
        })
    })

    Method("status", func() {
        Description("Check authentication status and get current user info")
        Result(func() {
            Field(1, "enabled", Boolean, "Whether authentication is enabled")
            Field(2, "authenticated", Boolean, "Whether current request is authenticated")
            Field(3, "username", String, "Current username if authenticated")
            Required("enabled", "authenticated")
        })
        HTTP(func() {
            GET("/api/v1/auth/status")
            Response(StatusOK)
        })
    })
})

// Camera management service
var _ = Service("camera", func() {
    Description("Camera management service for video alarm system")
    
    Method("list", func() {
        Description("List all configured cameras")
        Result(ArrayOf(CameraInfo))
        HTTP(func() {
            GET("/api/v1/cameras")
            Response(StatusOK)
        })
    })
    
    Method("get", func() {
        Description("Get camera information by ID")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(CameraInfo)
        Error("not_found", NotFoundError, "Camera not found")
        HTTP(func() {
            GET("/api/v1/cameras/{id}")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
        })
    })
    
    Method("create", func() {
        Description("Add a new camera")
        Payload(func() {
            Field(1, "name", String, "Camera name")
            Field(2, "device", String, "Camera device path")
            Field(3, "resolution", String, "Camera resolution", func() {
                Default("640x480")
            })
            Field(4, "fps", Int, "Frames per second", func() {
                Default(30)
            })
            Required("name", "device")
        })
        Result(CameraInfo)
        Error("bad_request", BadRequestError, "Invalid camera configuration")
        HTTP(func() {
            POST("/api/v1/cameras")
            Response(StatusCreated)
            Response("bad_request", StatusBadRequest)
        })
    })
    
    Method("update", func() {
        Description("Update camera configuration. Device can only be changed when camera is inactive.")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Field(2, "name", String, "Camera name")
            Field(3, "device", String, "Camera device path (only when inactive)")
            Field(4, "resolution", String, "Camera resolution")
            Field(5, "fps", Int, "Frames per second")
            Required("id")
        })
        Result(CameraInfo)
        Error("not_found", NotFoundError, "Camera not found")
        Error("bad_request", BadRequestError, "Invalid camera configuration")
        HTTP(func() {
            PUT("/api/v1/cameras/{id}")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
            Response("bad_request", StatusBadRequest)
        })
    })
    
    Method("delete", func() {
        Description("Remove a camera")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(Empty)
        Error("not_found", NotFoundError, "Camera not found")
        HTTP(func() {
            DELETE("/api/v1/cameras/{id}")
            Response(StatusNoContent)
            Response("not_found", StatusNotFound)
        })
    })
    
    Method("activate", func() {
        Description("Activate camera for motion detection")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(CameraInfo)
        Error("not_found", NotFoundError, "Camera not found")
        Error("internal", InternalError, "Failed to activate camera")
        HTTP(func() {
            POST("/api/v1/cameras/{id}/activate")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
            Response("internal", StatusInternalServerError)
        })
    })
    
    Method("deactivate", func() {
        Description("Deactivate camera")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(CameraInfo)
        Error("not_found", NotFoundError, "Camera not found")
        HTTP(func() {
            POST("/api/v1/cameras/{id}/deactivate")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
        })
    })
    
    Method("capture", func() {
        Description("Capture a single frame from camera as base64")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(FrameResponse)
        Error("not_found", NotFoundError, "Camera not found")
        Error("internal", InternalError, "Failed to capture frame")
        HTTP(func() {
            GET("/api/v1/cameras/{id}/frame")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
            Response("internal", StatusInternalServerError)
        })
    })
})

// Motion detection service
var _ = Service("motion", func() {
    Description("Motion detection and event management service")
    
    Method("events", func() {
        Description("List motion detection events")
        Payload(func() {
            Field(1, "camera_id", String, "Filter by camera ID", func() {
                Format(FormatUUID)
            })
            Field(2, "since", String, "Show events since timestamp", func() {
                Format(FormatDateTime)
            })
            Field(3, "limit", Int, "Maximum number of events to return", func() {
                Default(50)
                Maximum(500)
            })
        })
        Result(ArrayOf(MotionEvent))
        HTTP(func() {
            GET("/api/v1/motion/events")
            Param("camera_id")
            Param("since")
            Param("limit")
            Response(StatusOK)
        })
    })
    
    Method("event", func() {
        Description("Get motion event by ID")
        Payload(func() {
            Field(1, "id", String, "Event ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(MotionEvent)
        Error("not_found", NotFoundError, "Event not found")
        HTTP(func() {
            GET("/api/v1/motion/events/{id}")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
        })
    })
    
    Method("frame", func() {
        Description("Get captured frame for motion event as base64")
        Payload(func() {
            Field(1, "id", String, "Event ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(FrameResponse)
        Error("not_found", NotFoundError, "Event or frame not found")
        HTTP(func() {
            GET("/api/v1/motion/events/{id}/frame")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
        })
    })

    Method("forensic_thumbnail", func() {
        Description("Get forensic face analysis thumbnail (NSA-style with landmarks) for a motion event")
        Payload(func() {
            Field(1, "id", String, "Event ID", func() {
                Format(FormatUUID)
            })
            Field(2, "index", Int, "Face thumbnail index (0-based)")
            Required("id", "index")
        })
        Result(FrameResponse)
        Error("not_found", NotFoundError, "Event or thumbnail not found")
        HTTP(func() {
            GET("/api/v1/motion/events/{id}/forensic/{index}")
            Response(StatusOK)
            Response("not_found", StatusNotFound)
        })
    })
})

// Configuration service
var _ = Service("config", func() {
    Description("System configuration management")
    
    Method("get", func() {
        Description("Get current notification configuration")
        Result(NotificationConfig)
        HTTP(func() {
            GET("/api/v1/config/notifications")
            Response(StatusOK)
        })
    })
    
    Method("update", func() {
        Description("Update notification configuration")
        Payload(NotificationConfig)
        Result(NotificationConfig)
        Error("bad_request", BadRequestError, "Invalid configuration")
        HTTP(func() {
            PUT("/api/v1/config/notifications")
            Response(StatusOK)
            Response("bad_request", StatusBadRequest)
        })
    })
    
    Method("test_notification", func() {
        Description("Send test notification")
        Result(func() {
            Field(1, "success", Boolean, "Test result")
            Field(2, "message", String, "Result message")
            Required("success", "message")
        })
        Error("internal", InternalError, "Failed to send test notification")
        HTTP(func() {
            POST("/api/v1/config/notifications/test")
            Response(StatusOK)
            Response("internal", StatusInternalServerError)
        })
    })

    Method("get_dinov3", func() {
        Description("Get current DINOv3 AI configuration")
        Result(DINOv3Config)
        HTTP(func() {
            GET("/api/v1/config/dinov3")
            Response(StatusOK)
        })
    })
    
    Method("update_dinov3", func() {
        Description("Update DINOv3 AI configuration")
        Payload(DINOv3Config)
        Result(DINOv3Config)
        Error("bad_request", BadRequestError, "Invalid DINOv3 configuration")
        HTTP(func() {
            PUT("/api/v1/config/dinov3")
            Response(StatusOK)
            Response("bad_request", StatusBadRequest)
        })
    })

    Method("test_dinov3", func() {
        Description("Test DINOv3 service connectivity")
        Result(func() {
            Field(1, "healthy", Boolean, "Service health status")
            Field(2, "endpoint", String, "Service endpoint")
            Field(3, "response_time_ms", Float32, "Response time in milliseconds")
            Field(4, "device", String, "Detection device")
            Field(5, "message", String, "Status message")
            Required("healthy", "message")
        })
        Error("internal", InternalError, "Failed to test DINOv3 service")
        HTTP(func() {
            POST("/api/v1/config/dinov3/test")
            Response(StatusOK)
            Response("internal", StatusInternalServerError)
        })
    })

    Method("get_yolo", func() {
        Description("Get current YOLO detection configuration")
        Result(YOLOConfig)
        HTTP(func() {
            GET("/api/v1/config/yolo")
            Response(StatusOK)
        })
    })

    Method("update_yolo", func() {
        Description("Update YOLO detection configuration")
        Payload(YOLOConfig)
        Result(YOLOConfig)
        Error("bad_request", BadRequestError, "Invalid YOLO configuration")
        HTTP(func() {
            PUT("/api/v1/config/yolo")
            Response(StatusOK)
            Response("bad_request", StatusBadRequest)
        })
    })

    Method("test_yolo", func() {
        Description("Test YOLO service connectivity")
        Result(func() {
            Field(1, "healthy", Boolean, "Service health status")
            Field(2, "endpoint", String, "Service endpoint")
            Field(3, "response_time_ms", Float32, "Response time in milliseconds")
            Field(4, "device", String, "Detection device (cpu/cuda)")
            Field(5, "model_loaded", Boolean, "Model loaded status")
            Field(6, "message", String, "Status message")
            Required("healthy", "message")
        })
        Error("internal", InternalError, "Failed to test YOLO service")
        HTTP(func() {
            POST("/api/v1/config/yolo/test")
            Response(StatusOK)
            Response("internal", StatusInternalServerError)
        })
    })

    Method("get_detection", func() {
        Description("Get combined detection configuration")
        Result(DetectionConfig)
        HTTP(func() {
            GET("/api/v1/config/detection")
            Response(StatusOK)
        })
    })

    Method("update_detection", func() {
        Description("Update combined detection configuration")
        Payload(DetectionConfig)
        Result(DetectionConfig)
        Error("bad_request", BadRequestError, "Invalid detection configuration")
        HTTP(func() {
            PUT("/api/v1/config/detection")
            Response(StatusOK)
            Response("bad_request", StatusBadRequest)
        })
    })

    Method("get_pipeline", func() {
        Description("Get detection pipeline configuration")
        Result(PipelineConfig)
        HTTP(func() {
            GET("/api/v1/config/pipeline")
            Response(StatusOK)
        })
    })

    Method("update_pipeline", func() {
        Description("Update detection pipeline configuration")
        Payload(PipelineConfig)
        Result(PipelineConfig)
        Error("bad_request", BadRequestError, "Invalid pipeline configuration")
        HTTP(func() {
            PUT("/api/v1/config/pipeline")
            Response(StatusOK)
            Response("bad_request", StatusBadRequest)
        })
    })
})

// System status service
var _ = Service("system", func() {
    Description("System status and monitoring")
    
    Method("status", func() {
        Description("Get overall system status")
        Result(SystemStatus)
        HTTP(func() {
            GET("/api/v1/system/status")
            Response(StatusOK)
        })
    })
    
    Method("start_detection", func() {
        Description("Start motion detection on all active cameras")
        Result(SystemStatus)
        Error("internal", InternalError, "Failed to start motion detection")
        HTTP(func() {
            POST("/api/v1/system/detection/start")
            Response(StatusOK)
            Response("internal", StatusInternalServerError)
        })
    })
    
    Method("stop_detection", func() {
        Description("Stop motion detection")
        Result(SystemStatus)
        HTTP(func() {
            POST("/api/v1/system/detection/stop")
            Response(StatusOK)
        })
    })
})