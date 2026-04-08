#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123}"
DEVICE_COUNT="${DEVICE_COUNT:-5}"
SIMULATE_WEBHOOK="${SIMULATE_WEBHOOK:-true}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-}"
SIGN_HEADER="${SIGN_HEADER:-X-EMQX-Signature}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url) BASE_URL="$2"; shift 2 ;;
    --username) USERNAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --device-count) DEVICE_COUNT="$2"; shift 2 ;;
    --simulate-webhook) SIMULATE_WEBHOOK="$2"; shift 2 ;;
    --webhook-secret) WEBHOOK_SECRET="$2"; shift 2 ;;
    --sign-header) SIGN_HEADER="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

for cmd in curl jq; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Missing dependency: $cmd"; exit 1; }
done

hmac_sha256_hex() {
  local secret="$1"
  local payload="$2"
  printf '%s' "$payload" | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print $2}'
}

echo "[1/5] Login and get token..."
login_payload=$(jq -nc --arg u "$USERNAME" --arg p "$PASSWORD" '{username:$u,password:$p}')
login_resp=$(curl -sS -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" -d "$login_payload")

code=$(echo "$login_resp" | jq -r '.code // -1')
token=$(echo "$login_resp" | jq -r '.data.token // empty')
if [[ "$code" != "0" || -z "$token" ]]; then
  echo "Login failed: $login_resp"
  exit 1
fi

echo "[2/5] Create mock devices, count=$DEVICE_COUNT ..."
created_file=$(mktemp)
for i in $(seq 1 "$DEVICE_COUNT"); do
  name="mock-device-$i"
  body=$(jq -nc --arg n "$name" '{name:$n,group_id:100}')
  resp=$(curl -sS -X POST "$BASE_URL/api/v1/device" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "$body")

  rcode=$(echo "$resp" | jq -r '.code // -1')
  if [[ "$rcode" == "0" ]]; then
    echo "$resp" | jq -c '.data' >> "$created_file"
    did=$(echo "$resp" | jq -r '.data.device_id')
    id=$(echo "$resp" | jq -r '.data.id')
    echo "  + created id=$id device_id=$did"
  else
    echo "  ! create failed name=$name resp=$resp"
  fi
done

echo "[3/5] Query device list..."
list_resp=$(curl -sS -X GET "$BASE_URL/api/v1/device" -H "Authorization: Bearer $token")
count=$(echo "$list_resp" | jq -r '(.data // []) | length')
echo "  device count (returned): $count"

if [[ "$SIMULATE_WEBHOOK" == "true" && -s "$created_file" ]]; then
  echo "[4/5] Simulate webhook online events..."
  while IFS= read -r row; do
    did=$(echo "$row" | jq -r '.device_id')
    payload=$(jq -nc --arg d "$did" '{event:"client.connected",username:$d,clientid:("sim-"+$d)}')

    if [[ -n "$WEBHOOK_SECRET" ]]; then
      sign=$(hmac_sha256_hex "$WEBHOOK_SECRET" "$payload")
      resp=$(curl -sS -X POST "$BASE_URL/api/v1/emqx/webhook" \
        -H "Content-Type: application/json" \
        -H "$SIGN_HEADER: sha256=$sign" \
        -d "$payload")
    else
      resp=$(curl -sS -X POST "$BASE_URL/api/v1/emqx/webhook" \
        -H "Content-Type: application/json" \
        -d "$payload")
    fi

    echo "  webhook device_id=$did resp=$resp"
  done < "$created_file"
else
  echo "[4/5] Skip webhook simulation (disabled or no devices)"
fi

echo "[5/5] Created devices summary"
jq -s '.' "$created_file"
rm -f "$created_file"
echo "Done."
