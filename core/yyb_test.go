package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestYybGoCompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"ok": true}`)
		case "/accounts":
			fmt.Fprint(w, `[{"id":"1","openid":"o1"},{"id":"2","openid":"o2"},{"id":"3","openid":"o3"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	panel := YybPanel{Address: srv.URL}
	ctx := context.Background()

	count := countYybAccounts(ctx, panel.Address)
	if count != 3 {
		t.Fatalf("countYybAccounts = %d, want 3", count)
	}

	envelope, err := requestYybJSON(ctx, panel.Address, "/health", nil)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("health envelope code = %d, want 0", envelope.Code)
	}

	result, err := testYybPanel(panel)
	if err != nil {
		t.Fatalf("testYybPanel failed: %v", err)
	}
	if result.AccountCount != 3 {
		t.Fatalf("testYybPanel AccountCount = %d, want 3", result.AccountCount)
	}
	if result.Status != "online" {
		t.Fatalf("testYybPanel Status = %s, want online", result.Status)
	}
}
