#!/bin/bash
# ORBO End-to-End API Tests
# Usage: ./e2e-test.sh [namespace]

set -e

NAMESPACE="${1:-orbo}"
SERVICE_NAME="orbo"

echo "============================================"
echo "  ORBO End-to-End API Tests"
echo "============================================"

# Check if pod is running
if ! kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name="$SERVICE_NAME" --no-headers | grep -q Running; then
    echo "Error: No running pods found in namespace $NAMESPACE"
    exit 1
fi

# Get service IP
SVC_IP=$(kubectl get svc -n "$NAMESPACE" "$SERVICE_NAME" -o jsonpath='{.spec.clusterIP}')
echo "Service IP: $SVC_IP"
echo ""

PASSED=0
FAILED=0

# Function to run a test
run_test() {
    local name="$1"
    local expected="$2"
    local url="$3"
    local method="${4:-GET}"
    local data="$5"

    echo "--- Test: $name ---"

    if [ -n "$data" ]; then
        RESULT=$(kubectl run curl-test-$RANDOM --image=curlimages/curl --rm -i --restart=Never -n "$NAMESPACE" \
            -- curl -s -o /dev/null -w "%{http_code}" -X "$method" \
            -H "Content-Type: application/json" -d "$data" \
            "http://$SVC_IP:8080$url" 2>&1 | grep -oE '^[0-9]{3}' | head -1)
    else
        RESULT=$(kubectl run curl-test-$RANDOM --image=curlimages/curl --rm -i --restart=Never -n "$NAMESPACE" \
            -- curl -s -o /dev/null -w "%{http_code}" -X "$method" \
            "http://$SVC_IP:8080$url" 2>&1 | grep -oE '^[0-9]{3}' | head -1)
    fi

    if [ "$RESULT" = "$expected" ]; then
        echo "PASS: $method $url returned $RESULT"
        PASSED=$((PASSED+1))
    else
        echo "FAIL: $method $url returned '$RESULT' (expected $expected)"
        FAILED=$((FAILED+1))
    fi
    echo ""
}

# Run tests
run_test "Health Check" "200" "/healthz"
run_test "Readiness Check" "200" "/readyz"
run_test "System Status" "200" "/api/v1/system/status"
run_test "List Cameras" "200" "/api/v1/cameras"
run_test "Get Detection Config" "200" "/api/v1/config/detection"
run_test "Update Detection Config" "200" "/api/v1/config/detection" "PUT" '{"primary_detector":"basic","fallback_enabled":true}'
run_test "Get YOLO Config" "200" "/api/v1/config/yolo"
run_test "Get DINOv3 Config" "200" "/api/v1/config/dinov3"
run_test "Get Notifications Config" "200" "/api/v1/config/notifications"
run_test "Get Motion Events" "200" "/api/v1/motion/events"
run_test "Start Detection" "200" "/api/v1/system/detection/start" "POST"
run_test "Stop Detection" "200" "/api/v1/system/detection/stop" "POST"

echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
