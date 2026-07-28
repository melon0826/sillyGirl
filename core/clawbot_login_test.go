package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClawbotLoginConfirmedStoresToken(t *testing.T) {
	oldBase := clawbotLoginAPIBase
	oldClient := clawbotLoginHTTPClient
	oldToken := clawbotLoginBucket.GetString("token")
	oldEnable := clawbotLoginBucket.GetString("enable")
	oldAPIBase := clawbotLoginBucket.GetString("api_base")
	defer func() {
		clawbotLoginAPIBase = oldBase
		clawbotLoginHTTPClient = oldClient
		_, _, _ = clawbotLoginBucket.Set("token", oldToken)
		_, _, _ = clawbotLoginBucket.Set("enable", oldEnable)
		_, _, _ = clawbotLoginBucket.Set("api_base", oldAPIBase)
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/ilink/bot/get_bot_qrcode":
			if r.URL.Query().Get("bot_type") != clawbotLoginDefaultBotTyp {
				t.Fatalf("unexpected bot_type: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"qrcode":             "qr-token",
				"qrcode_img_content": "https://open.weixin.qq.com/connect/qrcode/test",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/ilink/bot/get_qrcode_status":
			if r.URL.Query().Get("qrcode") != "qr-token" {
				t.Fatalf("unexpected qrcode: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"status":        "confirmed",
				"bot_token":     "new-clawbot-token",
				"ilink_bot_id":  "bot-1",
				"ilink_user_id": "user-1",
				"baseurl":       serverURLForTest(r),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clawbotLoginAPIBase = server.URL
	clawbotLoginHTTPClient = server.Client()

	start, err := startClawbotLogin(context.Background(), "")
	if err != nil {
		t.Fatalf("start login failed: %v", err)
	}
	sessionID, _ := start["session"].(string)
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("session should not be empty")
	}
	status, err := pollClawbotLogin(context.Background(), sessionID, "")
	if err != nil {
		t.Fatalf("poll login failed: %v", err)
	}
	if !status.Connected {
		t.Fatalf("expected connected status, got %#v", status)
	}
	if got := clawbotLoginBucket.GetString("token"); got != "new-clawbot-token" {
		t.Fatalf("stored token = %q", got)
	}
	if got := clawbotLoginBucket.GetString("enable"); got != "true" {
		t.Fatalf("stored enable = %q", got)
	}
}

func TestClawbotRedirectBaseURL(t *testing.T) {
	if got := clawbotRedirectBaseURL("ilink.example.com"); got != "https://ilink.example.com" {
		t.Fatalf("unexpected redirect base url: %q", got)
	}
	for _, value := range []string{"", "evil.com/path", "evil.com?x=1", "https://evil.com/path"} {
		if got := clawbotRedirectBaseURL(value); got != "" {
			t.Fatalf("redirect %q should be rejected, got %q", value, got)
		}
	}
}

func serverURLForTest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
