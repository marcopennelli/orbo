#!/bin/bash
set -e

echo "🎥 Orbo Camera Test Script"
echo "========================="

echo ""
echo "📋 System Information:"
echo "- User: $(whoami)"
echo "- Groups: $(groups)"
echo "- Video group check: $(groups | grep -q video && echo "✅ In video group" || echo "❌ NOT in video group")"

echo ""
echo "🔍 Video Devices:"
if ls /dev/video* >/dev/null 2>&1; then
    for device in /dev/video*; do
        if [ -e "$device" ]; then
            perms=$(stat -c '%A' "$device" 2>/dev/null || stat -f '%Sp' "$device" 2>/dev/null || echo "unknown")
            owner=$(stat -c '%U:%G' "$device" 2>/dev/null || stat -f '%Su:%Sg' "$device" 2>/dev/null || echo "unknown")
            echo "  $device - $perms - $owner"
        fi
    done
else
    echo "  ❌ No video devices found"
    exit 1
fi

echo ""
echo "🧪 Test Camera Access:"
primary_camera="/dev/video0"
if [ -r "$primary_camera" ] && [ -w "$primary_camera" ]; then
    echo "  ✅ Can read/write $primary_camera"
else
    echo "  ❌ Cannot access $primary_camera"
    echo "  💡 Try running: sudo usermod -a -G video $USER"
    echo "  💡 Then logout and login again"
fi

echo ""
echo "🔧 Required Tools Check:"
command -v podman >/dev/null 2>&1 && echo "  ✅ Podman available: $(podman --version)" || echo "  ❌ Podman not found"
command -v kubectl >/dev/null 2>&1 && echo "  ✅ kubectl available: $(kubectl version --client --short 2>/dev/null)" || echo "  ⚠️  kubectl not found (needed for K8s deployment)"
command -v helm >/dev/null 2>&1 && echo "  ✅ Helm available: $(helm version --short 2>/dev/null)" || echo "  ⚠️  Helm not found (needed for K8s deployment)"

echo ""
echo "🐳 Container Test:"
echo "Once the Orbo container is built, you can test it with:"
echo "  cd deploy && make run"
echo ""
echo "This will:"
echo "  - Mount $primary_camera into the container"
echo "  - Expose port 8080 for the REST API"
echo "  - Create a volume for storing captured frames"

echo ""
echo "🌐 API Test Commands (after container is running):"
cat << 'EOF'
  # Check health
  curl http://localhost:8080/healthz
  curl http://localhost:8080/readyz

  # List cameras
  curl http://localhost:8080/api/v1/cameras

  # Add a camera
  curl -X POST http://localhost:8080/api/v1/cameras \
    -H "Content-Type: application/json" \
    -d '{"name":"USB Camera","device":"/dev/video0"}'

  # Get system status
  curl http://localhost:8080/api/v1/system/status
EOF

echo ""
echo "📱 Telegram Setup (optional):"
echo "1. Message @BotFather on Telegram"
echo "2. Create a new bot with /newbot"
echo "3. Get your bot token"
echo "4. Send a message to your bot"
echo "5. Get your chat ID from: https://api.telegram.org/bot<TOKEN>/getUpdates"
echo "6. Set environment variables:"
echo "   export TELEGRAM_BOT_TOKEN='your_token_here'"
echo "   export TELEGRAM_CHAT_ID='your_chat_id_here'"

echo ""
echo "✅ Test complete!"