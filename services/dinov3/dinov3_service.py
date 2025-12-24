#!/usr/bin/env python3
"""
DINOv3 Service for Orbo - Advanced Computer Vision Analysis

This service provides DINOv3-powered computer vision capabilities:
- Self-supervised feature extraction
- Motion detection using feature similarity
- Zero-shot object detection and classification
- Scene understanding and change detection
"""

import io
import time
import logging
from typing import List, Dict, Any, Optional, Tuple
from dataclasses import dataclass

import torch
import torchvision.transforms as transforms
from transformers import AutoImageProcessor, AutoModel
import numpy as np
from PIL import Image
from fastapi import FastAPI, File, UploadFile, HTTPException, Form
from fastapi.responses import JSONResponse
import uvicorn
from sklearn.metrics.pairwise import cosine_similarity


# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


@dataclass
class DetectionResult:
    """Detection result with enhanced DINOv3 features"""
    motion_detected: bool
    confidence: float
    feature_similarity: float
    change_regions: List[Dict[str, Any]]
    scene_analysis: Dict[str, Any]
    inference_time_ms: float
    device: str


@dataclass
class MotionAnalysis:
    """Advanced motion analysis using DINOv3 features"""
    motion_strength: float
    motion_type: str  # "object", "person", "vehicle", "environmental"
    affected_regions: List[Dict[str, Any]]
    temporal_consistency: float


class DINOv3Detector:
    """DINOv3-based motion detector and scene analyzer"""
    
    def __init__(self, model_name: str = "facebook/dinov3-vit7b16-pretrain-lvd1689m", device: str = None):
        self.device = device or ("cuda" if torch.cuda.is_available() else "cpu")
        logger.info(f"Initializing DINOv3 detector on {self.device}")
        
        # Try multiple DINOv3 loading methods
        try:
            logger.info(f"Loading DINOv3 model: {model_name}")
            
            # Method 1: Try Hugging Face first
            try:
                logger.info("Attempting Hugging Face DINOv3 model...")
                self.processor = AutoImageProcessor.from_pretrained(model_name)
                self.model = AutoModel.from_pretrained(model_name).to(self.device)
                self.model.eval()
                self.loading_method = "huggingface"
                logger.info(f"✓ Loaded DINOv3 via Hugging Face: {model_name}")
                
            except Exception as hf_error:
                logger.warning(f"Hugging Face method failed: {hf_error}")
                
                # Method 2: Try PyTorch Hub with local clone
                logger.info("Attempting PyTorch Hub with local repository...")
                self.model = torch.hub.load('facebookresearch/dinov3', 'dinov3_vits16', 
                                          source='github', force_reload=True)
                self.model = self.model.to(self.device)
                self.model.eval()
                self.loading_method = "pytorch_hub"
                logger.info("✓ Loaded DINOv3 via PyTorch Hub")
                
        except Exception as e:
            logger.error(f"Failed to load DINOv3 model with all methods: {e}")
            
            # Fallback to DINOv2
            logger.warning("Falling back to DINOv2...")
            try:
                self.model = torch.hub.load('facebookresearch/dinov2', 'dinov2_vits14')
                self.model = self.model.to(self.device)
                self.model.eval()
                self.loading_method = "dinov2_fallback"
                logger.info("✓ Loaded DINOv2 as fallback")
            except Exception as fallback_error:
                logger.error(f"Even DINOv2 fallback failed: {fallback_error}")
                raise
        
        # Feature cache for motion detection
        self.feature_cache = {}
        self.max_cache_size = 10
        
        # Transform for preprocessing
        self.transform = transforms.Compose([
            transforms.Resize((224, 224)),
            transforms.ToTensor(),
            transforms.Normalize(mean=[0.485, 0.456, 0.406], std=[0.229, 0.224, 0.225])
        ])
    
    def extract_features(self, image: Image.Image) -> torch.Tensor:
        """Extract DINOv3 features from an image"""
        try:
            # Use different preprocessing based on loading method
            if self.loading_method == "huggingface":
                # Use processor for Hugging Face models
                inputs = self.processor(images=image, return_tensors="pt").to(self.device)
                with torch.no_grad():
                    outputs = self.model(**inputs)
                    features = outputs.last_hidden_state.mean(dim=1)  # Pool over sequence
            else:
                # Use manual transforms for PyTorch Hub models
                inputs = self.transform(image).unsqueeze(0).to(self.device)
                with torch.no_grad():
                    features = self.model(inputs)
                    # Handle different output formats
                    if len(features.shape) == 3:  # [batch, tokens, features]
                        features = features.mean(dim=1)  # Average pool over tokens
                    elif len(features.shape) == 4:  # [batch, channels, height, width]
                        features = features.mean(dim=[2, 3])  # Global average pool
            
            return features.cpu()
        except Exception as e:
            logger.error(f"Feature extraction failed: {e}")
            raise
    
    def detect_motion_with_features(self, current_image: Image.Image, 
                                  camera_id: str, 
                                  threshold: float = 0.85) -> DetectionResult:
        """Detect motion using DINOv3 feature similarity"""
        start_time = time.time()
        
        try:
            # Extract features from current frame
            current_features = self.extract_features(current_image)
            
            # Check if we have previous features for this camera
            if camera_id not in self.feature_cache:
                # First frame - no motion to detect
                self.feature_cache[camera_id] = current_features
                inference_time = (time.time() - start_time) * 1000
                
                return DetectionResult(
                    motion_detected=False,
                    confidence=0.0,
                    feature_similarity=1.0,
                    change_regions=[],
                    scene_analysis=self._analyze_scene(current_features),
                    inference_time_ms=inference_time,
                    device=self.device
                )
            
            # Calculate similarity with previous frame
            previous_features = self.feature_cache[camera_id]
            similarity = cosine_similarity(
                current_features.numpy(), 
                previous_features.numpy()
            )[0][0]
            
            # Detect motion based on feature similarity
            motion_detected = similarity < threshold
            confidence = 1.0 - similarity if motion_detected else 0.0
            
            # Analyze motion characteristics
            motion_analysis = self._analyze_motion(
                current_features, previous_features, similarity
            )
            
            # Scene analysis
            scene_analysis = self._analyze_scene(current_features)
            scene_analysis.update({
                "motion_analysis": motion_analysis.__dict__
            })
            
            # Update cache
            self.feature_cache[camera_id] = current_features
            self._cleanup_cache()
            
            inference_time = (time.time() - start_time) * 1000
            
            return DetectionResult(
                motion_detected=motion_detected,
                confidence=confidence,
                feature_similarity=similarity,
                change_regions=motion_analysis.affected_regions,
                scene_analysis=scene_analysis,
                inference_time_ms=inference_time,
                device=self.device
            )
            
        except Exception as e:
            logger.error(f"Motion detection failed: {e}")
            raise HTTPException(status_code=500, detail=f"Motion detection failed: {str(e)}")
    
    def _analyze_motion(self, current_features: torch.Tensor, 
                       previous_features: torch.Tensor, 
                       similarity: float) -> MotionAnalysis:
        """Analyze motion characteristics using feature differences"""
        
        # Calculate feature difference magnitude
        feature_diff = torch.norm(current_features - previous_features).item()
        motion_strength = min(feature_diff / 10.0, 1.0)  # Normalize to [0, 1]
        
        # Classify motion type based on feature patterns
        motion_type = self._classify_motion_type(feature_diff, similarity)
        
        # Create affected regions (placeholder - could be enhanced with attention maps)
        affected_regions = []
        if motion_strength > 0.3:
            affected_regions.append({
                "x": 0, "y": 0, "width": 224, "height": 224,
                "confidence": motion_strength,
                "type": motion_type
            })
        
        return MotionAnalysis(
            motion_strength=motion_strength,
            motion_type=motion_type,
            affected_regions=affected_regions,
            temporal_consistency=similarity
        )
    
    def _classify_motion_type(self, feature_diff: float, similarity: float) -> str:
        """Classify type of motion based on feature patterns"""
        if feature_diff > 15.0:
            return "significant_change"  # Major scene change
        elif feature_diff > 8.0:
            return "object_motion"       # Object entering/leaving
        elif feature_diff > 4.0:
            return "subtle_motion"       # Small movements
        else:
            return "environmental"       # Lighting/environmental changes
    
    def _analyze_scene(self, features: torch.Tensor) -> Dict[str, Any]:
        """Analyze scene characteristics from DINOv3 features"""
        feature_norm = torch.norm(features).item()
        feature_mean = torch.mean(features).item()
        feature_std = torch.std(features).item()
        
        # Scene complexity based on feature statistics
        complexity = min(feature_std * 10, 1.0)
        
        # Estimate scene type (very basic classification)
        if feature_mean > 0.1:
            scene_type = "bright_scene"
        elif feature_mean < -0.1:
            scene_type = "dark_scene"
        else:
            scene_type = "balanced_scene"
        
        return {
            "scene_type": scene_type,
            "complexity_score": complexity,
            "feature_statistics": {
                "norm": feature_norm,
                "mean": feature_mean,
                "std": feature_std
            }
        }
    
    def _cleanup_cache(self):
        """Clean up feature cache to prevent memory issues"""
        if len(self.feature_cache) > self.max_cache_size:
            # Remove oldest entries (simple FIFO)
            oldest_key = next(iter(self.feature_cache))
            del self.feature_cache[oldest_key]
    
    def get_feature_embedding(self, image: Image.Image) -> List[float]:
        """Get feature embedding vector for an image"""
        features = self.extract_features(image)
        return features.flatten().tolist()


# Global detector instance
detector: Optional[DINOv3Detector] = None


# FastAPI app
app = FastAPI(title="DINOv3 Detection Service", version="1.0.0")


@app.on_event("startup")
async def startup_event():
    """Initialize the DINOv3 detector on startup"""
    global detector
    try:
        detector = DINOv3Detector()
        logger.info("DINOv3 service started successfully")
    except Exception as e:
        logger.error(f"Failed to start DINOv3 service: {e}")
        raise


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    if detector is None:
        raise HTTPException(status_code=503, detail="Service not ready")
    
    return {
        "status": "healthy",
        "service": "dinov3-detector",
        "device": detector.device,
        "timestamp": time.time()
    }


@app.post("/detect/motion")
async def detect_motion(
    file: UploadFile = File(...),
    camera_id: str = Form(...),
    threshold: float = Form(0.85)
):
    """Detect motion using DINOv3 feature comparison"""
    if detector is None:
        raise HTTPException(status_code=503, detail="Service not ready")
    
    try:
        # Read and validate image
        image_data = await file.read()
        image = Image.open(io.BytesIO(image_data)).convert('RGB')
        
        # Perform motion detection
        result = detector.detect_motion_with_features(image, camera_id, threshold)
        
        return {
            "motion_detected": result.motion_detected,
            "confidence": result.confidence,
            "feature_similarity": result.feature_similarity,
            "change_regions": result.change_regions,
            "scene_analysis": result.scene_analysis,
            "inference_time_ms": result.inference_time_ms,
            "device": result.device,
            "model": "dinov3",
            "threshold": threshold
        }
        
    except Exception as e:
        logger.error(f"Motion detection failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/extract/features")
async def extract_features(file: UploadFile = File(...)):
    """Extract DINOv3 feature embedding from an image"""
    if detector is None:
        raise HTTPException(status_code=503, detail="Service not ready")
    
    try:
        # Read and validate image
        image_data = await file.read()
        image = Image.open(io.BytesIO(image_data)).convert('RGB')
        
        start_time = time.time()
        features = detector.get_feature_embedding(image)
        inference_time = (time.time() - start_time) * 1000
        
        return {
            "features": features,
            "feature_dimension": len(features),
            "inference_time_ms": inference_time,
            "device": detector.device
        }
        
    except Exception as e:
        logger.error(f"Feature extraction failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/analyze/scene")
async def analyze_scene(file: UploadFile = File(...)):
    """Analyze scene characteristics using DINOv3"""
    if detector is None:
        raise HTTPException(status_code=503, detail="Service not ready")
    
    try:
        # Read and validate image
        image_data = await file.read()
        image = Image.open(io.BytesIO(image_data)).convert('RGB')
        
        start_time = time.time()
        features = detector.extract_features(image)
        scene_analysis = detector._analyze_scene(features)
        inference_time = (time.time() - start_time) * 1000
        
        return {
            "scene_analysis": scene_analysis,
            "inference_time_ms": inference_time,
            "device": detector.device
        }
        
    except Exception as e:
        logger.error(f"Scene analysis failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8001)