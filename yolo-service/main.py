#!/usr/bin/env python3
"""
AMD GPU-accelerated YOLOv8 Object Detection Service
Optimized for Orbo video alarm system
"""

from fastapi import FastAPI, File, UploadFile, HTTPException, Query
from fastapi.responses import JSONResponse, Response
from ultralytics import YOLO
import torch
import cv2
import numpy as np
import io
from PIL import Image
import time
import logging
from typing import List, Dict, Any, Optional, Tuple
import os
import base64

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="Orbo YOLOv8 Detection Service",
    description="AMD GPU-accelerated object detection for video surveillance",
    version="1.0.0"
)

class YOLODetectionService:
    def __init__(self):
        self.model = None
        self.device = None
        self.model_loaded = False
        self.default_classes_filter = None  # Class names to filter at inference time
        self.initialize_model()
        self._load_default_classes_filter()
    
    def initialize_model(self):
        """Initialize YOLOv8 model with AMD GPU support"""
        try:
            # Check GPU availability
            if torch.cuda.is_available():
                self.device = 'cuda'
                gpu_name = torch.cuda.get_device_name(0)
                logger.info(f"AMD GPU detected: {gpu_name}")
            else:
                self.device = 'cpu'
                logger.warning("No GPU detected, using CPU")
            
            # Load YOLOv8 model
            model_path = os.getenv('YOLO_MODEL', 'yolov8n.pt')
            logger.info(f"Loading YOLOv8 model: {model_path}")
            
            self.model = YOLO(model_path)
            self.model.to(self.device)
            
            # Warm up the model
            logger.info("Warming up model...")
            dummy_image = torch.randn(1, 3, 640, 640).to(self.device)
            with torch.no_grad():
                _ = self.model(dummy_image, verbose=False)
            
            self.model_loaded = True
            logger.info(f"YOLOv8 model loaded successfully on {self.device}")

        except Exception as e:
            logger.error(f"Failed to initialize model: {e}")
            self.model_loaded = False

    def _load_default_classes_filter(self):
        """Load default class filter from environment variable"""
        classes_filter_env = os.getenv('CLASSES_FILTER', '')
        if classes_filter_env:
            self.default_classes_filter = [c.strip().lower() for c in classes_filter_env.split(',') if c.strip()]
            logger.info(f"Default class filter set: {self.default_classes_filter}")
        else:
            self.default_classes_filter = None
            logger.info("No default class filter set - detecting all classes")

    def _get_class_ids(self, class_names: Optional[List[str]]) -> Optional[List[int]]:
        """Convert class names to YOLO class IDs for inference-time filtering"""
        if not class_names or not self.model_loaded:
            return None

        # Build reverse mapping: class_name -> class_id
        name_to_id = {v.lower(): k for k, v in self.model.names.items()}

        class_ids = []
        for name in class_names:
            name_lower = name.lower()
            if name_lower in name_to_id:
                class_ids.append(name_to_id[name_lower])
            else:
                logger.warning(f"Unknown class name '{name}' - skipping")

        return class_ids if class_ids else None

    def preprocess_image(self, image_data: bytes) -> np.ndarray:
        """Preprocess image for YOLOv8 inference"""
        try:
            # Convert bytes to PIL Image
            image = Image.open(io.BytesIO(image_data))
            
            # Convert to RGB if needed
            if image.mode != 'RGB':
                image = image.convert('RGB')
            
            return np.array(image)
        except Exception as e:
            logger.error(f"Image preprocessing failed: {e}")
            raise HTTPException(status_code=400, detail="Invalid image format")
    
    def detect_objects(self, image_data: bytes, conf_threshold: float = 0.5, classes_filter: Optional[List[str]] = None) -> Dict[str, Any]:
        """Run YOLOv8 inference on image with optional class filtering at inference time"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            # Preprocess image
            image = self.preprocess_image(image_data)

            # Use provided filter or fall back to default
            effective_filter = classes_filter if classes_filter else self.default_classes_filter
            class_ids = self._get_class_ids(effective_filter)

            # Run inference with class filtering at model level (faster than post-filtering)
            results = self.model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)
            
            # Parse results
            detections = []
            for r in results:
                boxes = r.boxes
                if boxes is not None and len(boxes) > 0:
                    for i in range(len(boxes)):
                        cls_id = int(boxes.cls[i])
                        confidence = float(boxes.conf[i])
                        bbox = boxes.xyxy[i].tolist()  # [x1, y1, x2, y2]
                        
                        detection = {
                            "class": r.names[cls_id],
                            "class_id": cls_id,
                            "confidence": confidence,
                            "bbox": bbox,
                            "center": [
                                (bbox[0] + bbox[2]) / 2,  # center_x
                                (bbox[1] + bbox[3]) / 2   # center_y
                            ],
                            "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1])
                        }
                        detections.append(detection)
            
            inference_time = time.time() - start_time
            
            result = {
                "detections": detections,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": "YOLOv8n",
                "conf_threshold": conf_threshold
            }

            # Include filter info if applied
            if effective_filter:
                result["classes_filter"] = effective_filter

            return result

        except Exception as e:
            logger.error(f"Detection failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")

    def detect_and_annotate(
        self,
        image_data: bytes,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        box_color: Tuple[int, int, int] = (0, 255, 0),
        box_thickness: int = 2,
        show_labels: bool = True,
        show_confidence: bool = True
    ) -> Tuple[bytes, Dict[str, Any]]:
        """Run YOLOv8 inference and return annotated image with bounding boxes using YOLO built-in plotting"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            # Preprocess image
            image = self.preprocess_image(image_data)

            # Use provided filter or fall back to default
            effective_filter = classes_filter if classes_filter else self.default_classes_filter
            class_ids = self._get_class_ids(effective_filter)

            # Run inference with class filtering at model level (faster than post-filtering)
            results = self.model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)

            # Parse detections for metadata
            detections = []
            for r in results:
                boxes = r.boxes
                if boxes is not None and len(boxes) > 0:
                    for i in range(len(boxes)):
                        cls_id = int(boxes.cls[i])
                        class_name = r.names[cls_id]
                        confidence = float(boxes.conf[i])
                        bbox = boxes.xyxy[i].tolist()  # [x1, y1, x2, y2]

                        detection = {
                            "class": class_name,
                            "class_id": cls_id,
                            "confidence": confidence,
                            "bbox": bbox,
                            "center": [
                                (bbox[0] + bbox[2]) / 2,
                                (bbox[1] + bbox[3]) / 2
                            ],
                            "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1])
                        }
                        detections.append(detection)

            # Use YOLO's built-in plot() for faster annotation
            # This returns a numpy array with bounding boxes already drawn
            annotated_frame = results[0].plot(
                conf=show_confidence,
                labels=show_labels,
                line_width=box_thickness
            )

            inference_time = time.time() - start_time

            # Convert annotated image to JPEG bytes
            # plot() returns BGR, so we can encode directly
            _, jpeg_data = cv2.imencode('.jpg', annotated_frame, [cv2.IMWRITE_JPEG_QUALITY, 90])

            result_info = {
                "detections": detections,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": "YOLOv8n",
                "conf_threshold": conf_threshold
            }

            # Include filter info if applied
            if effective_filter:
                result_info["classes_filter"] = effective_filter

            return jpeg_data.tobytes(), result_info

        except Exception as e:
            logger.error(f"Detection with annotation failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")

# Global service instance
detection_service = YOLODetectionService()

@app.get("/")
async def root():
    """Root endpoint with service info"""
    return {
        "service": "Orbo YOLOv8 Detection Service",
        "version": "1.0.0",
        "device": detection_service.device,
        "model_loaded": detection_service.model_loaded,
        "gpu_available": torch.cuda.is_available(),
        "default_classes_filter": detection_service.default_classes_filter
    }

@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy" if detection_service.model_loaded else "unhealthy",
        "device": detection_service.device,
        "gpu_available": torch.cuda.is_available(),
        "model_loaded": detection_service.model_loaded
    }

@app.post("/detect")
async def detect_objects(
    file: UploadFile = File(...),
    conf_threshold: float = 0.5,
    classes_filter: str = None
):
    """
    Detect objects in uploaded image

    Args:
        file: Image file (JPEG, PNG, etc.)
        conf_threshold: Confidence threshold (0.0-1.0)
        classes_filter: Comma-separated class names to filter (e.g., "person,car")
                       Filtering happens at inference time for better performance.
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        # Read image data
        image_data = await file.read()

        # Parse class filter
        filter_list = None
        if classes_filter:
            filter_list = [c.strip() for c in classes_filter.split(',') if c.strip()]

        # Run detection with inference-time class filtering
        result = detection_service.detect_objects(image_data, conf_threshold, filter_list)

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")

@app.post("/detect/security")
async def detect_security_objects(
    file: UploadFile = File(...),
    conf_threshold: float = 0.6
):
    """
    Detect security-relevant objects (person, car, truck, bicycle, motorcycle)
    Optimized for surveillance applications with inference-time class filtering
    """
    # Security-focused classes - filtered at inference time for speed
    security_classes = ["person", "car", "truck", "bus", "bicycle", "motorcycle"]

    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()

        # Use inference-time filtering for better performance
        result = detection_service.detect_objects(image_data, conf_threshold, security_classes)

        # Categorize by threat level (detections already filtered)
        threat_analysis = {
            "high_priority": [det for det in result["detections"] if det["class"] == "person"],
            "medium_priority": [det for det in result["detections"] if det["class"] in ["car", "truck", "bus"]],
            "low_priority": [det for det in result["detections"] if det["class"] in ["bicycle", "motorcycle"]]
        }

        return {
            "detections": result["detections"],
            "count": result["count"],
            "threat_analysis": threat_analysis,
            "inference_time_ms": result["inference_time_ms"],
            "device": result["device"],
            "security_filter": security_classes
        }

    except Exception as e:
        logger.error(f"Security detection failed: {e}")
        raise HTTPException(status_code=500, detail="Security detection failed")

@app.get("/classes")
async def get_supported_classes():
    """Get list of supported object classes"""
    if not detection_service.model_loaded:
        raise HTTPException(status_code=503, detail="Model not loaded")

    return {
        "classes": list(detection_service.model.names.values()),
        "count": len(detection_service.model.names)
    }


@app.post("/detect/annotated")
async def detect_annotated(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0),
    classes_filter: Optional[str] = Query(None, description="Comma-separated class names"),
    show_labels: bool = Query(True),
    show_confidence: bool = Query(True),
    format: str = Query("image", description="Response format: 'image' for raw JPEG, 'base64' for JSON with base64")
):
    """
    Detect objects and return annotated image with bounding boxes drawn.

    Args:
        file: Image file (JPEG, PNG, etc.)
        conf_threshold: Confidence threshold (0.0-1.0)
        classes_filter: Comma-separated class names to filter (e.g., "person,car")
        show_labels: Show class labels on boxes
        show_confidence: Show confidence percentage on boxes
        format: 'image' returns raw JPEG, 'base64' returns JSON with base64 encoded image
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()

        # Parse class filter
        filter_list = None
        if classes_filter:
            filter_list = [c.strip() for c in classes_filter.split(',')]

        # Run detection with annotation
        annotated_image, result_info = detection_service.detect_and_annotate(
            image_data,
            conf_threshold=conf_threshold,
            classes_filter=filter_list,
            show_labels=show_labels,
            show_confidence=show_confidence
        )

        if format == "base64":
            # Return JSON with base64 encoded image
            base64_image = base64.b64encode(annotated_image).decode('utf-8')
            return {
                "image": {
                    "data": base64_image,
                    "content_type": "image/jpeg"
                },
                **result_info
            }
        else:
            # Return raw JPEG image
            return Response(
                content=annotated_image,
                media_type="image/jpeg",
                headers={
                    "X-Detection-Count": str(result_info["count"]),
                    "X-Inference-Time-Ms": str(result_info["inference_time_ms"]),
                    "X-Device": result_info["device"]
                }
            )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Annotated detection failed: {e}")
        raise HTTPException(status_code=500, detail="Annotated detection failed")


@app.post("/detect/security/annotated")
async def detect_security_annotated(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.6, ge=0.0, le=1.0),
    show_labels: bool = Query(True),
    show_confidence: bool = Query(True),
    format: str = Query("image", description="Response format: 'image' or 'base64'")
):
    """
    Detect security-relevant objects and return annotated image.
    Filters for: person, car, truck, bus, bicycle, motorcycle

    Color coding:
    - Red: person (high priority)
    - Orange: car, truck, bus (medium priority)
    - Yellow: bicycle, motorcycle (low priority)
    """
    security_classes = ["person", "car", "truck", "bus", "bicycle", "motorcycle"]

    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()

        # Run detection with annotation, filtered to security classes
        annotated_image, result_info = detection_service.detect_and_annotate(
            image_data,
            conf_threshold=conf_threshold,
            classes_filter=security_classes,
            show_labels=show_labels,
            show_confidence=show_confidence
        )

        # Add threat analysis
        threat_analysis = {
            "high_priority": [d for d in result_info["detections"] if d["class"] == "person"],
            "medium_priority": [d for d in result_info["detections"] if d["class"] in ["car", "truck", "bus"]],
            "low_priority": [d for d in result_info["detections"] if d["class"] in ["bicycle", "motorcycle"]]
        }
        result_info["threat_analysis"] = threat_analysis
        result_info["security_filter"] = security_classes

        if format == "base64":
            base64_image = base64.b64encode(annotated_image).decode('utf-8')
            return {
                "image": {
                    "data": base64_image,
                    "content_type": "image/jpeg"
                },
                **result_info
            }
        else:
            return Response(
                content=annotated_image,
                media_type="image/jpeg",
                headers={
                    "X-Detection-Count": str(result_info["count"]),
                    "X-Inference-Time-Ms": str(result_info["inference_time_ms"]),
                    "X-Device": result_info["device"],
                    "X-High-Priority-Count": str(len(threat_analysis["high_priority"])),
                    "X-Medium-Priority-Count": str(len(threat_analysis["medium_priority"])),
                    "X-Low-Priority-Count": str(len(threat_analysis["low_priority"]))
                }
            )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Security annotated detection failed: {e}")
        raise HTTPException(status_code=500, detail="Security annotated detection failed")


if __name__ == "__main__":
    import uvicorn
    
    # Start server
    port = int(os.getenv('PORT', 8081))
    host = os.getenv('HOST', '0.0.0.0')
    
    logger.info(f"Starting YOLOv8 service on {host}:{port}")
    uvicorn.run(
        "main:app",
        host=host,
        port=port,
        reload=False,
        workers=1  # Single worker for GPU efficiency
    )