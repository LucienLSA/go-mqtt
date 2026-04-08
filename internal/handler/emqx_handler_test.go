package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestParseOnlineStatusByEvent(t *testing.T) {
	status, ok := parseOnlineStatus("client.connected", "")
	if !ok || status != 1 {
		t.Fatalf("expected connected => (1,true), got (%d,%v)", status, ok)
	}

	status, ok = parseOnlineStatus("client.disconnected", "")
	if !ok || status != 0 {
		t.Fatalf("expected disconnected => (0,true), got (%d,%v)", status, ok)
	}
}

func TestParseOnlineStatusByActionFallback(t *testing.T) {
	status, ok := parseOnlineStatus("", "connected")
	if !ok || status != 1 {
		t.Fatalf("expected action connected => (1,true), got (%d,%v)", status, ok)
	}

	status, ok = parseOnlineStatus("", "disconnected")
	if !ok || status != 0 {
		t.Fatalf("expected action disconnected => (0,true), got (%d,%v)", status, ok)
	}
}

func TestParseOnlineStatusUnsupported(t *testing.T) {
	status, ok := parseOnlineStatus("session.created", "created")
	if ok {
		t.Fatalf("expected unsupported event => ok=false, got status=%d", status)
	}
}

func TestWebhookIPWhitelist(t *testing.T) {
	t.Setenv("EMQX_WEBHOOK_IP_WHITELIST", "127.0.0.1,10.0.0.0/8")
	if !isWebhookIPAllowed("127.0.0.1") {
		t.Fatal("expected 127.0.0.1 to be allowed")
	}
	if !isWebhookIPAllowed("10.1.2.3") {
		t.Fatal("expected 10.1.2.3 to be allowed by cidr")
	}
	if isWebhookIPAllowed("192.168.1.1") {
		t.Fatal("expected 192.168.1.1 to be denied")
	}
}

func TestWebhookHMACVerify(t *testing.T) {
	body := []byte(`{"event":"client.connected","username":"DEV001"}`)
	t.Setenv("EMQX_WEBHOOK_HMAC_SECRET", "top-secret")

	mac := hmac.New(sha256.New, []byte("top-secret"))
	_, _ = mac.Write(body)
	signHex := hex.EncodeToString(mac.Sum(nil))

	if !verifyWebhookSignature(signHex, body) {
		t.Fatal("expected plain hex signature to pass")
	}
	if !verifyWebhookSignature("sha256="+signHex, body) {
		t.Fatal("expected sha256= prefixed signature to pass")
	}
	if verifyWebhookSignature("sha256=deadbeef", body) {
		t.Fatal("expected invalid signature to fail")
	}
}
