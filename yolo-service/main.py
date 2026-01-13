#!/usr/bin/env python3
"""
GPU-accelerated YOLO11 Multi-Task Detection Service
Supports: Detection, Pose Estimation, Segmentation, OBB, Classification
Optimized for Orbo video alarm system with real-time per-frame analysis
"""

import os
import logging
from collections import OrderedDict
from enum import Enum

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
from typing import List, Dict, Any, Optional, Tuple, Set
import base64

app = FastAPI(
    title="Orbo YOLO11 Multi-Task Service",
    description="GPU-accelerated multi-task AI for real-time video surveillance",
    version="3.0.0"
)


class YoloTask(str, Enum):
    """Supported YOLO11 tasks"""
    DETECT = "detect"
    POSE = "pose"
    SEGMENT = "segment"
    OBB = "obb"
    CLASSIFY = "classify"


# Model file mapping for each task
TASK_MODEL_FILES = {
    YoloTask.DETECT: "yolo11{size}.pt",
    YoloTask.POSE: "yolo11{size}-pose.pt",
    YoloTask.SEGMENT: "yolo11{size}-seg.pt",
    YoloTask.OBB: "yolo11{size}-obb.pt",
    YoloTask.CLASSIFY: "yolo11{size}-cls.pt",
}

# Drawing order (bottom to top) for proper Z-ordering
TASK_DRAW_ORDER = [YoloTask.SEGMENT, YoloTask.OBB, YoloTask.DETECT, YoloTask.POSE]

# COCO keypoint names for pose estimation
KEYPOINT_NAMES = [
    'nose', 'left_eye', 'right_eye', 'left_ear', 'right_ear',
    'left_shoulder', 'right_shoulder', 'left_elbow', 'right_elbow',
    'left_wrist', 'right_wrist', 'left_hip', 'right_hip',
    'left_knee', 'right_knee', 'left_ankle', 'right_ankle'
]

# Skeleton connections for pose drawing
SKELETON_CONNECTIONS = [
    (0, 1), (0, 2), (1, 3), (2, 4),  # Head
    (5, 6), (5, 7), (7, 9), (6, 8), (8, 10),  # Arms
    (5, 11), (6, 12), (11, 12),  # Torso
    (11, 13), (13, 15), (12, 14), (14, 16)  # Legs
]


class LRUModelCache:
    """LRU cache for YOLO models to manage memory"""

    def __init__(self, max_size: int = 3):
        self.max_size = max_size
        self.cache: OrderedDict[str, YOLO] = OrderedDict()
        self.device = None

    def set_device(self, device: str):
        self.device = device

    def get(self, model_path: str) -> Optional[YOLO]:
        """Get model from cache, updating LRU order"""
        if model_path in self.cache:
            # Move to end (most recently used)
            self.cache.move_to_end(model_path)
            return self.cache[model_path]
        return None

    def put(self, model_path: str, model: YOLO):
        """Add model to cache, evicting LRU if necessary"""
        if model_path in self.cache:
            self.cache.move_to_end(model_path)
            return

        # Evict LRU if at capacity
        while len(self.cache) >= self.max_size:
            evicted_path, evicted_model = self.cache.popitem(last=False)
            logger.info(f"Evicting model from cache: {evicted_path}")
            del evicted_model
            if torch.cuda.is_available():
                torch.cuda.empty_cache()

        self.cache[model_path] = model
        logger.info(f"Cached model: {model_path} (cache size: {len(self.cache)})")

    def get_loaded_models(self) -> List[str]:
        """Get list of currently loaded model paths"""
        return list(self.cache.keys())


class MultiTaskYOLOService:
    """Multi-task YOLO11 service with lazy model loading and LRU caching"""

    def __init__(self):
        self.device = None
        self.model_size = os.getenv('MODEL_SIZE', 'n')  # n, s, m, l, x
        self.max_cached_models = int(os.getenv('MAX_CACHED_MODELS', '3'))
        self.model_cache = LRUModelCache(max_size=self.max_cached_models)

        # Enabled tasks (from environment)
        self.enabled_tasks: Set[YoloTask] = self._parse_enabled_tasks()

        # Tracking configuration
        self.tracker_type = os.getenv('TRACKER_TYPE', '')
        self.tracking_enabled = self.tracker_type != ''
        self.track_history: Dict[str, Dict] = {}

        # Appearance configuration
        self.box_color: Tuple[int, int, int] = self._hex_to_bgr(os.getenv('BOX_COLOR', '#0066FF'))
        self.box_thickness: int = int(os.getenv('BOX_THICKNESS', '2'))
        self.pose_color: Tuple[int, int, int] = self._hex_to_bgr(os.getenv('POSE_COLOR', '#00FF00'))
        self.segment_color: Tuple[int, int, int] = self._hex_to_bgr(os.getenv('SEGMENT_COLOR', '#FF00FF'))
        self.segment_alpha: float = float(os.getenv('SEGMENT_ALPHA', '0.4'))
        self.obb_color: Tuple[int, int, int] = self._hex_to_bgr(os.getenv('OBB_COLOR', '#FFFF00'))

        # Pose alert configuration
        self.fall_detection_enabled = os.getenv('FALL_DETECTION', 'true').lower() == 'true'
        self.alert_on_poses: List[str] = os.getenv('ALERT_ON_POSES', 'fallen,crouching').split(',')

        # Default class filter
        self.default_classes_filter = None
        self._load_default_classes_filter()

        # Initialize device
        self._initialize_device()

        # Preload models if specified
        self._preload_models()

        logger.info(f"MultiTaskYOLOService initialized")
        logger.info(f"  Device: {self.device}")
        logger.info(f"  Model size: {self.model_size}")
        logger.info(f"  Enabled tasks: {[t.value for t in self.enabled_tasks]}")
        logger.info(f"  Max cached models: {self.max_cached_models}")

    def _parse_enabled_tasks(self) -> Set[YoloTask]:
        """Parse enabled tasks from environment"""
        tasks_env = os.getenv('ENABLED_TASKS', 'detect')
        task_names = [t.strip().lower() for t in tasks_env.split(',') if t.strip()]

        enabled = set()
        for name in task_names:
            try:
                enabled.add(YoloTask(name))
            except ValueError:
                logger.warning(f"Unknown task '{name}', skipping")

        # Always enable detect as fallback
        if not enabled:
            enabled.add(YoloTask.DETECT)

        return enabled

    def _initialize_device(self):
        """Initialize compute device"""
        if torch.cuda.is_available():
            self.device = 'cuda'
            gpu_name = torch.cuda.get_device_name(0)
            logger.info(f"GPU detected: {gpu_name}")
        else:
            self.device = 'cpu'
            logger.warning("No GPU detected, using CPU")

        self.model_cache.set_device(self.device)

    def _preload_models(self):
        """Preload models specified in environment"""
        preload_tasks = os.getenv('PRELOAD_TASKS', 'detect')
        if not preload_tasks:
            return

        task_names = [t.strip().lower() for t in preload_tasks.split(',') if t.strip()]
        for name in task_names:
            try:
                task = YoloTask(name)
                if task in self.enabled_tasks:
                    logger.info(f"Preloading model for task: {task.value}")
                    self._get_model(task)
            except ValueError:
                logger.warning(f"Unknown preload task '{name}', skipping")

    def _load_default_classes_filter(self):
        """Load default class filter from environment variable"""
        classes_filter_env = os.getenv('CLASSES_FILTER', '')
        if classes_filter_env:
            self.default_classes_filter = [c.strip().lower() for c in classes_filter_env.split(',') if c.strip()]
            logger.info(f"Default class filter set: {self.default_classes_filter}")
        else:
            self.default_classes_filter = None

    def _hex_to_bgr(self, hex_color: str) -> Tuple[int, int, int]:
        """Convert hex color string to BGR tuple for OpenCV"""
        hex_color = hex_color.lstrip('#')
        if len(hex_color) != 6:
            return (255, 102, 0)  # Default blue
        r = int(hex_color[0:2], 16)
        g = int(hex_color[2:4], 16)
        b = int(hex_color[4:6], 16)
        return (b, g, r)

    def _get_model_path(self, task: YoloTask) -> str:
        """Get model file path for a task"""
        template = TASK_MODEL_FILES.get(task)
        if not template:
            raise ValueError(f"Unknown task: {task}")
        return template.format(size=self.model_size)

    def _get_model(self, task: YoloTask) -> YOLO:
        """Get or load model for a task"""
        if task not in self.enabled_tasks:
            raise HTTPException(status_code=400, detail=f"Task '{task.value}' is not enabled")

        model_path = self._get_model_path(task)

        # Check cache first
        model = self.model_cache.get(model_path)
        if model is not None:
            return model

        # Load model
        logger.info(f"Loading model: {model_path}")
        try:
            model = YOLO(model_path)
            model.to(self.device)

            # Warm up
            dummy_image = np.zeros((640, 640, 3), dtype=np.uint8)
            _ = model(dummy_image, verbose=False, save=False)

            # Cache model
            self.model_cache.put(model_path, model)

            return model
        except Exception as e:
            logger.error(f"Failed to load model {model_path}: {e}")
            raise HTTPException(status_code=503, detail=f"Failed to load model: {str(e)}")

    def _get_class_ids(self, model: YOLO, class_names: Optional[List[str]]) -> Optional[List[int]]:
        """Convert class names to YOLO class IDs"""
        if not class_names:
            return None

        name_to_id = {v.lower(): k for k, v in model.names.items()}
        class_ids = []
        for name in class_names:
            name_lower = name.lower()
            if name_lower in name_to_id:
                class_ids.append(name_to_id[name_lower])

        return class_ids if class_ids else None

    def preprocess_image(self, image_data: bytes) -> np.ndarray:
        """Preprocess image for inference"""
        try:
            image = Image.open(io.BytesIO(image_data))
            if image.mode != 'RGB':
                image = image.convert('RGB')
            rgb_array = np.array(image)
            bgr_array = cv2.cvtColor(rgb_array, cv2.COLOR_RGB2BGR)
            return bgr_array
        except Exception as e:
            logger.error(f"Image preprocessing failed: {e}")
            raise HTTPException(status_code=400, detail="Invalid image format")

    # =========================================================================
    # Task-specific inference methods
    # =========================================================================

    def run_detection(
        self,
        image: np.ndarray,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Run object detection"""
        model = self._get_model(YoloTask.DETECT)
        effective_filter = classes_filter or self.default_classes_filter
        class_ids = self._get_class_ids(model, effective_filter)

        results = model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)

        detections = []
        for r in results:
            if r.boxes is not None:
                for i in range(len(r.boxes)):
                    bbox = r.boxes.xyxy[i].tolist()
                    detections.append({
                        "class": r.names[int(r.boxes.cls[i])],
                        "class_id": int(r.boxes.cls[i]),
                        "confidence": float(r.boxes.conf[i]),
                        "bbox": bbox,
                        "center": [(bbox[0] + bbox[2]) / 2, (bbox[1] + bbox[3]) / 2],
                        "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1])
                    })

        return {"detections": detections, "count": len(detections)}

    def run_pose(
        self,
        image: np.ndarray,
        conf_threshold: float = 0.5
    ) -> Dict[str, Any]:
        """Run pose estimation"""
        model = self._get_model(YoloTask.POSE)

        results = model(image, device=self.device, conf=conf_threshold, verbose=False)

        poses = []
        alerts = []

        for r in results:
            if r.keypoints is not None and r.boxes is not None:
                for i in range(len(r.keypoints)):
                    kp = r.keypoints[i]
                    bbox = r.boxes.xyxy[i].tolist() if i < len(r.boxes) else None

                    # Extract keypoints
                    keypoints_xy = kp.xy[0].tolist() if kp.xy is not None else []
                    keypoints_conf = kp.conf[0].tolist() if kp.conf is not None else []

                    # Classify pose
                    pose_analysis = self._classify_pose(keypoints_xy, keypoints_conf, bbox)

                    pose_data = {
                        "keypoints": keypoints_xy,
                        "keypoint_confidence": keypoints_conf,
                        "keypoint_names": KEYPOINT_NAMES,
                        "bbox": bbox,
                        "confidence": float(r.boxes.conf[i]) if bbox else 0.0,
                        "pose_class": pose_analysis["class"],
                        "pose_confidence": pose_analysis["confidence"],
                        "body_metrics": pose_analysis["metrics"]
                    }
                    poses.append(pose_data)

                    # Check for alerts
                    if pose_analysis["class"] in self.alert_on_poses:
                        alerts.append({
                            "type": f"{pose_analysis['class']}_detected",
                            "severity": "high" if pose_analysis["class"] == "fallen" else "medium",
                            "pose_index": len(poses) - 1,
                            "confidence": pose_analysis["confidence"]
                        })

        return {"poses": poses, "count": len(poses), "alerts": alerts}

    def _classify_pose(
        self,
        keypoints: List[List[float]],
        confidences: List[float],
        bbox: Optional[List[float]]
    ) -> Dict[str, Any]:
        """Classify pose based on keypoints"""
        result = {
            "class": "unknown",
            "confidence": 0.0,
            "metrics": {}
        }

        if len(keypoints) < 17 or len(confidences) < 17:
            return result

        # Build keypoint dict with confidence check
        kp = {}
        for i, name in enumerate(KEYPOINT_NAMES):
            if i < len(keypoints) and i < len(confidences):
                if confidences[i] > 0.3:
                    kp[name] = {"xy": keypoints[i], "conf": confidences[i]}

        # Calculate key measurements
        try:
            # Nose position
            nose_y = kp.get('nose', {}).get('xy', [0, 0])[1] if 'nose' in kp else None

            # Hip center
            hip_y = None
            if 'left_hip' in kp and 'right_hip' in kp:
                hip_y = (kp['left_hip']['xy'][1] + kp['right_hip']['xy'][1]) / 2

            # Shoulder center
            shoulder_y = None
            if 'left_shoulder' in kp and 'right_shoulder' in kp:
                shoulder_y = (kp['left_shoulder']['xy'][1] + kp['right_shoulder']['xy'][1]) / 2

            # Ankle center
            ankle_y = None
            if 'left_ankle' in kp and 'right_ankle' in kp:
                ankle_y = (kp['left_ankle']['xy'][1] + kp['right_ankle']['xy'][1]) / 2

            # Wrist positions
            left_wrist_y = kp.get('left_wrist', {}).get('xy', [0, 9999])[1]
            right_wrist_y = kp.get('right_wrist', {}).get('xy', [0, 9999])[1]

            # Store metrics
            result["metrics"] = {
                "nose_y": nose_y,
                "shoulder_y": shoulder_y,
                "hip_y": hip_y,
                "ankle_y": ankle_y
            }

            # Pose classification logic
            if nose_y is not None and hip_y is not None:
                # Fall detection: head below hips (in image coords, higher Y = lower position)
                if nose_y > hip_y + 50:
                    result["class"] = "fallen"
                    result["confidence"] = 0.9
                    return result

            if shoulder_y is not None and hip_y is not None and ankle_y is not None:
                torso_height = abs(hip_y - shoulder_y)
                leg_height = abs(ankle_y - hip_y)

                result["metrics"]["torso_height"] = torso_height
                result["metrics"]["leg_height"] = leg_height

                # Crouching: torso much taller than legs (person bent down)
                if torso_height > leg_height * 1.5 and leg_height < 100:
                    result["class"] = "crouching"
                    result["confidence"] = 0.8
                    return result

                # Lying down: shoulder and hip at similar Y (horizontal body)
                if abs(shoulder_y - hip_y) < 30:
                    result["class"] = "lying"
                    result["confidence"] = 0.85
                    return result

            # Arms raised
            if shoulder_y is not None:
                if left_wrist_y < shoulder_y - 50 and right_wrist_y < shoulder_y - 50:
                    result["class"] = "arms_raised"
                    result["confidence"] = 0.8
                    return result

            # Default: standing
            result["class"] = "standing"
            result["confidence"] = 0.9

        except Exception as e:
            logger.warning(f"Pose classification failed: {e}")
            result["class"] = "unknown"
            result["confidence"] = 0.0

        return result

    def run_segmentation(
        self,
        image: np.ndarray,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Run instance segmentation"""
        model = self._get_model(YoloTask.SEGMENT)
        effective_filter = classes_filter or self.default_classes_filter
        class_ids = self._get_class_ids(model, effective_filter)

        results = model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)

        segments = []
        for r in results:
            if r.masks is not None and r.boxes is not None:
                for i in range(len(r.masks)):
                    mask = r.masks[i]
                    bbox = r.boxes.xyxy[i].tolist()

                    # Get polygon points
                    polygon = mask.xy[0].tolist() if mask.xy is not None and len(mask.xy) > 0 else []

                    # Calculate mask area
                    mask_area = int(mask.data.sum().item()) if mask.data is not None else 0

                    segments.append({
                        "class": r.names[int(r.boxes.cls[i])],
                        "class_id": int(r.boxes.cls[i]),
                        "confidence": float(r.boxes.conf[i]),
                        "bbox": bbox,
                        "polygon": polygon,
                        "mask_area_pixels": mask_area
                    })

        return {"segments": segments, "count": len(segments)}

    def run_obb(
        self,
        image: np.ndarray,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Run oriented bounding box detection"""
        model = self._get_model(YoloTask.OBB)
        effective_filter = classes_filter or self.default_classes_filter
        class_ids = self._get_class_ids(model, effective_filter)

        results = model(image, device=self.device, conf=conf_threshold, classes=class_ids, verbose=False)

        obbs = []
        for r in results:
            if r.obb is not None:
                for i in range(len(r.obb)):
                    obb = r.obb[i]

                    # Get rotated box corners
                    xyxyxyxy = obb.xyxyxyxy[0].tolist() if obb.xyxyxyxy is not None else []

                    # Get center, width, height, angle
                    xywhr = obb.xywhr[0].tolist() if obb.xywhr is not None else [0, 0, 0, 0, 0]

                    obbs.append({
                        "class": r.names[int(obb.cls[0])] if obb.cls is not None else "unknown",
                        "class_id": int(obb.cls[0]) if obb.cls is not None else -1,
                        "confidence": float(obb.conf[0]) if obb.conf is not None else 0.0,
                        "corners": xyxyxyxy,  # 4 corner points
                        "center": [xywhr[0], xywhr[1]],
                        "width": xywhr[2],
                        "height": xywhr[3],
                        "angle_rad": xywhr[4]
                    })

        return {"obbs": obbs, "count": len(obbs)}

    def run_classification(
        self,
        image: np.ndarray,
        top_k: int = 5
    ) -> Dict[str, Any]:
        """Run image classification"""
        model = self._get_model(YoloTask.CLASSIFY)

        results = model(image, device=self.device, verbose=False)

        classifications = []
        for r in results:
            if r.probs is not None:
                probs = r.probs
                top_indices = probs.top5 if hasattr(probs, 'top5') else []
                top_confs = probs.top5conf.tolist() if hasattr(probs, 'top5conf') else []

                for idx, conf in zip(top_indices[:top_k], top_confs[:top_k]):
                    classifications.append({
                        "class": r.names[idx],
                        "class_id": int(idx),
                        "confidence": float(conf)
                    })

        return {"classifications": classifications, "count": len(classifications)}

    # =========================================================================
    # Multi-task analysis with combined annotations
    # =========================================================================

    def analyze(
        self,
        image_data: bytes,
        tasks: List[str],
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        return_annotated: bool = True
    ) -> Dict[str, Any]:
        """
        Run multiple YOLO tasks on the same frame with combined annotations.
        """
        start_time = time.time()

        # Preprocess image once
        original_image = self.preprocess_image(image_data)
        annotated_frame = original_image.copy() if return_annotated else None

        # Parse and validate tasks
        valid_tasks = []
        for task_name in tasks:
            try:
                task = YoloTask(task_name.lower())
                if task in self.enabled_tasks:
                    valid_tasks.append(task)
                else:
                    logger.warning(f"Task '{task_name}' not enabled, skipping")
            except ValueError:
                logger.warning(f"Unknown task '{task_name}', skipping")

        if not valid_tasks:
            valid_tasks = [YoloTask.DETECT]

        # Run tasks and collect results
        results = {
            "frame_timestamp": time.time(),
            "tasks": {},
            "alerts": []
        }

        # Sort tasks by draw order
        sorted_tasks = sorted(
            valid_tasks,
            key=lambda t: TASK_DRAW_ORDER.index(t) if t in TASK_DRAW_ORDER else 99
        )

        for task in sorted_tasks:
            try:
                if task == YoloTask.DETECT:
                    task_result = self.run_detection(original_image, conf_threshold, classes_filter)
                    if annotated_frame is not None:
                        self._draw_detections(annotated_frame, task_result["detections"])

                elif task == YoloTask.POSE:
                    task_result = self.run_pose(original_image, conf_threshold)
                    if annotated_frame is not None:
                        self._draw_poses(annotated_frame, task_result["poses"])
                    # Collect alerts
                    results["alerts"].extend(task_result.get("alerts", []))

                elif task == YoloTask.SEGMENT:
                    task_result = self.run_segmentation(original_image, conf_threshold, classes_filter)
                    if annotated_frame is not None:
                        self._draw_segments(annotated_frame, task_result["segments"])

                elif task == YoloTask.OBB:
                    task_result = self.run_obb(original_image, conf_threshold, classes_filter)
                    if annotated_frame is not None:
                        self._draw_obbs(annotated_frame, task_result["obbs"])

                elif task == YoloTask.CLASSIFY:
                    task_result = self.run_classification(original_image)
                    # No drawing for classification

                else:
                    continue

                results["tasks"][task.value] = task_result

            except Exception as e:
                logger.error(f"Task {task.value} failed: {e}")
                results["tasks"][task.value] = {"error": str(e)}

        # Encode annotated image
        if annotated_frame is not None:
            _, jpeg_data = cv2.imencode('.jpg', annotated_frame, [cv2.IMWRITE_JPEG_QUALITY, 90])
            results["annotated_image"] = base64.b64encode(jpeg_data.tobytes()).decode('utf-8')

        results["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        results["device"] = str(self.device)
        results["tasks_executed"] = [t.value for t in sorted_tasks]

        return results

    # =========================================================================
    # Drawing methods for annotations
    # =========================================================================

    def _draw_detections(self, frame: np.ndarray, detections: List[Dict]):
        """Draw detection bounding boxes"""
        for det in detections:
            bbox = det["bbox"]
            x1, y1, x2, y2 = int(bbox[0]), int(bbox[1]), int(bbox[2]), int(bbox[3])

            # Draw box
            cv2.rectangle(frame, (x1, y1), (x2, y2), self.box_color, self.box_thickness)

            # Draw label
            label = f"{det['class']} {det['confidence']:.0%}"
            self._draw_label(frame, label, (x1, y1), self.box_color)

    def _draw_poses(self, frame: np.ndarray, poses: List[Dict]):
        """Draw pose skeletons"""
        for pose in poses:
            keypoints = pose.get("keypoints", [])
            confidences = pose.get("keypoint_confidence", [])

            if len(keypoints) < 17:
                continue

            # Draw skeleton connections
            for conn in SKELETON_CONNECTIONS:
                if conn[0] < len(keypoints) and conn[1] < len(keypoints):
                    pt1 = keypoints[conn[0]]
                    pt2 = keypoints[conn[1]]
                    conf1 = confidences[conn[0]] if conn[0] < len(confidences) else 0
                    conf2 = confidences[conn[1]] if conn[1] < len(confidences) else 0

                    if conf1 > 0.3 and conf2 > 0.3:
                        cv2.line(
                            frame,
                            (int(pt1[0]), int(pt1[1])),
                            (int(pt2[0]), int(pt2[1])),
                            self.pose_color,
                            2
                        )

            # Draw keypoints
            for i, (kp, conf) in enumerate(zip(keypoints, confidences)):
                if conf > 0.3:
                    cv2.circle(frame, (int(kp[0]), int(kp[1])), 4, self.pose_color, -1)

            # Draw pose class label if not standing
            pose_class = pose.get("pose_class", "unknown")
            if pose_class not in ["standing", "unknown"]:
                bbox = pose.get("bbox")
                if bbox:
                    label = f"POSE: {pose_class.upper()}"
                    color = (0, 0, 255) if pose_class == "fallen" else (0, 165, 255)
                    self._draw_label(frame, label, (int(bbox[0]), int(bbox[1]) - 20), color)

    def _draw_segments(self, frame: np.ndarray, segments: List[Dict]):
        """Draw segmentation masks"""
        overlay = frame.copy()

        for seg in segments:
            polygon = seg.get("polygon", [])
            if not polygon:
                continue

            # Draw filled polygon
            pts = np.array(polygon, dtype=np.int32)
            cv2.fillPoly(overlay, [pts], self.segment_color)

            # Draw outline
            cv2.polylines(frame, [pts], True, self.segment_color, 2)

            # Draw label
            bbox = seg.get("bbox")
            if bbox:
                label = f"{seg['class']} {seg['confidence']:.0%}"
                self._draw_label(frame, label, (int(bbox[0]), int(bbox[1])), self.segment_color)

        # Blend overlay with frame
        cv2.addWeighted(overlay, self.segment_alpha, frame, 1 - self.segment_alpha, 0, frame)

    def _draw_obbs(self, frame: np.ndarray, obbs: List[Dict]):
        """Draw oriented bounding boxes"""
        for obb in obbs:
            corners = obb.get("corners", [])
            if len(corners) < 4:
                continue

            # Draw rotated rectangle
            pts = np.array(corners, dtype=np.int32)
            cv2.polylines(frame, [pts], True, self.obb_color, self.box_thickness)

            # Draw label at first corner
            label = f"{obb['class']} {obb['confidence']:.0%}"
            self._draw_label(frame, label, (int(corners[0][0]), int(corners[0][1])), self.obb_color)

    def _draw_label(self, frame: np.ndarray, text: str, position: Tuple[int, int], color: Tuple[int, int, int]):
        """Draw label with background"""
        font = cv2.FONT_HERSHEY_SIMPLEX
        font_scale = 0.5
        font_thickness = 1

        (text_width, text_height), baseline = cv2.getTextSize(text, font, font_scale, font_thickness)

        x, y = position
        label_y1 = max(0, y - text_height - 8)

        # Draw background
        cv2.rectangle(frame, (x, label_y1), (x + text_width + 4, y), color, -1)

        # Draw text
        cv2.putText(frame, text, (x + 2, y - 4), font, font_scale, (255, 255, 255), font_thickness, cv2.LINE_AA)

    # =========================================================================
    # Legacy single-task methods for backward compatibility
    # =========================================================================

    def detect_objects(
        self,
        image_data: bytes,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Legacy method: Run object detection"""
        image = self.preprocess_image(image_data)
        start_time = time.time()
        result = self.run_detection(image, conf_threshold, classes_filter)
        result["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        result["device"] = str(self.device)
        return result

    def detect_and_annotate(
        self,
        image_data: bytes,
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        **kwargs
    ) -> Tuple[bytes, Dict[str, Any]]:
        """Legacy method: Detect and return annotated image"""
        result = self.analyze(
            image_data,
            tasks=["detect"],
            conf_threshold=conf_threshold,
            classes_filter=classes_filter,
            return_annotated=True
        )

        annotated_bytes = base64.b64decode(result.get("annotated_image", ""))

        return annotated_bytes, {
            "detections": result["tasks"].get("detect", {}).get("detections", []),
            "count": result["tasks"].get("detect", {}).get("count", 0),
            "inference_time_ms": result["inference_time_ms"],
            "device": result["device"]
        }

    def detect_with_tracking(
        self,
        image_data: bytes,
        camera_id: str = "",
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Legacy method: Run object detection with tracking (tracking delegated to analyze)"""
        result = self.analyze(
            image_data,
            tasks=["detect"],
            conf_threshold=conf_threshold,
            classes_filter=classes_filter,
            return_annotated=False
        )

        return {
            "detections": result["tasks"].get("detect", {}).get("detections", []),
            "count": result["tasks"].get("detect", {}).get("count", 0),
            "inference_time_ms": result["inference_time_ms"],
            "device": result["device"],
            "tracks": []  # Tracking data placeholder
        }

    def detect_and_annotate_with_tracking(
        self,
        image_data: bytes,
        camera_id: str = "",
        conf_threshold: float = 0.5,
        classes_filter: Optional[List[str]] = None,
        **kwargs
    ) -> Tuple[bytes, Dict[str, Any]]:
        """Legacy method: Detect with tracking and return annotated image"""
        result = self.analyze(
            image_data,
            tasks=["detect"],
            conf_threshold=conf_threshold,
            classes_filter=classes_filter,
            return_annotated=True
        )

        annotated_bytes = base64.b64decode(result.get("annotated_image", ""))

        return annotated_bytes, {
            "detections": result["tasks"].get("detect", {}).get("detections", []),
            "count": result["tasks"].get("detect", {}).get("count", 0),
            "inference_time_ms": result["inference_time_ms"],
            "device": result["device"],
            "tracks": []  # Tracking data placeholder
        }

    @property
    def model_loaded(self) -> bool:
        """Check if at least one model is loaded (for gRPC compatibility)"""
        return len(self.model_cache.get_loaded_models()) > 0

    def get_status(self) -> Dict[str, Any]:
        """Get service status"""
        return {
            "service": "Orbo YOLO11 Multi-Task Service",
            "version": "3.0.0",
            "device": self.device,
            "gpu_available": torch.cuda.is_available(),
            "model_size": self.model_size,
            "enabled_tasks": [t.value for t in self.enabled_tasks],
            "loaded_models": self.model_cache.get_loaded_models(),
            "max_cached_models": self.max_cached_models,
            "tracking_enabled": self.tracking_enabled,
            "fall_detection_enabled": self.fall_detection_enabled,
            "model_loaded": self.model_loaded
        }


# Global service instance
service = MultiTaskYOLOService()


# =============================================================================
# HTTP Endpoints
# =============================================================================

@app.get("/")
async def root():
    """Root endpoint with service info"""
    return service.get_status()


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    status = service.get_status()
    return {
        "status": "healthy",
        **status
    }


@app.get("/tasks")
async def get_available_tasks():
    """Get list of available tasks"""
    return {
        "available_tasks": [t.value for t in YoloTask],
        "enabled_tasks": [t.value for t in service.enabled_tasks],
        "loaded_models": service.model_cache.get_loaded_models()
    }


@app.post("/analyze")
async def analyze_frame(
    file: UploadFile = File(...),
    tasks: str = Query("detect", description="Comma-separated task names: detect,pose,segment,obb,classify"),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0),
    classes_filter: Optional[str] = Query(None, description="Comma-separated class names to filter"),
    return_annotated: bool = Query(True, description="Return annotated image")
):
    """
    Run multiple YOLO11 tasks on an image with combined annotations.

    Tasks:
    - detect: Object detection (bounding boxes)
    - pose: Human pose estimation (skeleton keypoints)
    - segment: Instance segmentation (pixel masks)
    - obb: Oriented bounding boxes (rotated boxes)
    - classify: Image classification (whole image)
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()

        task_list = [t.strip() for t in tasks.split(',') if t.strip()]
        filter_list = [c.strip() for c in classes_filter.split(',')] if classes_filter else None

        result = service.analyze(
            image_data,
            tasks=task_list,
            conf_threshold=conf_threshold,
            classes_filter=filter_list,
            return_annotated=return_annotated
        )

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Analyze failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/detect")
async def detect_objects(
    file: UploadFile = File(...),
    conf_threshold: float = 0.5,
    classes_filter: str = None
):
    """Legacy endpoint: Detect objects in uploaded image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        filter_list = [c.strip() for c in classes_filter.split(',')] if classes_filter else None
        result = service.detect_objects(image_data, conf_threshold, filter_list)
        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/detect/annotated")
async def detect_annotated(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0),
    classes_filter: Optional[str] = Query(None),
    show_labels: bool = Query(True),
    show_confidence: bool = Query(True),
    format: str = Query("image", description="Response format: 'image' or 'base64'")
):
    """Legacy endpoint: Detect objects and return annotated image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        filter_list = [c.strip() for c in classes_filter.split(',')] if classes_filter else None

        annotated_image, result_info = service.detect_and_annotate(
            image_data,
            conf_threshold=conf_threshold,
            classes_filter=filter_list
        )

        if format == "base64":
            return {
                "image": {
                    "data": base64.b64encode(annotated_image).decode('utf-8'),
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
                    "X-Device": result_info["device"]
                }
            )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Annotated detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/pose")
async def estimate_pose(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0)
):
    """Estimate human poses in uploaded image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        image = service.preprocess_image(image_data)

        start_time = time.time()
        result = service.run_pose(image, conf_threshold)
        result["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        result["device"] = str(service.device)

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Pose estimation failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/segment")
async def segment_objects(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0),
    classes_filter: Optional[str] = Query(None)
):
    """Segment objects in uploaded image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        image = service.preprocess_image(image_data)
        filter_list = [c.strip() for c in classes_filter.split(',')] if classes_filter else None

        start_time = time.time()
        result = service.run_segmentation(image, conf_threshold, filter_list)
        result["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        result["device"] = str(service.device)

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Segmentation failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/obb")
async def detect_obb(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.5, ge=0.0, le=1.0),
    classes_filter: Optional[str] = Query(None)
):
    """Detect oriented bounding boxes in uploaded image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        image = service.preprocess_image(image_data)
        filter_list = [c.strip() for c in classes_filter.split(',')] if classes_filter else None

        start_time = time.time()
        result = service.run_obb(image, conf_threshold, filter_list)
        result["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        result["device"] = str(service.device)

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"OBB detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/classify")
async def classify_image(
    file: UploadFile = File(...),
    top_k: int = Query(5, ge=1, le=10)
):
    """Classify uploaded image"""
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        image = service.preprocess_image(image_data)

        start_time = time.time()
        result = service.run_classification(image, top_k)
        result["inference_time_ms"] = round((time.time() - start_time) * 1000, 2)
        result["device"] = str(service.device)

        return result

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Classification failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/classes")
async def get_supported_classes():
    """Get list of supported object classes for detection"""
    try:
        model = service._get_model(YoloTask.DETECT)
        return {
            "classes": list(model.names.values()),
            "count": len(model.names)
        }
    except Exception as e:
        raise HTTPException(status_code=503, detail=str(e))


@app.post("/detect/security")
async def detect_security_objects(
    file: UploadFile = File(...),
    conf_threshold: float = 0.6
):
    """Legacy endpoint: Detect security-relevant objects"""
    security_classes = ["person", "car", "truck", "bus", "bicycle", "motorcycle"]

    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        result = service.detect_objects(image_data, conf_threshold, security_classes)

        threat_analysis = {
            "high_priority": [det for det in result["detections"] if det["class"] == "person"],
            "medium_priority": [det for det in result["detections"] if det["class"] in ["car", "truck", "bus"]],
            "low_priority": [det for det in result["detections"] if det["class"] in ["bicycle", "motorcycle"]]
        }

        return {
            **result,
            "threat_analysis": threat_analysis,
            "security_filter": security_classes
        }

    except Exception as e:
        logger.error(f"Security detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/detect/security/annotated")
async def detect_security_annotated(
    file: UploadFile = File(...),
    conf_threshold: float = Query(0.6, ge=0.0, le=1.0),
    format: str = Query("image", description="Response format: 'image' or 'base64'")
):
    """Legacy endpoint: Detect security-relevant objects and return annotated image"""
    security_classes = ["person", "car", "truck", "bus", "bicycle", "motorcycle"]

    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()

        annotated_image, result_info = service.detect_and_annotate(
            image_data,
            conf_threshold=conf_threshold,
            classes_filter=security_classes
        )

        # Calculate threat analysis from detections
        threat_analysis = {
            "high_priority": [det for det in result_info.get("detections", []) if det["class"] == "person"],
            "medium_priority": [det for det in result_info.get("detections", []) if det["class"] in ["car", "truck", "bus"]],
            "low_priority": [det for det in result_info.get("detections", []) if det["class"] in ["bicycle", "motorcycle"]]
        }

        if format == "base64":
            return {
                "image": {
                    "data": base64.b64encode(annotated_image).decode('utf-8'),
                    "content_type": "image/jpeg"
                },
                **result_info,
                "threat_analysis": threat_analysis,
                "security_filter": security_classes
            }
        else:
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
        logger.error(f"Security annotated detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


def start_grpc_server():
    """Start gRPC server in a background thread"""
    grpc_port = int(os.getenv('GRPC_PORT', '50051'))
    if grpc_port <= 0:
        logger.info("gRPC server disabled (GRPC_PORT not set or 0)")
        return None

    try:
        from grpc_server import serve as grpc_serve
        grpc_server = grpc_serve(service, port=grpc_port)
        return grpc_server
    except ImportError as e:
        logger.warning(f"gRPC server not available (missing proto files): {e}")
        return None
    except Exception as e:
        logger.error(f"Failed to start gRPC server: {e}")
        return None


if __name__ == "__main__":
    import uvicorn

    # Start gRPC server in background thread
    grpc_server = start_grpc_server()

    # Start HTTP server (main thread)
    port = int(os.getenv('PORT', 8081))
    host = os.getenv('HOST', '0.0.0.0')

    logger.info(f"Starting YOLO11 Multi-Task Service - HTTP on {host}:{port}")

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
