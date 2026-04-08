#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
LIMIT="${LIMIT:-5}"
COMMAND="${COMMAND:-reboot}"
DELAY_SECONDS="${DELAY_SECONDS:-1}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2 ;;
    --username) USERNAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --limit) LIMIT="$2"; shift 2 ;;
    --command) COMMAND="$2"; shift 2 ;;
    --delay-seconds) DELAY_SECONDS="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

for cmd in curl jq; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing dependency: $cmd"; exit 1; }
done

echo "[1/4] Login and get token..."
login_payload=$(jq -nc --arg u "$USERNAME" --arg p "$PASSWORD" '{username:$u,password:$p}')
login_resp=$(curl -sS -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" -d "$login_payload")

code=$(echo "$login_resp" | jq -r '.code // -1')
token=$(echo "$login_resp" | jq -r '.data.token // empty')
if [[ "$code" != "0" || -z "$token" ]]; then
  echo "Login failed: $login_resp"
  exit 1
fi

echo "[2/4] Query device list..."
list_resp=$(curl -sS -X GET "$BASE_URL/api/v1/device" -H "Authorization: Bearer $token")
devices_json=$(echo "$list_resp" | jq -c --argjson lim "$LIMIT" '.data // [] | .[0:$lim]')
device_count=$(echo "$devices_json" | jq 'length')
if [[ "$device_count" -eq 0 ]]; then
  echo "No devices found. Run seed_api_data.sh first."
  exit 1
fi

echo "[3/4] Batch send control commands..."
results_file=$(mktemp)
echo "[]" > "$results_file"

for i in $(seq 0 $((device_count - 1))); do
  id=$(echo "$devices_json" | jq -r ".[$i].id")
  device_id=$(echo "$devices_json" | jq -r ".[$i].device_id")

  body=$(jq -nc --arg cmd "$COMMAND" --argjson delay "$DELAY_SECONDS" '{cmd:$cmd,param:{delay:$delay}}')
  resp=$(curl -sS -X POST "$BASE_URL/api/v1/device/$id/control" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "$body")

  rcode=$(echo "$resp" | jq -r '.code // -1')
  if [[ "$rcode" == "0" ]]; then
    trace_id=$(echo "$resp" | jq -r '.data.trace_id')
    timeout_at=$(echo "$resp" | jq -r '.data.timeout_at')
    max_retry=$(echo "$resp" | jq -r '.data.max_retry')

    tmp=$(mktemp)
    jq --arg id "$id" --arg did "$device_id" --arg trace "$trace_id" --arg to "$timeout_at" --arg mr "$max_retry" \
      '. += [{id:$id,device_id:$did,trace_id:$trace,timeout_at:$to,max_retry:$mr}]' "$results_file" > "$tmp"
    mv "$tmp" "$results_file"

    echo "  + control ok id=$id trace_id=$trace_id"
  else
    echo "  ! control failed id=$id resp=$resp"
  fi
done

echo "[4/4] Query command history for first device..."
first_id=$(echo "$devices_json" | jq -r '.[0].id')
first_device_id=$(echo "$devices_json" | jq -r '.[0].device_id')
history=$(curl -sS -X GET "$BASE_URL/api/v1/device/$first_id/command?limit=10" -H "Authorization: Bearer $token")
history_count=$(echo "$history" | jq -r '(.data // []) | length')
echo "  device_id=$first_device_id command_count=$history_count"

echo "Result summary:"
jq '.' "$results_file"
rm -f "$results_file"
echo "Done."