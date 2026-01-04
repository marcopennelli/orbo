FROM python:3.10-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    python3-pip \
    libgl1 \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    libgomp1 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .

# Install dependencies (onnxruntime needs specific version for compatibility)
RUN pip3 install --no-cache-dir -r requirements.txt

# Pre-download InsightFace model
RUN python3 -c "from insightface.app import FaceAnalysis; app = FaceAnalysis(name='buffalo_l'); app.prepare(ctx_id=-1, det_size=(640, 640))"

# Copy application code
COPY main.py .

# Create data directory for face database
RUN mkdir -p /app/data/faces

# Expose port
EXPOSE 8082

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=3 \
    CMD python3 -c "import requests; r = requests.get('http://localhost:8082/health'); exit(0 if r.json()['status'] == 'healthy' else 1)" || exit 1

# Run the application
CMD ["python3", "main.py"]
