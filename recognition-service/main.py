#!/usr/bin/env python3
"""
Face Recognition Service using InsightFace
Provides face detection, embedding extraction, and identity matching for Orbo
"""

from fastapi import FastAPI, File, UploadFile, HTTPException, Query, Form
from fastapi.responses import JSONResponse, Response
import numpy as np
import cv2
import io
from PIL import Image
import time
import logging
from typing import List, Dict, Any, Optional, Tuple
import os
import base64
import json
from pathlib import Path
import pickle
from datetime import datetime

# InsightFace imports
import insightface
from insightface.app import FaceAnalysis

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="Orbo Face Recognition Service",
    description="Face detection and recognition using InsightFace for video surveillance",
    version="1.0.0"
)

# Face database path
FACES_DB_PATH = os.getenv('FACES_DB_PATH', '/app/data/faces.pkl')
FACES_IMAGES_PATH = os.getenv('FACES_IMAGES_PATH', '/app/data/faces')


class FaceRecognitionService:
    def __init__(self):
        self.face_app = None
        self.model_loaded = False
        self.device = 'cpu'  # InsightFace auto-detects GPU
        # Data model supports multiple embeddings per person:
        # name -> {embeddings: [np.array, ...], image_paths: [...], created_at, ...}
        self.known_faces: Dict[str, Dict[str, Any]] = {}
        self.similarity_threshold = float(os.getenv('SIMILARITY_THRESHOLD', '0.5'))
        self.max_images_per_person = int(os.getenv('MAX_IMAGES_PER_PERSON', '10'))

        # Bounding box appearance configuration (RGB format)
        # Known faces: green, Unknown faces: red
        self.known_face_color: Tuple[int, int, int] = (0, 255, 0)  # RGB green
        self.unknown_face_color: Tuple[int, int, int] = (255, 0, 0)  # RGB red
        self.box_thickness: int = 2

        self.initialize_model()
        self.load_known_faces()

    def initialize_model(self):
        """Initialize InsightFace model"""
        try:
            # Check for GPU
            try:
                import onnxruntime as ort
                providers = ort.get_available_providers()
                if 'CUDAExecutionProvider' in providers:
                    self.device = 'cuda'
                    logger.info("GPU detected, using CUDA")
                elif 'ROCMExecutionProvider' in providers:
                    self.device = 'rocm'
                    logger.info("AMD GPU detected, using ROCm")
                else:
                    logger.info("No GPU detected, using CPU")
            except Exception as e:
                logger.warning(f"Could not check GPU availability: {e}")

            # Initialize InsightFace with buffalo_l model (good balance of speed/accuracy)
            model_name = os.getenv('INSIGHTFACE_MODEL', 'buffalo_l')
            logger.info(f"Loading InsightFace model: {model_name}")

            self.face_app = FaceAnalysis(
                name=model_name,
                providers=['CUDAExecutionProvider', 'CPUExecutionProvider']
            )

            # Prepare model - ctx_id=-1 for CPU, 0 for first GPU
            ctx_id = 0 if self.device in ['cuda', 'rocm'] else -1
            self.face_app.prepare(ctx_id=ctx_id, det_size=(640, 640))

            self.model_loaded = True
            logger.info(f"InsightFace model loaded successfully on {self.device}")

        except Exception as e:
            logger.error(f"Failed to initialize model: {e}")
            self.model_loaded = False

    def load_known_faces(self):
        """Load known faces database from disk and migrate old format if needed"""
        try:
            if os.path.exists(FACES_DB_PATH):
                with open(FACES_DB_PATH, 'rb') as f:
                    self.known_faces = pickle.load(f)

                # Migrate old single-embedding format to new multi-embedding format
                migrated = 0
                for name, data in self.known_faces.items():
                    if 'embedding' in data and 'embeddings' not in data:
                        # Old format: single embedding -> convert to list
                        data['embeddings'] = [data['embedding']]
                        del data['embedding']
                        # Also convert image_path to image_paths list
                        if 'image_path' in data:
                            data['image_paths'] = [data['image_path']]
                            del data['image_path']
                        migrated += 1

                if migrated > 0:
                    logger.info(f"Migrated {migrated} faces from single to multi-embedding format")
                    self.save_known_faces()

                logger.info(f"Loaded {len(self.known_faces)} known faces from database")
            else:
                logger.info("No faces database found, starting fresh")
                self.known_faces = {}
        except Exception as e:
            logger.error(f"Failed to load faces database: {e}")
            self.known_faces = {}

    def save_known_faces(self):
        """Save known faces database to disk"""
        try:
            # Ensure directory exists
            os.makedirs(os.path.dirname(FACES_DB_PATH), exist_ok=True)
            with open(FACES_DB_PATH, 'wb') as f:
                pickle.dump(self.known_faces, f)
            logger.info(f"Saved {len(self.known_faces)} faces to database")
        except Exception as e:
            logger.error(f"Failed to save faces database: {e}")
            raise

    def _hex_to_rgb(self, hex_color: str) -> Tuple[int, int, int]:
        """Convert hex color string to RGB tuple"""
        # Remove # if present
        hex_color = hex_color.lstrip('#')
        # Parse RGB values
        r = int(hex_color[0:2], 16)
        g = int(hex_color[2:4], 16)
        b = int(hex_color[4:6], 16)
        return (r, g, b)

    def set_known_face_color(self, hex_color: str):
        """Set color for known/recognized faces from hex string (e.g., '#00FF00')"""
        try:
            self.known_face_color = self._hex_to_rgb(hex_color)
            logger.info(f"Known face color set to {hex_color} -> RGB {self.known_face_color}")
        except Exception as e:
            logger.warning(f"Invalid color {hex_color}: {e}, keeping current color")

    def set_unknown_face_color(self, hex_color: str):
        """Set color for unknown faces from hex string (e.g., '#FF0000')"""
        try:
            self.unknown_face_color = self._hex_to_rgb(hex_color)
            logger.info(f"Unknown face color set to {hex_color} -> RGB {self.unknown_face_color}")
        except Exception as e:
            logger.warning(f"Invalid color {hex_color}: {e}, keeping current color")

    def set_box_thickness(self, thickness: int):
        """Set bounding box line thickness (1-5)"""
        self.box_thickness = max(1, min(5, thickness))
        logger.info(f"Box thickness set to {self.box_thickness}")

    def preprocess_image(self, image_data: bytes) -> np.ndarray:
        """Preprocess image for face detection"""
        try:
            # Convert bytes to PIL Image
            image = Image.open(io.BytesIO(image_data))

            # Convert to RGB if needed
            if image.mode != 'RGB':
                image = image.convert('RGB')

            # Convert to numpy array (RGB format for InsightFace)
            return np.array(image)

        except Exception as e:
            logger.error(f"Image preprocessing failed: {e}")
            raise HTTPException(status_code=400, detail="Invalid image format")

    def detect_faces(self, image_data: bytes) -> Dict[str, Any]:
        """Detect faces in image and return face information"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            image = self.preprocess_image(image_data)

            # Detect faces
            faces = self.face_app.get(image)

            detections = []
            for face in faces:
                bbox = face.bbox.tolist()  # [x1, y1, x2, y2]

                detection = {
                    "bbox": bbox,
                    "confidence": float(face.det_score),
                    "center": [
                        (bbox[0] + bbox[2]) / 2,
                        (bbox[1] + bbox[3]) / 2
                    ],
                    "area": (bbox[2] - bbox[0]) * (bbox[3] - bbox[1])
                }

                # Add landmarks if available
                if face.landmark_2d_106 is not None:
                    detection["landmarks_count"] = len(face.landmark_2d_106)

                # Add age/gender if available
                if hasattr(face, 'age') and face.age is not None:
                    detection["age"] = int(face.age)
                if hasattr(face, 'gender') and face.gender is not None:
                    detection["gender"] = "male" if face.gender == 1 else "female"

                detections.append(detection)

            inference_time = time.time() - start_time

            return {
                "faces": detections,
                "count": len(detections),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": self.device
            }

        except Exception as e:
            logger.error(f"Face detection failed: {e}")
            raise HTTPException(status_code=500, detail=f"Face detection failed: {str(e)}")

    def recognize_faces(self, image_data: bytes) -> Dict[str, Any]:
        """Detect faces and match against known identities"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            image = self.preprocess_image(image_data)

            # Detect faces with embeddings
            faces = self.face_app.get(image)

            recognitions = []
            for face in faces:
                bbox = face.bbox.tolist()

                recognition = {
                    "bbox": bbox,
                    "confidence": float(face.det_score),
                    "identity": None,
                    "similarity": 0.0,
                    "is_known": False
                }

                # Add age/gender if available
                if hasattr(face, 'age') and face.age is not None:
                    recognition["age"] = int(face.age)
                if hasattr(face, 'gender') and face.gender is not None:
                    recognition["gender"] = "male" if face.gender == 1 else "female"

                # Match against known faces
                if face.embedding is not None and len(self.known_faces) > 0:
                    best_match = None
                    best_similarity = 0.0

                    for name, data in self.known_faces.items():
                        # Compute similarity against all embeddings for this person
                        similarity = self._compute_similarity(face.embedding, data['embeddings'])
                        if similarity > best_similarity:
                            best_similarity = similarity
                            best_match = name

                    if best_similarity >= self.similarity_threshold:
                        recognition["identity"] = best_match
                        recognition["similarity"] = round(best_similarity, 3)
                        recognition["is_known"] = True

                recognitions.append(recognition)

            inference_time = time.time() - start_time

            # Summary
            known_count = sum(1 for r in recognitions if r["is_known"])
            unknown_count = len(recognitions) - known_count

            return {
                "recognitions": recognitions,
                "count": len(recognitions),
                "known_count": known_count,
                "unknown_count": unknown_count,
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": self.device,
                "similarity_threshold": self.similarity_threshold
            }

        except Exception as e:
            logger.error(f"Face recognition failed: {e}")
            raise HTTPException(status_code=500, detail=f"Face recognition failed: {str(e)}")

    def _compute_similarity_single(self, embedding1: np.ndarray, embedding2: np.ndarray) -> float:
        """Compute cosine similarity between two face embeddings"""
        # Normalize embeddings
        norm1 = np.linalg.norm(embedding1)
        norm2 = np.linalg.norm(embedding2)

        if norm1 == 0 or norm2 == 0:
            return 0.0

        # Cosine similarity
        similarity = np.dot(embedding1, embedding2) / (norm1 * norm2)
        return float(similarity)

    def _compute_similarity(self, query_embedding: np.ndarray, stored_embeddings: List[np.ndarray]) -> float:
        """
        Compute similarity between query embedding and multiple stored embeddings.
        Uses ensemble averaging: computes similarity to each stored embedding and returns the max.
        This allows matching against any of the registered images for a person.
        """
        if not stored_embeddings:
            return 0.0

        # Compute similarity against each stored embedding, take the max
        # This means: "match if the face looks like ANY of the registered images"
        similarities = [self._compute_similarity_single(query_embedding, stored) for stored in stored_embeddings]
        return max(similarities)

    def analyze_faces_forensic(self, image_data: bytes) -> Dict[str, Any]:
        """
        Forensic face analysis - returns cropped faces with landmarks overlay
        NSA-style biometric analysis view
        """
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            image = self.preprocess_image(image_data)
            faces = self.face_app.get(image)

            face_analyses = []

            for idx, face in enumerate(faces):
                bbox = face.bbox.astype(int)

                # Expand crop area for better context
                padding = 40
                h, w = image.shape[:2]
                x1 = max(0, bbox[0] - padding)
                y1 = max(0, bbox[1] - padding)
                x2 = min(w, bbox[2] + padding)
                y2 = min(h, bbox[3] + padding)

                # Crop face region
                face_crop = image[y1:y2, x1:x2].copy()

                # Offset for landmarks (since we cropped)
                offset_x = x1
                offset_y = y1

                # Draw landmarks if available
                if face.landmark_2d_106 is not None:
                    landmarks = face.landmark_2d_106

                    # InsightFace 106-point landmark indices:
                    # 0-32: face contour
                    # 33-37: right eyebrow
                    # 38-42: left eyebrow
                    # 43-47: nose bridge
                    # 48-51: nose bottom
                    # 52-71: right eye
                    # 72-91: left eye
                    # 92-95: upper lip top
                    # 96-100: lower lip bottom
                    # 101-103: upper lip bottom
                    # 104-105: lower lip top

                    # Draw all landmarks as small dots
                    for i, (lx, ly) in enumerate(landmarks):
                        px = int(lx - offset_x)
                        py = int(ly - offset_y)
                        if 0 <= px < face_crop.shape[1] and 0 <= py < face_crop.shape[0]:
                            # Color code by region
                            if i <= 32:  # Face contour - cyan
                                color = (0, 255, 255)
                            elif i <= 42:  # Eyebrows - yellow
                                color = (255, 255, 0)
                            elif i <= 51:  # Nose - magenta
                                color = (255, 0, 255)
                            elif i <= 91:  # Eyes - green
                                color = (0, 255, 0)
                            else:  # Mouth - red
                                color = (255, 100, 100)
                            cv2.circle(face_crop, (px, py), 2, color, -1)

                    # Draw connecting lines for face contour
                    contour_pts = [(int(landmarks[i][0] - offset_x), int(landmarks[i][1] - offset_y))
                                   for i in range(33) if 0 <= int(landmarks[i][0] - offset_x) < face_crop.shape[1]]
                    for i in range(len(contour_pts) - 1):
                        cv2.line(face_crop, contour_pts[i], contour_pts[i+1], (0, 255, 255), 1)

                    # Draw eye outlines
                    # Right eye: 52-71
                    right_eye_pts = [(int(landmarks[i][0] - offset_x), int(landmarks[i][1] - offset_y))
                                     for i in range(52, 72)]
                    if len(right_eye_pts) > 2:
                        cv2.polylines(face_crop, [np.array(right_eye_pts)], True, (0, 255, 0), 1)

                    # Left eye: 72-91
                    left_eye_pts = [(int(landmarks[i][0] - offset_x), int(landmarks[i][1] - offset_y))
                                    for i in range(72, 92)]
                    if len(left_eye_pts) > 2:
                        cv2.polylines(face_crop, [np.array(left_eye_pts)], True, (0, 255, 0), 1)

                # Add forensic overlay info
                overlay_height = 60
                forensic_view = np.zeros((face_crop.shape[0] + overlay_height, face_crop.shape[1], 3), dtype=np.uint8)
                forensic_view[overlay_height:, :] = face_crop

                # Dark header bar
                forensic_view[:overlay_height, :] = (20, 20, 30)

                # Add text info
                cv2.putText(forensic_view, f"SUBJECT #{idx+1}", (5, 15),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.4, (0, 255, 0), 1)
                cv2.putText(forensic_view, f"CONF: {face.det_score:.1%}", (5, 30),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.35, (200, 200, 200), 1)

                # Age/Gender if available
                info_y = 45
                if hasattr(face, 'age') and face.age is not None:
                    cv2.putText(forensic_view, f"AGE: {int(face.age)}", (5, info_y),
                               cv2.FONT_HERSHEY_SIMPLEX, 0.35, (200, 200, 200), 1)
                    info_y += 12
                if hasattr(face, 'gender') and face.gender is not None:
                    gender_str = "M" if face.gender == 1 else "F"
                    cv2.putText(forensic_view, f"SEX: {gender_str}", (60, 45),
                               cv2.FONT_HERSHEY_SIMPLEX, 0.35, (200, 200, 200), 1)

                # Recognition status
                identity = None
                similarity = 0.0
                is_known = False

                if face.embedding is not None and len(self.known_faces) > 0:
                    best_match = None
                    best_similarity = 0.0

                    for name, data in self.known_faces.items():
                        sim = self._compute_similarity(face.embedding, data['embeddings'])
                        if sim > best_similarity:
                            best_similarity = sim
                            best_match = name

                    if best_similarity >= self.similarity_threshold:
                        identity = best_match
                        similarity = best_similarity
                        is_known = True
                        # Green border for known
                        cv2.rectangle(forensic_view, (0, overlay_height),
                                     (forensic_view.shape[1]-1, forensic_view.shape[0]-1), (0, 255, 0), 2)
                        cv2.putText(forensic_view, f"ID: {identity}", (5, 57),
                                   cv2.FONT_HERSHEY_SIMPLEX, 0.35, (0, 255, 0), 1)
                    else:
                        # Red border for unknown
                        cv2.rectangle(forensic_view, (0, overlay_height),
                                     (forensic_view.shape[1]-1, forensic_view.shape[0]-1), (0, 0, 255), 2)
                        cv2.putText(forensic_view, "ID: UNKNOWN", (5, 57),
                                   cv2.FONT_HERSHEY_SIMPLEX, 0.35, (0, 0, 255), 1)
                else:
                    # Yellow border for no database
                    cv2.rectangle(forensic_view, (0, overlay_height),
                                 (forensic_view.shape[1]-1, forensic_view.shape[0]-1), (0, 255, 255), 2)

                # Convert to JPEG
                forensic_bgr = cv2.cvtColor(forensic_view, cv2.COLOR_RGB2BGR)
                _, jpeg_data = cv2.imencode('.jpg', forensic_bgr, [cv2.IMWRITE_JPEG_QUALITY, 95])

                face_analysis = {
                    "index": idx,
                    "bbox": bbox.tolist(),
                    "confidence": float(face.det_score),
                    "identity": identity,
                    "similarity": round(similarity, 3) if is_known else None,
                    "is_known": is_known,
                    "image_base64": base64.b64encode(jpeg_data.tobytes()).decode('utf-8'),
                    "has_landmarks": face.landmark_2d_106 is not None
                }

                if hasattr(face, 'age') and face.age is not None:
                    face_analysis["age"] = int(face.age)
                if hasattr(face, 'gender') and face.gender is not None:
                    face_analysis["gender"] = "male" if face.gender == 1 else "female"

                face_analyses.append(face_analysis)

            inference_time = time.time() - start_time

            return {
                "faces": face_analyses,
                "count": len(face_analyses),
                "known_count": sum(1 for f in face_analyses if f["is_known"]),
                "unknown_count": sum(1 for f in face_analyses if not f["is_known"]),
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": self.device
            }

        except Exception as e:
            logger.error(f"Forensic analysis failed: {e}")
            raise HTTPException(status_code=500, detail=f"Forensic analysis failed: {str(e)}")

    def register_face(self, name: str, image_data: bytes) -> Dict[str, Any]:
        """Register a new face identity"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        try:
            image = self.preprocess_image(image_data)

            # Detect faces
            faces = self.face_app.get(image)

            if len(faces) == 0:
                raise HTTPException(status_code=400, detail="No face detected in image")

            if len(faces) > 1:
                raise HTTPException(status_code=400, detail=f"Multiple faces detected ({len(faces)}). Please provide an image with a single face.")

            face = faces[0]

            if face.embedding is None:
                raise HTTPException(status_code=500, detail="Could not extract face embedding")

            # Save face image
            os.makedirs(FACES_IMAGES_PATH, exist_ok=True)
            image_filename = f"{name.replace(' ', '_')}_{int(time.time())}.jpg"
            image_path = os.path.join(FACES_IMAGES_PATH, image_filename)

            # Crop face from image
            bbox = face.bbox.astype(int)
            # Add padding
            padding = 20
            x1 = max(0, bbox[0] - padding)
            y1 = max(0, bbox[1] - padding)
            x2 = min(image.shape[1], bbox[2] + padding)
            y2 = min(image.shape[0], bbox[3] + padding)
            face_crop = image[y1:y2, x1:x2]

            # Save cropped face
            face_pil = Image.fromarray(face_crop)
            face_pil.save(image_path, 'JPEG', quality=95)

            # Store in database (new multi-embedding format)
            self.known_faces[name] = {
                'embeddings': [face.embedding],  # List of embeddings
                'image_paths': [image_path],      # List of image paths
                'created_at': datetime.now().isoformat(),
                'bbox': bbox.tolist(),
                'confidence': float(face.det_score),
                'image_count': 1
            }

            # Add age/gender if available
            if hasattr(face, 'age') and face.age is not None:
                self.known_faces[name]['age'] = int(face.age)
            if hasattr(face, 'gender') and face.gender is not None:
                self.known_faces[name]['gender'] = "male" if face.gender == 1 else "female"

            self.save_known_faces()

            return {
                "success": True,
                "name": name,
                "message": f"Face registered successfully for '{name}'",
                "face_count": len(self.known_faces),
                "image_count": 1,
                "image_paths": [image_path]
            }

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Face registration failed: {e}")
            raise HTTPException(status_code=500, detail=f"Face registration failed: {str(e)}")

    def add_face_image(self, name: str, image_data: bytes) -> Dict[str, Any]:
        """Add an additional image to an existing face identity for better recognition"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        if name not in self.known_faces:
            raise HTTPException(status_code=404, detail=f"Face '{name}' not found. Use /faces/register to create a new identity.")

        try:
            data = self.known_faces[name]
            current_count = len(data.get('embeddings', []))

            if current_count >= self.max_images_per_person:
                raise HTTPException(
                    status_code=400,
                    detail=f"Maximum images ({self.max_images_per_person}) reached for '{name}'. Delete some images first."
                )

            image = self.preprocess_image(image_data)

            # Detect faces
            faces = self.face_app.get(image)

            if len(faces) == 0:
                raise HTTPException(status_code=400, detail="No face detected in image")

            if len(faces) > 1:
                raise HTTPException(status_code=400, detail=f"Multiple faces detected ({len(faces)}). Please provide an image with a single face.")

            face = faces[0]

            if face.embedding is None:
                raise HTTPException(status_code=500, detail="Could not extract face embedding")

            # Save face image
            os.makedirs(FACES_IMAGES_PATH, exist_ok=True)
            image_filename = f"{name.replace(' ', '_')}_{int(time.time())}.jpg"
            image_path = os.path.join(FACES_IMAGES_PATH, image_filename)

            # Crop face from image
            bbox = face.bbox.astype(int)
            padding = 20
            x1 = max(0, bbox[0] - padding)
            y1 = max(0, bbox[1] - padding)
            x2 = min(image.shape[1], bbox[2] + padding)
            y2 = min(image.shape[0], bbox[3] + padding)
            face_crop = image[y1:y2, x1:x2]

            # Save cropped face
            face_pil = Image.fromarray(face_crop)
            face_pil.save(image_path, 'JPEG', quality=95)

            # Add to existing embeddings and image paths
            data['embeddings'].append(face.embedding)
            data['image_paths'].append(image_path)
            data['image_count'] = len(data['embeddings'])
            data['updated_at'] = datetime.now().isoformat()

            self.save_known_faces()

            logger.info(f"Added image #{data['image_count']} for '{name}'")

            return {
                "success": True,
                "name": name,
                "message": f"Image added successfully for '{name}'",
                "image_count": data['image_count'],
                "max_images": self.max_images_per_person,
                "image_path": image_path
            }

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Add face image failed: {e}")
            raise HTTPException(status_code=500, detail=f"Add face image failed: {str(e)}")

    def delete_face(self, name: str) -> Dict[str, Any]:
        """Delete a registered face and all its images"""
        if name not in self.known_faces:
            raise HTTPException(status_code=404, detail=f"Face '{name}' not found")

        try:
            data = self.known_faces[name]
            deleted_images = 0

            # Delete all image files (new format with image_paths list)
            if 'image_paths' in data:
                for image_path in data['image_paths']:
                    if os.path.exists(image_path):
                        os.remove(image_path)
                        deleted_images += 1
            # Also handle old format with single image_path
            elif 'image_path' in data and os.path.exists(data['image_path']):
                os.remove(data['image_path'])
                deleted_images = 1

            # Remove from database
            del self.known_faces[name]
            self.save_known_faces()

            return {
                "success": True,
                "name": name,
                "message": f"Face '{name}' deleted successfully ({deleted_images} images removed)",
                "face_count": len(self.known_faces),
                "deleted_images": deleted_images
            }

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Face deletion failed: {e}")
            raise HTTPException(status_code=500, detail=f"Face deletion failed: {str(e)}")

    def list_faces(self) -> Dict[str, Any]:
        """List all registered faces"""
        faces = []
        for name, data in self.known_faces.items():
            # Count images (handle both old and new format)
            if 'image_paths' in data:
                image_count = len(data['image_paths'])
                has_images = any(os.path.exists(p) for p in data['image_paths'])
            elif 'image_path' in data:
                image_count = 1
                has_images = os.path.exists(data.get('image_path', ''))
            else:
                image_count = 0
                has_images = False

            face_info = {
                "name": name,
                "created_at": data.get('created_at'),
                "updated_at": data.get('updated_at'),
                "image_count": image_count,
                "has_images": has_images,
            }
            if 'age' in data:
                face_info['age'] = data['age']
            if 'gender' in data:
                face_info['gender'] = data['gender']
            faces.append(face_info)

        return {
            "faces": faces,
            "count": len(faces),
            "max_images_per_person": self.max_images_per_person
        }

    def detect_and_annotate(self, image_data: bytes, recognize: bool = True) -> Tuple[bytes, Dict[str, Any]]:
        """Detect/recognize faces and return annotated image with configurable colors"""
        if not self.model_loaded:
            raise HTTPException(status_code=503, detail="Model not loaded")

        start_time = time.time()

        try:
            image = self.preprocess_image(image_data)

            # Detect faces
            faces = self.face_app.get(image)

            # Draw on image
            annotated = image.copy()
            recognitions = []

            for face in faces:
                bbox = face.bbox.astype(int)
                confidence = float(face.det_score)

                # Default: unknown face (use configurable color)
                color = self.unknown_face_color
                label = "Unknown"
                similarity = 0.0
                is_known = False

                # Try to recognize if requested
                if recognize and face.embedding is not None and len(self.known_faces) > 0:
                    best_match = None
                    best_similarity = 0.0

                    for name, data in self.known_faces.items():
                        sim = self._compute_similarity(face.embedding, data['embeddings'])
                        if sim > best_similarity:
                            best_similarity = sim
                            best_match = name

                    if best_similarity >= self.similarity_threshold:
                        label = best_match
                        similarity = best_similarity
                        is_known = True
                        color = self.known_face_color  # Use configurable known face color

                # Draw bounding box with configurable thickness
                cv2.rectangle(annotated, (bbox[0], bbox[1]), (bbox[2], bbox[3]), color, self.box_thickness)

                # Draw label background
                label_text = f"{label}"
                if is_known:
                    label_text += f" ({similarity:.0%})"

                (text_width, text_height), _ = cv2.getTextSize(label_text, cv2.FONT_HERSHEY_SIMPLEX, 0.6, 2)
                cv2.rectangle(annotated, (bbox[0], bbox[1] - text_height - 10),
                             (bbox[0] + text_width + 10, bbox[1]), color, -1)
                cv2.putText(annotated, label_text, (bbox[0] + 5, bbox[1] - 5),
                           cv2.FONT_HERSHEY_SIMPLEX, 0.6, (255, 255, 255), 2)

                recognition = {
                    "bbox": bbox.tolist(),
                    "confidence": confidence,
                    "identity": label if is_known else None,
                    "similarity": round(similarity, 3),
                    "is_known": is_known
                }

                if hasattr(face, 'age') and face.age is not None:
                    recognition["age"] = int(face.age)
                if hasattr(face, 'gender') and face.gender is not None:
                    recognition["gender"] = "male" if face.gender == 1 else "female"

                recognitions.append(recognition)

            inference_time = time.time() - start_time

            # Convert RGB to BGR for OpenCV encoding
            annotated_bgr = cv2.cvtColor(annotated, cv2.COLOR_RGB2BGR)
            _, jpeg_data = cv2.imencode('.jpg', annotated_bgr, [cv2.IMWRITE_JPEG_QUALITY, 90])

            known_count = sum(1 for r in recognitions if r["is_known"])

            result_info = {
                "recognitions": recognitions,
                "count": len(recognitions),
                "known_count": known_count,
                "unknown_count": len(recognitions) - known_count,
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": self.device
            }

            return jpeg_data.tobytes(), result_info

        except Exception as e:
            logger.error(f"Annotated detection failed: {e}")
            raise HTTPException(status_code=500, detail=f"Detection failed: {str(e)}")


# Global service instance
recognition_service = FaceRecognitionService()


@app.get("/")
async def root():
    """Root endpoint with service info"""
    return {
        "service": "Orbo Face Recognition Service",
        "version": "1.0.0",
        "device": recognition_service.device,
        "model_loaded": recognition_service.model_loaded,
        "known_faces_count": len(recognition_service.known_faces),
        "similarity_threshold": recognition_service.similarity_threshold
    }


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {
        "status": "healthy" if recognition_service.model_loaded else "unhealthy",
        "device": recognition_service.device,
        "model_loaded": recognition_service.model_loaded,
        "known_faces_count": len(recognition_service.known_faces)
    }


@app.post("/detect")
async def detect_faces(file: UploadFile = File(...)):
    """
    Detect faces in uploaded image

    Returns face locations, confidence scores, and optional age/gender estimation
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        return recognition_service.detect_faces(image_data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.post("/recognize")
async def recognize_faces(file: UploadFile = File(...)):
    """
    Detect faces and match against known identities

    Returns face locations with identity matches and similarity scores
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        return recognition_service.recognize_faces(image_data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.post("/recognize/annotated")
async def recognize_annotated(
    file: UploadFile = File(...),
    format: str = Query("image", description="Response format: 'image' for raw JPEG, 'base64' for JSON")
):
    """
    Detect and recognize faces, return annotated image

    Green boxes = known faces, Red boxes = unknown faces
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        annotated_image, result_info = recognition_service.detect_and_annotate(image_data, recognize=True)

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
                    "X-Face-Count": str(result_info["count"]),
                    "X-Known-Count": str(result_info["known_count"]),
                    "X-Unknown-Count": str(result_info["unknown_count"]),
                    "X-Inference-Time-Ms": str(result_info["inference_time_ms"])
                }
            )
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.post("/analyze/forensic")
async def analyze_forensic(file: UploadFile = File(...)):
    """
    NSA-style forensic face analysis with landmarks overlay

    Returns cropped face images with:
    - 106-point facial landmarks color-coded by region
    - Dark header with forensic info (SUBJECT #, CONF, AGE, SEX)
    - Color-coded borders: green=known, red=unknown, yellow=no database
    - Recognition status and identity if matched
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        return recognition_service.analyze_faces_forensic(image_data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.post("/faces/register")
async def register_face(
    name: str = Form(..., description="Name/identity to associate with this face"),
    file: UploadFile = File(...)
):
    """
    Register a new face identity

    The image should contain exactly one face. The face will be associated
    with the provided name and can be recognized in future images.
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    # Validate name
    name = name.strip()
    if not name:
        raise HTTPException(status_code=400, detail="Name cannot be empty")
    if len(name) > 100:
        raise HTTPException(status_code=400, detail="Name too long (max 100 characters)")

    try:
        image_data = await file.read()
        return recognition_service.register_face(name, image_data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.post("/faces/{name}/add-image")
async def add_face_image(
    name: str,
    file: UploadFile = File(...)
):
    """
    Add an additional image to an existing face identity.

    Multiple images per person improve recognition accuracy, especially for:
    - Different angles (front, profile, 3/4 view)
    - Different lighting conditions
    - Different expressions
    - With/without glasses, hats, etc.

    Maximum images per person is configurable (default: 10).
    """
    if not file.content_type.startswith('image/'):
        raise HTTPException(status_code=400, detail="File must be an image")

    try:
        image_data = await file.read()
        return recognition_service.add_face_image(name, image_data)
    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Unexpected error: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@app.delete("/faces/{name}")
async def delete_face(name: str):
    """Delete a registered face identity and all its images"""
    return recognition_service.delete_face(name)


@app.get("/faces")
async def list_faces():
    """List all registered face identities with image counts"""
    return recognition_service.list_faces()


@app.get("/faces/{name}/images")
async def get_face_images(name: str):
    """Get all registered images for a face identity"""
    if name not in recognition_service.known_faces:
        raise HTTPException(status_code=404, detail=f"Face '{name}' not found")

    data = recognition_service.known_faces[name]

    # Handle both old and new format
    if 'image_paths' in data:
        images = []
        for idx, image_path in enumerate(data['image_paths']):
            if os.path.exists(image_path):
                images.append({
                    "index": idx,
                    "path": image_path,
                    "exists": True
                })
            else:
                images.append({
                    "index": idx,
                    "path": image_path,
                    "exists": False
                })
        return {
            "name": name,
            "images": images,
            "count": len(images)
        }
    elif 'image_path' in data:
        # Old format
        exists = os.path.exists(data['image_path'])
        return {
            "name": name,
            "images": [{"index": 0, "path": data['image_path'], "exists": exists}],
            "count": 1 if exists else 0
        }
    else:
        return {
            "name": name,
            "images": [],
            "count": 0
        }


@app.get("/faces/{name}/image")
async def get_face_image(name: str, index: int = Query(0, description="Image index (0-based)")):
    """Get a specific registered image for a face identity by index"""
    if name not in recognition_service.known_faces:
        raise HTTPException(status_code=404, detail=f"Face '{name}' not found")

    data = recognition_service.known_faces[name]

    # Handle new format with multiple images
    if 'image_paths' in data:
        if index < 0 or index >= len(data['image_paths']):
            raise HTTPException(status_code=404, detail=f"Image index {index} not found. Available: 0-{len(data['image_paths'])-1}")
        image_path = data['image_paths'][index]
    elif 'image_path' in data:
        # Old format with single image
        if index != 0:
            raise HTTPException(status_code=404, detail="Only index 0 available for this face")
        image_path = data['image_path']
    else:
        raise HTTPException(status_code=404, detail="No images found")

    if not os.path.exists(image_path):
        raise HTTPException(status_code=404, detail="Image file not found")

    with open(image_path, 'rb') as f:
        return Response(content=f.read(), media_type="image/jpeg")


@app.post("/config/threshold")
async def set_threshold(threshold: float = Query(..., ge=0.0, le=1.0)):
    """Set the similarity threshold for face recognition"""
    recognition_service.similarity_threshold = threshold
    return {
        "success": True,
        "similarity_threshold": threshold
    }


@app.get("/config")
async def get_config():
    """Get current configuration"""
    return {
        "similarity_threshold": recognition_service.similarity_threshold,
        "faces_db_path": FACES_DB_PATH,
        "faces_images_path": FACES_IMAGES_PATH,
        "known_faces_count": len(recognition_service.known_faces)
    }


def start_grpc_server():
    """Start gRPC server in a background thread"""
    grpc_port = int(os.getenv('GRPC_PORT', '50052'))
    if grpc_port <= 0:
        logger.info("gRPC server disabled (GRPC_PORT not set or 0)")
        return None

    try:
        from grpc_server import serve as grpc_serve
        grpc_server = grpc_serve(recognition_service, port=grpc_port)
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
    port = int(os.getenv('PORT', 8082))
    host = os.getenv('HOST', '0.0.0.0')

    logger.info(f"Starting Face Recognition service - HTTP on {host}:{port}")

    try:
        uvicorn.run(
            "main:app",
            host=host,
            port=port,
            reload=False,
            workers=1  # Single worker for model efficiency
        )
    finally:
        if grpc_server:
            logger.info("Stopping gRPC server...")
            grpc_server.stop(grace=5)
