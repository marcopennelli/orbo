#!/usr/bin/env python3
"""
AMD GPU-accelerated YOLOv8 Object Detection Service
Optimized for Orbo video alarm system
"""

from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse
from ultralytics import YOLO
import torch
import cv2
import numpy as np
import io
from PIL import Image
import time
import logging
from typing import List, Dict, Any
import os

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
        self.initialize_model()
    
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
    
    def detect_objects(self, image_data: bytes, conf_threshold: float = 0.5) -> Dict[str, Any]:
        """Run YOLOv8 inference on image"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")
        
        start_time = time.time()
        
        try:
            # Preprocess image
            image = self.preprocess_image(image_data)
            
            # Run inference
            results = self.model(image, device=self.device, conf=conf_threshold, verbose=False)
            
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
            
            return {
                "detections": detections,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": "YOLOv8n",
                "conf_threshold": conf_threshold
            }
            
        except Exception as e:
            logger.error(f"Detection failed: {e}")
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
        "gpu_available": torch.cuda.is_available()
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
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")
    
    try:
        # Read image data
        image_data = await file.read()
        
        # Run detection
        result = detection_service.detect_objects(image_data, conf_threshold)
        
        # Apply class filtering if specified
        if classes_filter:
            filter_classes = set(cls.strip().lower() for cls in classes_filter.split(','))
            filtered_detections = [
                det for det in result["detections"] 
                if det["class"].lower() in filter_classes
            ]
            result["detections"] = filtered_detections
            result["count"] = len(filtered_detections)
            result["filter_applied"] = classes_filter
        
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
    Optimized for surveillance applications
    """
    # Security-focused classes
    security_classes = ["person", "car", "truck", "bus", "bicycle", "motorcycle"]
    
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")
    
    try:
        image_data = await file.read()
        result = detection_service.detect_objects(image_data, conf_threshold)
        
        # Filter for security-relevant objects
        security_detections = [
            det for det in result["detections"] 
            if det["class"] in security_classes
        ]
        
        # Categorize by threat level
        threat_analysis = {
            "high_priority": [det for det in security_detections if det["class"] == "person"],
            "medium_priority": [det for det in security_detections if det["class"] in ["car", "truck", "bus"]],
            "low_priority": [det for det in security_detections if det["class"] in ["bicycle", "motorcycle"]]
        }
        
        return {
            "detections": security_detections,
            "count": len(security_detections),
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