package qq

import (
	"net/http"
	"testing"
)

func TestValidOneBotAuthorization(t *testing.T) {
	tests := []struct {
		name  string
		auth  string
		token string
		want  bool
	}{
		{name: "empty token allows request", auth: "", token: "", want: true},
		{name: "raw token", auth: "secret", token: "secret", want: true},
		{name: "bearer token", auth: "Bearer secret", token: "secret", want: true},
		{name: "bearer token ignores extra whitespace", auth: "Bearer  secret  ", token: " secret ", want: true},
		{name: "substring is rejected", auth: "Bearer prefix-secret-suffix", token: "secret", want: false},
		{name: "wrong token is rejected", auth: "Bearer other", token: "secret", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validOneBotAuthorization(tt.auth, tt.token); got != tt.want {
				t.Fatalf("validOneBotAuthorization(%q, %q) = %v, want %v", tt.auth, tt.token, got, tt.want)
			}
		})
	}
}

func TestValidOneBotOrigin(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{name: "no origin allows server client", host: "127.0.0.1:8080", origin: "", want: true},
		{name: "same host", host: "bot.example.com", origin: "https://bot.example.com", want: true},
		{name: "localhost dev", host: "192.168.1.2:8080", origin: "http://localhost:5173", want: true},
		{name: "foreign origin rejected", host: "bot.example.com", origin: "https://evil.example.com", want: false},
		{name: "invalid origin rejected", host: "bot.example.com", origin: "://bad", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Host: tt.host, Header: http.Header{}}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := validOneBotOrigin(req); got != tt.want {
				t.Fatalf("validOneBotOrigin(%q, %q) = %v, want %v", tt.host, tt.origin, got, tt.want)
			}
		})
	}
}
