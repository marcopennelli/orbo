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

# Copy proto file from parent context (must be passed via build arg or copied beforehand)
# Proto generation happens at build time
COPY detection.proto /app/proto/
RUN python3 -m grpc_tools.protoc \
    -I/app/proto \
    --python_out=/app \
    --grpc_python_out=/app \
    /app/proto/detection.proto

# Copy application code
COPY main.py .
COPY grpc_server.py .

# Create directories for YOLO models and runtime (runs, settings)
RUN mkdir -p /app/models /app/runs /tmp/Ultralytics && chmod 777 /app/runs

# Set Ultralytics config and runs directory (writable at runtime)
ENV YOLO_CONFIG_DIR=/tmp/Ultralytics
ENV ULTRALYTICS_SETTINGS_JSON='{"runs_dir": "/app/runs"}'

# Download YOLO11 model weights for all enabled tasks (nano for best speed)
# detect: yolo11n.pt, segment: yolo11n-seg.pt, pose: yolo11n-pose.pt
RUN python3 -c "from ultralytics import YOLO; YOLO('yolo11n.pt'); YOLO('yolo11n-seg.pt'); YOLO('yolo11n-pose.pt')"

# Expose ports (HTTP and gRPC)
EXPOSE 8081 50051

# Health check (HTTP endpoint)
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python3 -c "import requests; requests.get('http://localhost:8081/health')" || exit 1

# Run the application
CMD ["python3", "main.py"]