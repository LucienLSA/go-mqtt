#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
GROUP_ID="${GROUP_ID:-100}"
WAIT_SEC="${WAIT_SEC:-8}"

SIM_PID=""
cleanup() {
  if [[ -n "$SIM_PID" ]]; then
    kill "$SIM_PID" >/dev/null 2>&1 || true
    wait "$SIM_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2 ;;
    --username) USERNAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --group-id) GROUP_ID="$2"; shift 2 ;;
    --wait-sec) WAIT_SEC="$2"; shift 2 ;;
    *)
      echo "Unknown arg: $1"
      echo "Usage: $0 [--base-url URL] [--username USER] [--password PASS] [--group-id ID] [--wait-sec N]"
      exit 1
      ;;
  esac
done

for cmd in curl jq go; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing dependency: $cmd"; exit 1; }
done

echo "[1/6] Login and get token..."
login_payload=$(jq -nc --arg u "$USERNAME" --arg p "$PASSWORD" '{username:$u,password:$p}')
login_resp=$(curl -sS -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" -d "$login_payload")
code=$(echo "$login_resp" | jq -r '.code // -1')
token=$(echo "$login_resp" | jq -r '.data.token // empty')
if [[ "$code" != "0" || -z "$token" ]]; then
  echo "Login failed: $login_resp"
  exit 1
fi

echo "[2/6] Create device..."
dev_name="e2e-device-$(date +%s)"
create_payload=$(jq -nc --arg n "$dev_name" --argjson gid "$GROUP_ID" '{name:$n,group_id:$gid}')
create_resp=$(curl -sS -X POST "$BASE_URL/api/v1/device" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "$create_payload")

ccode=$(echo "$create_resp" | jq -r '.code // -1')
if [[ "$ccode" != "0" ]]; then
  echo "Create device failed: $create_resp"
  exit 1
fi

device_id=$(echo "$create_resp" | jq -r '.data.device_id')
id=$(echo "$create_resp" | jq -r '.data.id')
echo "  + created id=$id device_id=$device_id"

echo "[3/6] Start simulator for device..."
(
  cd "$(dirname "$0")/../.."
  go run ./cmd/simulator -device-id "$device_id" -interval 2
) >/tmp/go_mqtt_e2e_simulator.log 2>&1 &
SIM_PID=$!
sleep 3

echo "[4/6] Send control command..."
control_payload='{"cmd":"reboot","param":{"delay":1}}'
control_resp=$(curl -sS -X POST "$BASE_URL/api/v1/device/$id/control" \
  -H "Authorization: Bearer $token" \
  -H "Content-Type: application/json" \
  -d "$control_payload")

kcode=$(echo "$control_resp" | jq -r '.code // -1')
if [[ "$kcode" != "0" ]]; then
  echo "Control failed: $control_resp"
  exit 1
fi

trace_id=$(echo "$control_resp" | jq -r '.data.trace_id // empty')
if [[ -z "$trace_id" ]]; then
  echo "Trace id missing in control response: $control_resp"
  exit 1
fi

echo "  + trace_id=$trace_id"

echo "[5/6] Wait feedback and query history..."
sleep "$WAIT_SEC"
history_resp=$(curl -sS -X GET "$BASE_URL/api/v1/device/$id/command?limit=20" \
  -H "Authorization: Bearer $token")

hcode=$(echo "$history_resp" | jq -r '.code // -1')
if [[ "$hcode" != "0" ]]; then
  echo "History query failed: $history_resp"
  exit 1
fi

status=$(echo "$history_resp" | jq -r --arg t "$trace_id" '.data[] | select(.trace_id==$t) | .status' | head -n 1)
result=$(echo "$history_resp" | jq -r --arg t "$trace_id" '.data[] | select(.trace_id==$t) | .result' | head -n 1)

if [[ -z "$status" ]]; then
  echo "Trace id not found in history: $trace_id"
  echo "$history_resp" | jq '.'
  exit 1
fi

echo "  + history matched trace_id=$trace_id status=$status result=$result"
if [[ "$status" != "1" ]]; then
  echo "E2E failed: command not successful yet"
  echo "$history_resp" | jq '.'
  exit 1
fi

echo "[6/6] E2E flow passed"
echo "Simulator log: /tmp/go_mqtt_e2e_simulator.log"
