package clawbot

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildClientVersion(t *testing.T) {
	if got := buildClientVersion("2.4.6"); got != 132102 {
		t.Fatalf("unexpected client version: %d", got)
	}
	if got := buildClientVersion("1.0.11"); got != 65547 {
		t.Fatalf("unexpected client version: %d", got)
	}
}

func TestRandomWechatUin(t *testing.T) {
	value := randomWechatUin()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("uin should be base64: %v", err)
	}
	if strings.TrimSpace(string(decoded)) == "" {
		t.Fatal("uin payload should not be empty")
	}
}

func TestMessageText(t *testing.T) {
	msg := weixinMessage{
		ItemList: []messageItem{
			{Type: messageItemText, TextItem: textItem{Text: "你好"}},
			{Type: 2},
			{Type: messageItemText, TextItem: textItem{Text: "世界"}},
		},
	}
	if got := messageText(msg); got != "你好\n世界" {
		t.Fatalf("unexpected message text: %q", got)
	}
}

func TestStripUnsupportedCQ(t *testing.T) {
	if got := stripUnsupportedCQ("文字[CQ:image,file=a.png]继续"); got != "文字继续" {
		t.Fatalf("unexpected stripped text: %q", got)
	}
}
