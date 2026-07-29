package mcpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireLoopbackOrToken(t *testing.T) {
	cases := []struct {
		addr, token string
		wantErr     bool
	}{
		{"127.0.0.1:7777", "", false},
		{"localhost:7777", "", false},
		{":7777", "", false},
		{"0.0.0.0:7777", "", true},
		{"0.0.0.0:7777", "secret", false},
		{"192.168.1.5:7777", "", true},
	}
	for _, c := range cases {
		err := requireLoopbackOrToken(c.addr, c.token)
		if (err != nil) != c.wantErr {
			t.Errorf("addr=%s token=%q: err=%v want %v", c.addr, c.token, err, c.wantErr)
		}
	}
}

func TestBearerAuthRejectsMissingToken(t *testing.T) {
	h := bearerAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec2.Code)
	}
}
