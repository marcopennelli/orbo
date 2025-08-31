#!/usr/bin/env python3
"""
Simple HTTP test server without dependencies
"""

import http.server
import socketserver
import json
import random
from urllib.parse import urlparse, parse_qs

class YOLOTestHandler(http.server.BaseHTTPRequestHandler):
    
    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {
                "status": "healthy",
                "device": "cpu",
                "gpu_available": False,
                "model_loaded": True
            }
            self.wfile.write(json.dumps(response).encode())
        else:
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {
                "service": "Test YOLOv8 Detection Service",
                "version": "1.0.0",
                "device": "cpu",
                "model_loaded": True,
                "gpu_available": False
            }
            self.wfile.write(json.dumps(response).encode())
    
    def do_POST(self):
        if self.path.startswith('/detect/security'):
            # Mock security detection
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            
            # Mock detection results
            detections = []
            if random.random() > 0.4:  # 60% chance of detection
                detection = {
                    "class": "person",
                    "class_id": 0,
                    "confidence": 0.87,
                    "bbox": [150, 100, 350, 400],
                    "center": [250, 250],
                    "area": 60000.0
                }
                detections.append(detection)
            
            response = {
                "detections": detections,
                "count": len(detections),
                "threat_analysis": {
                    "high_priority": detections if detections else [],
                    "medium_priority": [],
                    "low_priority": []
                },
                "inference_time_ms": 52.3,
                "device": "cpu"
            }
            self.wfile.write(json.dumps(response).encode())
        else:
            self.send_response(404)
            self.end_headers()

if __name__ == "__main__":
    PORT = 8081
    Handler = YOLOTestHandler
    with socketserver.TCPServer(("", PORT), Handler) as httpd:
        print(f"Test YOLOv8 service running on port {PORT}")
        httpd.serve_forever()