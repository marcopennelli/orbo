FROM docker.io/rocm/pytorch:rocm6.0_ubuntu20.04_py3.9_pytorch_2.1.1

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    python3-pip \
    libgl1-mesa-glx \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    libgomp1 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Set ROCm environment variables
ENV HSA_OVERRIDE_GFX_VERSION=10.3.0
ENV PYTORCH_ROCM_ARCH="gfx1030;gfx1031;gfx1032;gfx1100;gfx1101;gfx1102"

# Copy requirements and install Python dependencies
COPY requirements.txt .

# Install PyTorch with ROCm support first
RUN pip3 install torch==2.1.0+rocm5.6 torchvision==0.16.0+rocm5.6 \
    --index-url https://download.pytorch.org/whl/rocm5.6

# Install other requirements
RUN pip3 install -r requirements.txt

# Copy application code
COPY main.py .

# Create directory for YOLOv8 models
RUN mkdir -p /app/models

# Download YOLOv8 model weights
RUN python3 -c "from ultralytics import YOLO; YOLO('yolov8n.pt')"

# Expose port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python3 -c "import requests; requests.get('http://localhost:8081/health')" || exit 1

# Run the application
CMD ["python3", "main.py"]