#!/usr/bin/env bash
set -euo pipefail

# Simple mock data generator:
# 1) Login
# 2) Create N devices

BASE_URL="${BASE_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
DEVICE_COUNT="${DEVICE_COUNT:-10}"
PREFIX="${PREFIX:-simple-mock-device}"
GROUP_ID="${GROUP_ID:-100}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2 ;;
    --username) USERNAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --device-count) DEVICE_COUNT="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    --group-id) GROUP_ID="$2"; shift 2 ;;
    *)
      echo "Unknown arg: $1"
      echo "Usage: $0 [--base-url URL] [--username USER] [--password PASS] [--device-count N] [--prefix NAME_PREFIX] [--group-id ID]"
      exit 1
      ;;
  esac
done

for cmd in curl jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing dependency: $cmd"
    exit 1
  fi
done

echo "[1/3] Login..."
login_payload=$(jq -nc --arg u "$USERNAME" --arg p "$PASSWORD" '{username:$u,password:$p}')
login_resp=$(curl -sS -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" -d "$login_payload")

code=$(echo "$login_resp" | jq -r '.code // -1')
token=$(echo "$login_resp" | jq -r '.data.token // empty')
if [[ "$code" != "0" || -z "$token" ]]; then
  echo "Login failed: $login_resp"
  exit 1
fi

echo "[2/3] Create devices: $DEVICE_COUNT"
ok=0
failed=0

for i in $(seq 1 "$DEVICE_COUNT"); do
  name="$PREFIX-$i"
  body=$(jq -nc --arg n "$name" --argjson gid "$GROUP_ID" '{name:$n,group_id:$gid}')

  resp=$(curl -sS -X POST "$BASE_URL/api/v1/device" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "$body")

  rcode=$(echo "$resp" | jq -r '.code // -1')
  if [[ "$rcode" == "0" ]]; then
    ok=$((ok + 1))
    id=$(echo "$resp" | jq -r '.data.id // empty')
    did=$(echo "$resp" | jq -r '.data.device_id // empty')
    echo "  + $name created (id=$id, device_id=$did)"
  else
    failed=$((failed + 1))
    msg=$(echo "$resp" | jq -r '.message // .msg // "unknown error"')
    echo "  ! $name failed: $msg"
  fi
done

echo "[3/3] Done. success=$ok, failed=$failed"
