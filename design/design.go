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
    Required("id", "camera_id", "timestamp", "confidence")
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

var SystemStatus = Type("SystemStatus", func() {
    Description("System status information")
    Field(1, "cameras", ArrayOf(CameraInfo), "Camera status list")
    Field(2, "motion_detection_active", Boolean, "Motion detection status")
    Field(3, "notifications_active", Boolean, "Notification status")
    Field(4, "uptime_seconds", Int, "System uptime in seconds")
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
        Description("Update camera configuration")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Field(2, "name", String, "Camera name")
            Field(3, "resolution", String, "Camera resolution")
            Field(4, "fps", Int, "Frames per second")
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
        Description("Capture a single frame from camera as JPEG")
        Payload(func() {
            Field(1, "id", String, "Camera ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(Bytes)
        Error("not_found", NotFoundError, "Camera not found")
        Error("internal", InternalError, "Failed to capture frame")
        HTTP(func() {
            GET("/api/v1/cameras/{id}/frame")
            Response(StatusOK, func() {
                ContentType("image/jpeg")
            })
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
        Description("Get captured frame for motion event")
        Payload(func() {
            Field(1, "id", String, "Event ID", func() {
                Format(FormatUUID)
            })
            Required("id")
        })
        Result(Bytes)
        Error("not_found", NotFoundError, "Event or frame not found")
        HTTP(func() {
            GET("/api/v1/motion/events/{id}/frame")
            Response(StatusOK, func() {
                Header("Content-Type:image/jpeg")
            })
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