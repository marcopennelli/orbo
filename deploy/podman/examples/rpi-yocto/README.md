# Raspberry Pi + Yocto Deployment Example

This example shows how to deploy Orbo on a Raspberry Pi running a Yocto-based image with a USB camera for home security monitoring.

## Target Setup

- **Hardware**: Raspberry Pi 4 (4GB+ recommended)
- **Camera**: USB webcam (UVC compatible)
- **Storage**: 16GB+ SD card
- **Network**: WiFi or Ethernet for Telegram alerts

## Deployment Profiles

| Profile | Manifest | Detection | RAM Required |
|---------|----------|-----------|--------------|
| **basic** | `orbo-manifest-basic.yaml` | Motion only | 512MB |
| **full** | `orbo-manifest-full.yaml` | YOLO + Face | 4GB+ |

### Detection Modes

Each profile uses the modular detection pipeline:

| Mode | Description | Use Case |
|------|-------------|----------|
| `disabled` | Streaming only | Low resource viewing |
| `continuous` | Every frame | High security (resource intensive) |
| `motion_triggered` | Motion first, then AI | **Recommended** for RPi |
| `scheduled` | Fixed intervals | Periodic sampling |
| `hybrid` | Motion OR scheduled | Guaranteed coverage |

### Execution Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| `sequential` | YOLO → Face (if person) → Plate (if vehicle) | **Recommended** - efficient |
| `parallel` | All detectors at once | Maximum speed (more resources) |

## Yocto Layer Setup

### 1. Clone Required Layers

```bash
cd poky
git clone https://github.com/marcopennelli/meta-container-deploy.git sources/meta-container-deploy
git clone -b scarthgap https://git.yoctoproject.org/meta-virtualization sources/meta-virtualization
git clone -b scarthgap https://git.yoctoproject.org/meta-raspberrypi sources/meta-raspberrypi
```

### 2. Configure bblayers.conf

```bitbake
BBLAYERS ?= " \
  ${BSPDIR}/sources/poky/meta \
  ${BSPDIR}/sources/poky/meta-poky \
  ${BSPDIR}/sources/meta-openembedded/meta-oe \
  ${BSPDIR}/sources/meta-openembedded/meta-python \
  ${BSPDIR}/sources/meta-openembedded/meta-networking \
  ${BSPDIR}/sources/meta-raspberrypi \
  ${BSPDIR}/sources/meta-virtualization \
  ${BSPDIR}/sources/meta-container-deploy \
"
```

### 3. Configure local.conf

```bitbake
# Machine configuration
MACHINE = "raspberrypi4-64"

# Enable systemd and container support
DISTRO_FEATURES:append = " systemd virtualization"
DISTRO_FEATURES_BACKFILL_CONSIDERED = "sysvinit"
VIRTUAL-RUNTIME_init_manager = "systemd"
VIRTUAL-RUNTIME_initscripts = ""

# Enable USB camera support
IMAGE_INSTALL:append = " v4l-utils"
KERNEL_MODULE_AUTOLOAD:append = " uvcvideo"

# Container deployment - Basic (motion detection only)
CONTAINER_MANIFEST = "${THISDIR}/orbo-manifest-basic.yaml"

# Or for full AI deployment (requires more storage):
# CONTAINER_MANIFEST = "${THISDIR}/orbo-manifest-full.yaml"

# Add Orbo containers to image
IMAGE_INSTALL:append = " container-orbo"

# Ensure sufficient rootfs size for containers
IMAGE_ROOTFS_EXTRA_SPACE = "2097152"
```

## Container Manifests

### Basic Deployment (Motion Detection)

Uses `orbo-manifest-basic.yaml` - motion detection only, no AI services:

```yaml
containers:
  - name: orbo
    image: ghcr.io/marcopennelli/orbo:latest
    ports:
      - "8080:8080"
    volumes:
      - "/var/lib/orbo/frames:/app/frames:rw"
    devices:
      - "/dev/video0:/dev/video0"
    environment:
      # Detection Pipeline - motion only
      DETECTION_MODE: "motion_triggered"
      DETECTION_EXECUTION_MODE: "sequential"
      DETECTION_DETECTORS: ""  # Empty = motion only, no AI
      MOTION_SENSITIVITY: "0.1"
      MOTION_COOLDOWN_SECONDS: "2"
      # Services disabled
      YOLO_ENABLED: "false"
      RECOGNITION_ENABLED: "false"
      # Storage
      DATABASE_PATH: "/app/frames/orbo.db"
      FRAME_DIR: "/app/frames"
      # Notifications
      TELEGRAM_ENABLED: "true"
      TELEGRAM_BOT_TOKEN: "${TELEGRAM_BOT_TOKEN}"
      TELEGRAM_CHAT_ID: "${TELEGRAM_CHAT_ID}"
    restart_policy: always
    user: "1000:1000"
```

### Full AI Deployment

Uses `orbo-manifest-full.yaml` - YOLO object detection + face recognition:

```yaml
containers:
  - name: orbo
    image: ghcr.io/marcopennelli/orbo:latest
    ports:
      - "8080:8080"
    volumes:
      - "/var/lib/orbo/frames:/app/frames:rw"
    devices:
      - "/dev/video0:/dev/video0"
    environment:
      # Detection Pipeline - full AI
      DETECTION_MODE: "motion_triggered"
      DETECTION_EXECUTION_MODE: "sequential"
      DETECTION_DETECTORS: "yolo,face"
      DETECTION_SCHEDULE_INTERVAL: "5s"
      MOTION_SENSITIVITY: "0.1"
      MOTION_COOLDOWN_SECONDS: "2"
      # Services enabled
      YOLO_ENABLED: "true"
      YOLO_ENDPOINT: "http://localhost:8081"
      RECOGNITION_ENABLED: "true"
      RECOGNITION_SERVICE_ENDPOINT: "http://localhost:8082"
      # Storage
      DATABASE_PATH: "/app/frames/orbo.db"
      # Notifications
      TELEGRAM_ENABLED: "true"
    restart_policy: always
    depends_on:
      - yolo-service
      - recognition
    user: "1000:1000"

  - name: yolo-service
    image: ghcr.io/marcopennelli/orbo-yolo:latest
    ports:
      - "8081:8081"
    environment:
      YOLO_MODEL: "yolo11n.pt"
      CONFIDENCE_THRESHOLD: "0.5"
    restart_policy: always
    user: "1000:1000"

  - name: recognition
    image: ghcr.io/marcopennelli/orbo-recognition:latest
    ports:
      - "8082:8082"
    volumes:
      - "/var/lib/orbo/faces:/app/faces:rw"
    restart_policy: always
    user: "1000:1000"
```

## Build and Flash

```bash
# Build the image
bitbake core-image-minimal

# Flash to SD card
sudo dd if=tmp/deploy/images/raspberrypi4-64/core-image-minimal-raspberrypi4-64.wic \
    of=/dev/sdX bs=4M status=progress
sync
```

## First Boot Configuration

### 1. Connect and Configure

```bash
# SSH into the Pi (default: root, no password)
ssh root@<pi-ip>

# Set Telegram credentials (if using alerts)
cat > /etc/orbo-env.conf << EOF
TELEGRAM_BOT_TOKEN=your_bot_token_here
TELEGRAM_CHAT_ID=your_chat_id_here
EOF

# Restart Orbo to apply
systemctl restart orbo.service
```

### 2. Verify Camera

```bash
# Check USB camera detected
v4l2-ctl --list-devices

# Should show something like:
# USB Camera (usb-0000:01:00.0-1.2):
#     /dev/video0
```

### 3. Access Web UI

Open `http://<pi-ip>:8080` in your browser to:
- Add cameras (use `/dev/video0` for USB camera)
- Configure detection settings per camera
- View live streams
- Browse motion events

### 4. Per-Camera Configuration

Each camera can override global detection settings:

```bash
# Set front door to continuous detection
curl -X PUT http://localhost:8080/api/v1/cameras/{id}/detection-config \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "continuous",
    "detectors": ["yolo", "face"]
  }'

# Set backyard to scheduled detection (every 10 seconds)
curl -X PUT http://localhost:8080/api/v1/cameras/{id}/detection-config \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "scheduled",
    "scheduleInterval": "10s",
    "detectors": ["yolo"]
  }'
```

## Performance Considerations

### Raspberry Pi 4 (4GB)

| Profile | Detection Mode | CPU Usage | Memory | FPS |
|---------|----------------|-----------|--------|-----|
| basic | motion_triggered | ~15% | ~200MB | 30 |
| full | motion_triggered | ~80% | ~1.2GB | 2-3 |
| full | continuous | ~95% | ~1.5GB | 1-2 |

### Optimizations

For resource-constrained devices:

```yaml
# Use motion_triggered mode (not continuous)
DETECTION_MODE: "motion_triggered"

# Use sequential execution (not parallel)
DETECTION_EXECUTION_MODE: "sequential"

# Higher confidence = fewer detections = faster
environment:
  YOLO_MODEL: "yolo11n.pt"  # Use nano model
  CONFIDENCE_THRESHOLD: "0.6"
```

## Telegram Setup

1. Create bot: Message [@BotFather](https://t.me/BotFather) → `/newbot`
2. Get chat ID: `curl https://api.telegram.org/bot<TOKEN>/getUpdates`
3. Configure in manifest or at runtime

## Troubleshooting

### Camera Not Detected

```bash
# Check kernel module
lsmod | grep uvcvideo

# Load manually if needed
modprobe uvcvideo

# Check permissions
ls -la /dev/video*
```

### Container Won't Start

```bash
# Check container logs
podman logs orbo

# Check systemd service
journalctl -u orbo.service -f

# Verify image pulled
podman images
```

### Web UI Not Accessible

```bash
# Check container is running
podman ps

# Check port binding
ss -tlnp | grep 8080

# Check firewall
iptables -L -n
```

## Storage Management

Motion events accumulate frames. Set up periodic cleanup:

```bash
# Add to crontab
cat >> /var/spool/cron/crontabs/root << EOF
# Clean frames older than 7 days
0 3 * * * find /var/lib/orbo/frames -name "*.jpg" -mtime +7 -delete
EOF
```
