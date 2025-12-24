#!/usr/bin/env python3
"""
Download DINOv3 model for offline usage
"""

import torch

def download_model():
    """Download and cache the DINOv2 model"""
    model_name = "dinov2_vits14"
    
    print(f"Downloading model: {model_name}")
    print("This may take a few minutes...")
    
    try:
        # Download model using PyTorch Hub
        print("Downloading DINOv2 model weights...")
        model = torch.hub.load('facebookresearch/dinov2', model_name)
        
        print("✓ Model downloaded successfully!")
        print(f"Cached location: ~/.cache/torch/hub/")
        
        return True
        
    except Exception as e:
        print(f"❌ Failed to download model: {e}")
        return False

if __name__ == "__main__":
    success = download_model()
    exit(0 if success else 1)