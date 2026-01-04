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
        self.known_faces: Dict[str, Dict[str, Any]] = {}  # name -> {embedding, image_path, created_at}
        self.similarity_threshold = float(os.getenv('SIMILARITY_THRESHOLD', '0.5'))
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
        """Load known faces database from disk"""
        try:
            if os.path.exists(FACES_DB_PATH):
                with open(FACES_DB_PATH, 'rb') as f:
                    self.known_faces = pickle.load(f)
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
                        # Compute cosine similarity
                        similarity = self._compute_similarity(face.embedding, data['embedding'])
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

    def _compute_similarity(self, embedding1: np.ndarray, embedding2: np.ndarray) -> float:
        """Compute cosine similarity between two face embeddings"""
        # Normalize embeddings
        norm1 = np.linalg.norm(embedding1)
        norm2 = np.linalg.norm(embedding2)

        if norm1 == 0 or norm2 == 0:
            return 0.0

        # Cosine similarity
        similarity = np.dot(embedding1, embedding2) / (norm1 * norm2)
        return float(similarity)

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

            # Store in database
            self.known_faces[name] = {
                'embedding': face.embedding,
                'image_path': image_path,
                'created_at': datetime.now().isoformat(),
                'bbox': bbox.tolist(),
                'confidence': float(face.det_score)
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
                "image_path": image_path
            }

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Face registration failed: {e}")
            raise HTTPException(status_code=500, detail=f"Face registration failed: {str(e)}")

    def delete_face(self, name: str) -> Dict[str, Any]:
        """Delete a registered face"""
        if name not in self.known_faces:
            raise HTTPException(status_code=404, detail=f"Face '{name}' not found")

        try:
            # Delete image file if exists
            data = self.known_faces[name]
            if 'image_path' in data and os.path.exists(data['image_path']):
                os.remove(data['image_path'])

            # Remove from database
            del self.known_faces[name]
            self.save_known_faces()

            return {
                "success": True,
                "name": name,
                "message": f"Face '{name}' deleted successfully",
                "face_count": len(self.known_faces)
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
            face_info = {
                "name": name,
                "created_at": data.get('created_at'),
                "has_image": 'image_path' in data and os.path.exists(data.get('image_path', '')),
            }
            if 'age' in data:
                face_info['age'] = data['age']
            if 'gender' in data:
                face_info['gender'] = data['gender']
            faces.append(face_info)

        return {
            "faces": faces,
            "count": len(faces)
        }

    def detect_and_annotate(self, image_data: bytes, recognize: bool = True) -> Tuple[bytes, Dict[str, Any]]:
        """Detect/recognize faces and return annotated image"""
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

                # Default: unknown face (red box)
                color = (255, 0, 0)  # RGB - red
                label = "Unknown"
                similarity = 0.0
                is_known = False

                # Try to recognize if requested
                if recognize and face.embedding is not None and len(self.known_faces) > 0:
                    best_match = None
                    best_similarity = 0.0

                    for name, data in self.known_faces.items():
                        sim = self._compute_similarity(face.embedding, data['embedding'])
                        if sim > best_similarity:
                            best_similarity = sim
                            best_match = name

                    if best_similarity >= self.similarity_threshold:
                        label = best_match
                        similarity = best_similarity
                        is_known = True
                        color = (0, 255, 0)  # RGB - green for known

                # Draw bounding box
                cv2.rectangle(annotated, (bbox[0], bbox[1]), (bbox[2], bbox[3]), color, 2)

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


@app.delete("/faces/{name}")
async def delete_face(name: str):
    """Delete a registered face identity"""
    return recognition_service.delete_face(name)


@app.get("/faces")
async def list_faces():
    """List all registered face identities"""
    return recognition_service.list_faces()


@app.get("/faces/{name}/image")
async def get_face_image(name: str):
    """Get the registered image for a face identity"""
    if name not in recognition_service.known_faces:
        raise HTTPException(status_code=404, detail=f"Face '{name}' not found")

    data = recognition_service.known_faces[name]
    if 'image_path' not in data or not os.path.exists(data['image_path']):
        raise HTTPException(status_code=404, detail="Image not found")

    with open(data['image_path'], 'rb') as f:
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


if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv('PORT', 8082))
    host = os.getenv('HOST', '0.0.0.0')

    logger.info(f"Starting Face Recognition service on {host}:{port}")
    uvicorn.run(
        "main:app",
        host=host,
        port=port,
        reload=False,
        workers=1  # Single worker for model efficiency
    )
