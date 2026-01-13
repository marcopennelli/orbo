# Orbo Podman Deployment

This directory contains Podman Quadlet files and container manifests for deploying Orbo on embedded Linux systems using [meta-container-deploy](https://github.com/marcopennelli/meta-container-deploy).

## Deployment Profiles

Orbo supports multiple deployment profiles to match your hardware and use case:

| Profile | Containers | Detection | Use Case |
|---------|------------|-----------|----------|
| **basic** | orbo | Motion only | Low-power devices, 24/7 monitoring |
| **ai** | orbo, yolo-service | YOLO object detection | Standard security with person/vehicle detection |
| **full** | orbo, yolo-service, recognition | YOLO + Face recognition | Full AI with identity matching |

### Detection Configuration per Profile

| Profile | `DETECTION_MODE` | `DETECTION_DETECTORS` | `DETECTION_EXECUTION_MODE` |
|---------|------------------|----------------------|----------------------------|
| basic | `motion_triggered` | `` (empty) | `sequential` |
| ai | `motion_triggered` | `yolo` | `sequential` |
| full | `motion_triggered` | `yolo,face` | `sequential` |

### Recommended Hardware

| Profile | Minimum RAM | CPU | Storage | Notes |
|---------|-------------|-----|---------|-------|
| basic | 512MB | 1 core | 8GB | Works on RPi 3/Zero 2 |
| ai | 2GB | 2 cores | 16GB | RPi 4, ~2-3 FPS on CPU |
| full | 4GB+ | 4 cores | 32GB | RPi 4 8GB or Jetson Nano recommended |

## Quick Start (Standard Linux)

### Using Quadlet (systemd)

Copy the Quadlet files to your systemd containers directory:

```bash
# Create required directories
sudo mkdir -p /var/lib/orbo/{frames,faces}
sudo chown 1000:1000 /var/lib/orbo/{frames,faces}

# Copy Quadlet files
sudo cp quadlet/*.container /etc/containers/systemd/

# Reload systemd to generate services
sudo systemctl daemon-reload

# Start services
sudo systemctl start orbo.service

# Enable at boot
sudo systemctl enable orbo.service
```

### With AI Detection

Enable YOLO and/or face recognition:

```bash
# Start YOLO service
sudo systemctl start yolo-service.service

# Start recognition service
sudo systemctl start recognition.service

# Update orbo.container to enable AI
# Edit /etc/containers/systemd/orbo.container:
#   Environment=YOLO_ENABLED=true
#   Environment=DETECTION_DETECTORS=yolo,face
#   Environment=RECOGNITION_ENABLED=true

sudo systemctl daemon-reload
sudo systemctl restart orbo.service
```

## Yocto/OpenEmbedded Deployment

### Using meta-container-deploy

1. Add the layer to your `bblayers.conf`:

```bitbake
BBLAYERS += "${BSPDIR}/sources/meta-container-deploy"
```

2. Configure in `local.conf`:

```bitbake
# Enable container support
DISTRO_FEATURES:append = " systemd virtualization"

# Basic deployment (motion detection only)
CONTAINER_MANIFEST = "${THISDIR}/orbo-manifest-basic.yaml"

# AI deployment (YOLO object detection)
# CONTAINER_MANIFEST = "${THISDIR}/orbo-manifest.yaml"

# Full AI deployment (YOLO + face recognition)
# CONTAINER_MANIFEST = "${THISDIR}/orbo-manifest-full.yaml"

# Or use individual variables:
CONTAINER_orbo_IMAGE = "ghcr.io/marcopennelli/orbo:latest"
CONTAINER_orbo_PORTS = "8080:8080"
CONTAINER_orbo_VOLUMES = "/var/lib/orbo/frames:/app/frames:rw"
CONTAINER_orbo_DEVICES = "/dev/video0:/dev/video0"
CONTAINER_orbo_ENVIRONMENT = "DETECTION_MODE=motion_triggered DETECTION_DETECTORS=yolo"
```

3. Add to your image:

```bitbake
IMAGE_INSTALL:append = " container-orbo"
```

### Example Manifests

See `examples/rpi-yocto/` for Raspberry Pi-specific manifests:
- `orbo-manifest-basic.yaml` - Motion detection only (lightweight)
- `orbo-manifest-full.yaml` - Full AI with YOLO and face recognition

## Container Images

| Image | Description | Size |
|-------|-------------|------|
| `ghcr.io/marcopennelli/orbo:latest` | Main application | ~200MB |
| `ghcr.io/marcopennelli/orbo-yolo:latest` | YOLO detection | ~500MB |
| `ghcr.io/marcopennelli/orbo-recognition:latest` | Face recognition | ~400MB |

## Configuration

### Environment Variables

Configure via Quadlet `Environment=` directives or manifest `environment:` section:

#### Detection Pipeline

| Variable | Default | Description |
|----------|---------|-------------|
| `DETECTION_MODE` | motion_triggered | Detection mode (see table below) |
| `DETECTION_EXECUTION_MODE` | sequential | `sequential` or `parallel` |
| `DETECTION_DETECTORS` | yolo | Comma-separated detector list |
| `DETECTION_SCHEDULE_INTERVAL` | 5s | Interval for scheduled/hybrid modes |
| `MOTION_SENSITIVITY` | 0.1 | Motion detection sensitivity (0.0-1.0) |
| `MOTION_COOLDOWN_SECONDS` | 2 | Cooldown after motion stops |

**Detection Modes:**

| Mode | Description | Use Case |
|------|-------------|----------|
| `disabled` | No detection, streaming only | Low resource viewing |
| `visual_only` | Detection runs (bounding boxes visible) but no alerts | Testing, monitoring without notifications |
| `continuous` | Run detection on every frame | High security areas |
| `motion_triggered` | Detect only when motion is detected (default) | Static scenes |
| `scheduled` | Run detection at fixed intervals | Periodic sampling |
| `hybrid` | Motion-triggered OR scheduled | Balanced approach |

**Execution Modes:**

| Mode | Description | Use Case |
|------|-------------|----------|
| `sequential` | Chain: YOLO → Face (if person) → Plate (if vehicle) | Efficient, conditional |
| `parallel` | Run all detectors simultaneously | Maximum speed |

#### Detector Services

| Variable | Default | Description |
|----------|---------|-------------|
| `YOLO_ENABLED` | false | Enable YOLO detection |
| `YOLO_ENDPOINT` | http://localhost:8081 | YOLO service URL |
| `RECOGNITION_ENABLED` | false | Enable face recognition |
| `RECOGNITION_SERVICE_ENDPOINT` | http://localhost:8082 | Recognition service URL |

#### YOLO Service Configuration

The YOLO service now supports YOLO11 with multiple task types: detection, pose estimation, segmentation, oriented bounding boxes, and classification.

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL_SIZE` | n | Model size: n (nano), s (small), m (medium), l (large), x (xlarge) |
| `CONFIDENCE_THRESHOLD` | 0.5 | Detection confidence threshold (0.0-1.0) |
| `ENABLED_TASKS` | detect | Comma-separated list of tasks: detect, pose, segment, obb, classify |
| `MAX_CACHED_MODELS` | 3 | Maximum models to keep in LRU cache |
| `PRELOAD_TASKS` | detect | Tasks to preload on startup (empty = load on first request) |
| `BOX_COLOR` | #0066FF | Bounding box color for detection (hex format) |
| `BOX_THICKNESS` | 2 | Bounding box line thickness (1-5 pixels) |
| `POSE_COLOR` | #FF00FF | Skeleton/keypoint color for pose estimation (hex) |
| `FALL_DETECTION` | true | Enable fall detection alerts from pose analysis |
| `ALERT_ON_POSES` | fall_detected,lying | Pose classifications that trigger alerts |
| `SEGMENT_COLOR` | #00FFFF | Segmentation mask outline color (hex) |
| `SEGMENT_ALPHA` | 0.4 | Segmentation mask transparency (0.0-1.0) |
| `OBB_COLOR` | #FFFF00 | Oriented bounding box color (hex) |

**YOLO11 Tasks:**

| Task | Model | Description |
|------|-------|-------------|
| `detect` | yolo11{size}.pt | Standard object detection (80 COCO classes) |
| `pose` | yolo11{size}-pose.pt | Human pose estimation (17 COCO keypoints) |
| `segment` | yolo11{size}-seg.pt | Instance segmentation with pixel masks |
| `obb` | yolo11{size}-obb.pt | Oriented/rotated bounding boxes |
| `classify` | yolo11{size}-cls.pt | Image/scene classification |

**Memory Note:** Each task requires its own model. Use `MAX_CACHED_MODELS` to limit memory usage - models are loaded on-demand and evicted using LRU when the cache is full.

#### Face Recognition Service Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SIMILARITY_THRESHOLD` | 0.5 | Face matching threshold (0.0-1.0) |
| `FACES_DB_PATH` | /app/data/faces.pkl | Path to face database file |
| `FACES_IMAGES_PATH` | /app/data/faces | Path to face images directory |
| `KNOWN_FACE_COLOR` | #00FF00 | Bounding box color for known faces (hex) |
| `UNKNOWN_FACE_COLOR` | #FF6600 | Bounding box color for unknown faces (hex) |
| `BOX_THICKNESS` | 2 | Bounding box line thickness (1-5 pixels) |

#### Notifications

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_ENABLED` | false | Enable Telegram notifications |
| `TELEGRAM_BOT_TOKEN` | - | Telegram bot token |
| `TELEGRAM_CHAT_ID` | - | Telegram chat ID |
| `TELEGRAM_COOLDOWN` | 30 | Notification cooldown (seconds) |

#### Legacy (Backwards Compatibility)

| Variable | Default | Description |
|----------|---------|-------------|
| `PRIMARY_DETECTOR` | basic | Legacy detection mode (basic/yolo) |

### Volumes

| Host Path | Container Path | Purpose |
|-----------|----------------|---------|
| `/var/lib/orbo/frames` | `/app/frames` | Frame storage & SQLite DB |
| `/var/lib/orbo/faces` | `/app/data` | Face database (faces.pkl) & face images |

**Note:** The face recognition volume changed from `/app/faces` to `/app/data` to align with the service's internal structure.

### Devices

| Device | Purpose |
|--------|---------|
| `/dev/video0` | USB camera access |
| `/dev/video1` | Additional cameras |

## Per-Camera Configuration

Each camera can override global detection settings via the API:

```bash
# Set camera to continuous detection mode
curl -X PUT http://localhost:8080/api/v1/cameras/{id}/detection-config \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "continuous",
    "detectors": ["yolo", "face"],
    "executionMode": "sequential"
  }'
```

Cameras inherit from global defaults and can override:
- `mode` - Detection mode for this camera
- `executionMode` - Sequential or parallel
- `detectors` - List of detectors to use
- `scheduleInterval` - For scheduled/hybrid modes
- `motionSensitivity` - Motion detection threshold

## GPU Support

For NVIDIA GPU acceleration in YOLO service, uncomment in `yolo-service.container`:

```ini
AddDevice=nvidia.com/gpu=all
```

For Yocto, ensure `meta-tegra` or equivalent GPU layer is included.

## Networking

Containers communicate via localhost ports:
- Orbo: 8080
- YOLO: 8081
- Recognition: 8082

For pod networking, create a Quadlet `.pod` file to group services.

## Troubleshooting

```bash
# Check container status
podman ps -a

# View logs
podman logs orbo
journalctl -u orbo.service

# Check Quadlet generation
/usr/lib/systemd/system-generators/podman-system-generator --dryrun

# Verify camera access
ls -la /dev/video*
```

## Building Images

To build custom images for your architecture:

```bash
cd deploy
make docker           # Build orbo image
make docker-yolo      # Build YOLO image
make docker-recognition  # Build recognition image
```

For arm64 (e.g., Raspberry Pi, Jetson):

```bash
docker buildx build --platform linux/arm64 -t orbo:arm64 .
```
