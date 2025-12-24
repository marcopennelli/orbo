FROM python:3.11-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    libgomp1 \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy service code
COPY dinov3_service.py .
COPY download_model.py .

# Pre-download the model during build (optional)
# RUN python download_model.py

# Create non-root user with cache directory
# Using a generic UID that won't conflict
RUN useradd -m -u 1001 dinov3user && \
    mkdir -p /home/dinov3user/.cache/torch/hub && \
    chmod -R 777 /home/dinov3user/.cache && \
    chown -R dinov3user:dinov3user /app
USER dinov3user

# Set HOME environment variable
ENV HOME=/home/dinov3user
ENV TORCH_HOME=/home/dinov3user/.cache/torch

# Expose port
EXPOSE 8001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD python -c "import requests; requests.get('http://localhost:8001/health')" || exit 1

# Start service
CMD ["python", "dinov3_service.py"]