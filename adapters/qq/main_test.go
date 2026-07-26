package qq

import "testing"

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
