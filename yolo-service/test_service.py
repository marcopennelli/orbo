#!/usr/bin/env python3
"""
Simple CPU-based YOLOv8 test service
For testing integration without GPU requirements
"""

from fastapi import FastAPI, File, UploadFile, HTTPException
import time
import random

app = FastAPI(title="Test YOLOv8 Service")

# Mock detection classes for testing
MOCK_CLASSES = ["person", "car", "bicycle", "dog", "cat"]

@app.get("/")
async def root():
    return {
        "service": "Test YOLOv8 Detection Service", 
        "version": "1.0.0",
        "device": "cpu",
        "model_loaded": True,
        "gpu_available": False
    }

@app.get("/health")
async def health():
    return {
        "status": "healthy",
        "device": "cpu", 
        "gpu_available": False,
        "model_loaded": True
    }

@app.post("/detect/security")
async def detect_security(file: UploadFile = File(...), conf_threshold: float = 0.6):
    """Mock security detection for testing"""
    
    # Simulate processing time
    await asyncio.sleep(0.05)  # 50ms mock inference
    
    # Mock detection results
    detections = []
    if random.random() > 0.3:  # 70% chance of detection
        num_detections = random.randint(1, 2)
        for i in range(num_detections):
            class_name = random.choice(MOCK_CLASSES)
            detection = {
                "class": class_name,
                "class_id": MOCK_CLASSES.index(class_name),
                "confidence": random.uniform(0.6, 0.95),
                "bbox": [
                    random.randint(50, 200),   # x1
                    random.randint(50, 200),   # y1  
                    random.randint(300, 500),  # x2
                    random.randint(300, 400)   # y2
                ],
                "center": [320, 240],
                "area": 15000.0
            }
            detections.append(detection)
    
    # Categorize by threat level
    threat_analysis = {
        "high_priority": [d for d in detections if d["class"] == "person"],
        "medium_priority": [d for d in detections if d["class"] in ["car"]],
        "low_priority": [d for d in detections if d["class"] in ["bicycle"]]
    }
    
    return {
        "detections": detections,
        "count": len(detections),
        "threat_analysis": threat_analysis,
        "inference_time_ms": round(random.uniform(45, 65), 2),
        "device": "cpu",
        "security_filter": ["person", "car", "bicycle"]
    }

if __name__ == "__main__":
    import uvicorn
    import asyncio
    
    print("Starting test YOLOv8 service on http://0.0.0.0:8081")
    uvicorn.run(app, host="0.0.0.0", port=8081)