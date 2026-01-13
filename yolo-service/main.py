#!/usr/bin/env python3
"""
GPU-accelerated YOLO11 Object Detection Service
Optimized for Orbo video alarm system with real-time per-frame detection
"""

import os
import logging

# Configure Ultralytics BEFORE importing - prevents permission errors
os.environ['YOLO_CONFIG_DIR'] = '/tmp/Ultralytics'
os.makedirs('/tmp/Ultralytics', exist_ok=True)
os.makedirs('/tmp/yolo_runs', exist_ok=True)

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Now import ultralytics and configure settings
from ultralytics import YOLO, settings
settings.update({'runs_dir': '/tmp/yolo_runs', 'datasets_dir': '/tmp/datasets'})

from fastapi import FastAPI, File, UploadFile, HTTPException, Query
from fastapi.responses import JSONResponse, Response
import torch
import cv2
import numpy as np
import io
from PIL import Image
import time
from typing import List, Dict, Any, Optional, Tuple
import base64

app = FastAPI(
    title="Orbo YOLO11 Detection Service",
    description="GPU-accelerated object detection for real-time video surveillance",
    version="2.0.0"
)

class YOLODetectionService:
    def __init__(self):
        self.model = None
        self.device = None
        self.model_loaded = False
        self.default_classes_filter = None  # Class names to filter at inference time

        # Tracking configuration
        self.tracker_type = os.getenv('TRACKER_TYPE', '')  # 'bytetrack', 'botsort', or '' to disable
        self.tracking_enabled = self.tracker_type != ''

        # Per-camera tracking state (for persist=True in model.track)
        # Key: camera_id, Value: last tracking results
        self.track_history: Dict[str, Dict] = {}

        # Bounding box appearance configuration
        # Default blue color in BGR format (OpenCV uses BGR)
        self.box_color: Tuple[int, int, int] = (255, 102, 0)  # BGR for #0066FF
        self.box_thickness: int = 2

        self.initialize_model()
        self._load_default_classes_filter()

        if self.tracking_enabled:
            logger.info(f"Object tracking enabled with {self.tracker_type}")
    
    def initialize_model(self):
        """Initialize YOLO11 model with GPU support"""
        try:
            # Check GPU availability
            if torch.cuda.is_available():
                self.device = 'cuda'
                gpu_name = torch.cuda.get_device_name(0)
                logger.info(f"GPU detected: {gpu_name}")
            else:
                self.device = 'cpu'
                logger.warning("No GPU detected, using CPU")

            # Load YOLO11 model (defaults to yolo11n.pt for best speed)
            # Available models: yolo11n.pt, yolo11s.pt, yolo11m.pt, yolo11l.pt, yolo11x.pt
            model_path = os.getenv('YOLO_MODEL', 'yolo11n.pt')
            logger.info(f"Loading YOLO11 model: {model_path}")

            self.model = YOLO(model_path)
            self.model.to(self.device)

            # Store model name for reporting
            self.model_name = model_path.replace('.pt', '').upper()

            # Warm up the model (save=False to avoid creating runs directory)
            logger.info("Warming up model...")
            dummy_image = np.zeros((640, 640, 3), dtype=np.uint8)
            _ = self.model(dummy_image, verbose=False, save=False)

            self.model_loaded = True
            logger.info(f"YOLO11 model loaded successfully on {self.device}")

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

    def _hex_to_bgr(self, hex_color: str) -> Tuple[int, int, int]:
        """Convert hex color string to BGR tuple for OpenCV"""
        # Remove # if present
        hex_color = hex_color.lstrip('#')
        # Parse RGB values
        r = int(hex_color[0:2], 16)
        g = int(hex_color[2:4], 16)
        b = int(hex_color[4:6], 16)
        # Return as BGR (OpenCV format)
        return (b, g, r)

    def set_box_color(self, hex_color: str):
        """Set bounding box color from hex string (e.g., '#0066FF')"""
        try:
            self.box_color = self._hex_to_bgr(hex_color)
            logger.info(f"Box color set to {hex_color} -> BGR {self.box_color}")
        except Exception as e:
            logger.warning(f"Invalid color {hex_color}: {e}, keeping current color")

    def set_box_thickness(self, thickness: int):
        """Set bounding box line thickness (1-5)"""
        self.box_thickness = max(1, min(5, thickness))
        logger.info(f"Box thickness set to {self.box_thickness}")

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

            # Convert RGB to BGR for OpenCV/YOLO compatibility
            rgb_array = np.array(image)
            bgr_array = cv2.cvtColor(rgb_array, cv2.COLOR_RGB2BGR)
            return bgr_array
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
                "model_size": getattr(self, 'model_name', 'YOLO11N'),
                "conf_threshold": conf_threshold
            }

            # Include filter info if applied
            if effective_filter:
                result["classes_filter"] = effective_filter

            return result

        except Exception as e:
            logger.error(f"Detection failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")

    def detect_with_tracking(
        self,
        image_data: bytes,
        camera_id: str = "default",
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Run YOLOv8 inference with object tracking (ByteTrack/BoT-SORT)"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        if not self.tracking_enabled:
            # Fall back to regular detection
            return self.detect_objects(image_data, conf_threshold, classes_filter)

        start_time = time.time()

        try:
            # Preprocess image
            image = self.preprocess_image(image_data)

            # Use provided filter or fall back to default
            effective_filter = classes_filter if classes_filter else self.default_classes_filter
            class_ids = self._get_class_ids(effective_filter)

            # Run tracking with persist=True to maintain track IDs across frames
            # This uses ByteTrack or BoT-SORT depending on tracker_type
            results = self.model.track(
                image,
                device=self.device,
                conf=conf_threshold,
                classes=class_ids,
                persist=True,
                tracker=f"{self.tracker_type}.yaml",
                verbose=False
            )

            # Parse results with track IDs
            detections = []
            tracks = []

            for r in results:
                boxes = r.boxes
                if boxes is not None and len(boxes) > 0:
                    for i in range(len(boxes)):
                        cls_id = int(boxes.cls[i])
                        confidence = float(boxes.conf[i])
                        bbox = boxes.xyxy[i].tolist()

                        # Get track ID (may be None if tracking failed for this detection)
                        track_id = 0
                        if boxes.id is not None and i < len(boxes.id):
                            track_id = int(boxes.id[i])

                        detection = {
                            "class": r.names[cls_id],
                            "class_id": cls_id,
                            "confidence": confidence,
                            "bbox": bbox,
                            "center": [
                                (bbox[0] + bbox[2]) / 2,
                                (bbox[1] + bbox[3]) / 2
                            ],
                            "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1]),
                            "track_id": track_id
                        }
                        detections.append(detection)

                        # Compute velocity if we have history for this track
                        velocity = self._compute_velocity(camera_id, track_id, detection["center"])
                        detection["velocity_x"] = velocity[0]
                        detection["velocity_y"] = velocity[1]

                        # Add track update info
                        track_state = self._get_track_state(camera_id, track_id)
                        tracks.append({
                            "track_id": track_id,
                            "state": track_state,
                            "detection": detection,
                            "age": self._get_track_age(camera_id, track_id),
                            "time_since_update": 0  # Just updated
                        })

            # Update track history for velocity computation
            self._update_track_history(camera_id, detections)

            inference_time = time.time() - start_time

            result = {
                "detections": detections,
                "tracks": tracks,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": getattr(self, 'model_name', 'YOLO11N'),
                "conf_threshold": conf_threshold,
                "tracker_type": self.tracker_type
            }

            if effective_filter:
                result["classes_filter"] = effective_filter

            return result

        except Exception as e:
            logger.error(f"Detection with tracking failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")

    def _compute_velocity(self, camera_id: str, track_id: int, current_center: List[float]) -> Tuple[float, float]:
        """Compute velocity in pixels per frame for a tracked object"""
        if camera_id not in self.track_history:
            return (0.0, 0.0)

        history = self.track_history[camera_id]
        if track_id not in history:
            return (0.0, 0.0)

        prev = history[track_id]
        prev_center = prev.get("center", current_center)

        # Simple velocity: pixels moved since last frame
        vx = current_center[0] - prev_center[0]
        vy = current_center[1] - prev_center[1]
        return (vx, vy)

    def _get_track_state(self, camera_id: str, track_id: int) -> str:
        """Get state of a track: new, active, lost"""
        if camera_id not in self.track_history:
            return "new"

        if track_id not in self.track_history[camera_id]:
            return "new"

        return "active"

    def _get_track_age(self, camera_id: str, track_id: int) -> int:
        """Get number of frames a track has been active"""
        if camera_id not in self.track_history:
            return 0

        if track_id not in self.track_history[camera_id]:
            return 0

        return self.track_history[camera_id][track_id].get("age", 0) + 1

    def _update_track_history(self, camera_id: str, detections: List[Dict]):
        """Update track history for velocity computation"""
        if camera_id not in self.track_history:
            self.track_history[camera_id] = {}

        # Get current track IDs
        current_track_ids = set()
        for det in detections:
            track_id = det.get("track_id", 0)
            if track_id > 0:
                current_track_ids.add(track_id)
                if track_id in self.track_history[camera_id]:
                    # Update existing track
                    self.track_history[camera_id][track_id]["center"] = det["center"]
                    self.track_history[camera_id][track_id]["age"] += 1
                    self.track_history[camera_id][track_id]["time_since_update"] = 0
                else:
                    # New track
                    self.track_history[camera_id][track_id] = {
                        "center": det["center"],
                        "age": 0,
                        "time_since_update": 0
                    }

        # Increment time_since_update for tracks not seen this frame
        # and remove stale tracks
        stale_tracks = []
        for track_id in self.track_history[camera_id]:
            if track_id not in current_track_ids:
                self.track_history[camera_id][track_id]["time_since_update"] += 1
                if self.track_history[camera_id][track_id]["time_since_update"] > 30:
                    stale_tracks.append(track_id)

        for track_id in stale_tracks:
            del self.track_history[camera_id][track_id]

    def detect_and_annotate(
        self,
        image_data: bytes,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        box_color: Optional[Tuple[int, int, int]] = None,
        box_thickness: Optional[int] = None,
        show_labels: bool = True,
        show_confidence: bool = True
    ) -> Tuple[bytes, Dict[str, Any]]:
        """Run YOLOv8 inference and return annotated image with bounding boxes using custom OpenCV drawing"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        # Use instance defaults if not provided
        color = box_color if box_color is not None else self.box_color
        thickness = box_thickness if box_thickness is not None else self.box_thickness

        try:
            # Preprocess image
            image = self.preprocess_image(image_data)

            # Use provided filter or fall back to default
            effective_filter = classes_filter if classes_filter else self.default_classes_filter
            class_ids = self._get_class_ids(effective_filter)

            # Run inference with class filtering at model level (faster than post-filtering)
            results = self.model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)

            # Parse detections for metadata and draw bounding boxes
            detections = []
            annotated_frame = image.copy()

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

                        # Draw bounding box with custom color
                        x1, y1, x2, y2 = int(bbox[0]), int(bbox[1]), int(bbox[2]), int(bbox[3])
                        cv2.rectangle(annotated_frame, (x1, y1), (x2, y2), color, thickness)

                        # Draw label if requested
                        if show_labels or show_confidence:
                            label_parts = []
                            if show_labels:
                                label_parts.append(class_name)
                            if show_confidence:
                                label_parts.append(f"{confidence:.0%}")
                            label_text = " ".join(label_parts)

                            # Calculate label background size
                            font = cv2.FONT_HERSHEY_SIMPLEX
                            font_scale = 0.5
                            font_thickness = 1
                            (text_width, text_height), baseline = cv2.getTextSize(
                                label_text, font, font_scale, font_thickness
                            )

                            # Draw label background
                            label_y1 = max(0, y1 - text_height - 8)
                            cv2.rectangle(
                                annotated_frame,
                                (x1, label_y1),
                                (x1 + text_width + 4, y1),
                                color,
                                -1  # Filled
                            )

                            # Draw label text (white on colored background)
                            cv2.putText(
                                annotated_frame,
                                label_text,
                                (x1 + 2, y1 - 4),
                                font,
                                font_scale,
                                (255, 255, 255),
                                font_thickness,
                                cv2.LINE_AA
                            )

            inference_time = time.time() - start_time

            # Convert annotated image to JPEG bytes
            _, jpeg_data = cv2.imencode('.jpg', annotated_frame, [cv2.IMWRITE_JPEG_QUALITY, 90])

            result_info = {
                "detections": detections,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": getattr(self, 'model_name', 'YOLO11N'),
                "conf_threshold": conf_threshold
            }

            # Include filter info if applied
            if effective_filter:
                result_info["classes_filter"] = effective_filter

            return jpeg_data.tobytes(), result_info

        except Exception as e:
            logger.error(f"Detection with annotation failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")

    def detect_and_annotate_with_tracking(
        self,
        image_data: bytes,
        camera_id: str = "default",
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        box_color: Optional[Tuple[int, int, int]] = None,
        box_thickness: Optional[int] = None,
        show_labels: bool = True,
        show_confidence: bool = True
    ) -> Tuple[bytes, Dict[str, Any]]:
        """Run YOLOv8 with tracking and return annotated image with track IDs using custom OpenCV drawing"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        if not self.tracking_enabled:
            # Fall back to regular annotated detection
            return self.detect_and_annotate(
                image_data, conf_threshold, classes_filter,
                box_color=box_color, box_thickness=box_thickness,
                show_labels=show_labels, show_confidence=show_confidence
            )

        start_time = time.time()

        # Use instance defaults if not provided
        color = box_color if box_color is not None else self.box_color
        thickness = box_thickness if box_thickness is not None else self.box_thickness

        try:
            # Preprocess image
            image = self.preprocess_image(image_data)

            # Use provided filter or fall back to default
            effective_filter = classes_filter if classes_filter else self.default_classes_filter
            class_ids = self._get_class_ids(effective_filter)

            # Run tracking
            results = self.model.track(
                image,
                device=self.device,
                conf=conf_threshold,
                classes=class_ids,
                persist=True,
                tracker=f"{self.tracker_type}.yaml",
                verbose=False
            )

            # Parse results with track IDs and draw bounding boxes
            detections = []
            tracks = []
            annotated_frame = image.copy()

            for r in results:
                boxes = r.boxes
                if boxes is not None and len(boxes) > 0:
                    for i in range(len(boxes)):
                        cls_id = int(boxes.cls[i])
                        class_name = r.names[cls_id]
                        confidence = float(boxes.conf[i])
                        bbox = boxes.xyxy[i].tolist()

                        track_id = 0
                        if boxes.id is not None and i < len(boxes.id):
                            track_id = int(boxes.id[i])

                        detection = {
                            "class": class_name,
                            "class_id": cls_id,
                            "confidence": confidence,
                            "bbox": bbox,
                            "center": [
                                (bbox[0] + bbox[2]) / 2,
                                (bbox[1] + bbox[3]) / 2
                            ],
                            "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1]),
                            "track_id": track_id
                        }

                        velocity = self._compute_velocity(camera_id, track_id, detection["center"])
                        detection["velocity_x"] = velocity[0]
                        detection["velocity_y"] = velocity[1]

                        detections.append(detection)

                        track_state = self._get_track_state(camera_id, track_id)
                        tracks.append({
                            "track_id": track_id,
                            "state": track_state,
                            "detection": detection,
                            "age": self._get_track_age(camera_id, track_id),
                            "time_since_update": 0
                        })

                        # Draw bounding box with custom color
                        x1, y1, x2, y2 = int(bbox[0]), int(bbox[1]), int(bbox[2]), int(bbox[3])
                        cv2.rectangle(annotated_frame, (x1, y1), (x2, y2), color, thickness)

                        # Draw label if requested
                        if show_labels or show_confidence or track_id > 0:
                            label_parts = []
                            if show_labels:
                                label_parts.append(class_name)
                            if track_id > 0:
                                label_parts.append(f"#{track_id}")
                            if show_confidence:
                                label_parts.append(f"{confidence:.0%}")
                            label_text = " ".join(label_parts)

                            # Calculate label background size
                            font = cv2.FONT_HERSHEY_SIMPLEX
                            font_scale = 0.5
                            font_thickness = 1
                            (text_width, text_height), baseline = cv2.getTextSize(
                                label_text, font, font_scale, font_thickness
                            )

                            # Draw label background
                            label_y1 = max(0, y1 - text_height - 8)
                            cv2.rectangle(
                                annotated_frame,
                                (x1, label_y1),
                                (x1 + text_width + 4, y1),
                                color,
                                -1  # Filled
                            )

                            # Draw label text (white on colored background)
                            cv2.putText(
                                annotated_frame,
                                label_text,
                                (x1 + 2, y1 - 4),
                                font,
                                font_scale,
                                (255, 255, 255),
                                font_thickness,
                                cv2.LINE_AA
                            )

            # Update track history
            self._update_track_history(camera_id, detections)

            inference_time = time.time() - start_time

            _, jpeg_data = cv2.imencode('.jpg', annotated_frame, [cv2.IMWRITE_JPEG_QUALITY, 90])

            result_info = {
                "detections": detections,
                "tracks": tracks,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": str(self.device),
                "model_size": getattr(self, 'model_name', 'YOLO11N'),
                "conf_threshold": conf_threshold,
                "tracker_type": self.tracker_type
            }

            if effective_filter:
                result_info["classes_filter"] = effective_filter

            return jpeg_data.tobytes(), result_info

        except Exception as e:
            logger.error(f"Tracked detection with annotation failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")


# Global service instance
detection_service = YOLODetectionService()

@app.get("/")
async def root():
    """Root endpoint with service info"""
    return {
        "service": "Orbo YOLO11 Detection Service",
        "version": "2.0.0",
        "model": getattr(detection_service, 'model_name', 'YOLO11N'),
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


def start_grpc_server():
    """Start gRPC server in a background thread"""
    grpc_port = int(os.getenv('GRPC_PORT', '50051'))
    if grpc_port <= 0:
        logger.info("gRPC server disabled (GRPC_PORT not set or 0)")
        return None

    try:
        from grpc_server import serve as grpc_serve
        grpc_server = grpc_serve(detection_service, port=grpc_port)
        return grpc_server
    except ImportError as e:
        logger.warning(f"gRPC server not available (missing proto files): {e}")
        return None
    except Exception as e:
        logger.error(f"Failed to start gRPC server: {e}")
        return None


if __name__ == "__main__":
    import uvicorn
    import threading

    # Start gRPC server in background thread
    grpc_server = start_grpc_server()

    # Start HTTP server (main thread)
    port = int(os.getenv('PORT', 8081))
    host = os.getenv('HOST', '0.0.0.0')

    logger.info(f"Starting YOLO11 service - HTTP on {host}:{port}")

    try:
        uvicorn.run(
            "main:app",
            host=host,
            port=port,
            reload=False,
            workers=1  # Single worker for GPU efficiency
        )
    finally:
        if grpc_server:
            logger.info("Stopping gRPC server...")
            grpc_server.stop(grace=5)