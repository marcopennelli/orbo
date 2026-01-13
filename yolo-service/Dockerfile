# =============================================================================
# YOLO Service - CUDA GPU Build (Optimized for GHCR)
# =============================================================================
# Uses NVIDIA CUDA runtime image (smaller than dev image)
# Layers split to stay under GHCR's 10GB per-layer limit
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Build stage - compile proto files
# -----------------------------------------------------------------------------
FROM python:3.11-slim AS builder

WORKDIR /build

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir grpcio-tools

COPY detection.proto /build/proto/
RUN python -m grpc_tools.protoc \
    -I/build/proto \
    --python_out=/build \
    --grpc_python_out=/build \
    /build/proto/detection.proto

# -----------------------------------------------------------------------------
# Stage 2: Runtime stage - CUDA runtime (smaller than devel)
# -----------------------------------------------------------------------------
FROM nvidia/cuda:12.1.0-runtime-ubuntu22.04

WORKDIR /app

# Install Python and system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-dev \
    libgl1-mesa-glx \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender1 \
    libgomp1 \
    wget \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/python3 /usr/bin/python

# Layer 1: Core dependencies (small, ~200MB)
RUN pip install --no-cache-dir \
    flask>=2.0.0 \
    grpcio>=1.50.0 \
    protobuf>=4.21.0 \
    pillow>=9.0.0 \
    numpy>=1.21.0 \
    requests>=2.28.0

# Layer 2: OpenCV (headless, ~50MB)
RUN pip install --no-cache-dir opencv-python-headless>=4.7.0

# Layer 3: PyTorch with CUDA 12.1 (~2.5GB)
# Using cu121 index for CUDA 12.1 compatibility
RUN pip install --no-cache-dir \
    torch --index-url https://download.pytorch.org/whl/cu121

# Layer 4: TorchVision (~500MB)
RUN pip install --no-cache-dir \
    torchvision --index-url https://download.pytorch.org/whl/cu121

# Layer 5: Ultralytics YOLO (~100MB)
RUN pip install --no-cache-dir ultralytics>=8.0.0

# Copy compiled proto files from builder
COPY --from=builder /build/detection_pb2.py /build/detection_pb2_grpc.py /app/

# Copy application code
COPY main.py grpc_server.py ./

# Create directories for YOLO models and runtime
RUN mkdir -p /app/models /app/runs /tmp/Ultralytics \
    && chmod 777 /app/runs /tmp/Ultralytics

# Set Ultralytics config directory
ENV YOLO_CONFIG_DIR=/tmp/Ultralytics
ENV ULTRALYTICS_SETTINGS_JSON='{"runs_dir": "/app/runs"}'

# Download YOLO11 model weights (nano models, ~6MB each)
RUN python -c "from ultralytics import YOLO; YOLO('yolo11n.pt'); YOLO('yolo11n-seg.pt'); YOLO('yolo11n-pose.pt')"

# Create non-root user
RUN groupadd -g 1000 yolo && useradd -u 1000 -g yolo -s /bin/sh -m yolo \
    && chown -R yolo:yolo /app /tmp/Ultralytics

USER yolo

# Expose ports (HTTP and gRPC)
EXPOSE 8081 50051

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import requests; requests.get('http://localhost:8081/health')" || exit 1

CMD ["python", "main.py"]
