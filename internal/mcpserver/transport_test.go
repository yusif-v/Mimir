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
		// Empty host binds all interfaces, so it must NOT be tokenless-loopback.
		{":7777", "", true},
		{":7777", "secret", false},
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

func TestNormalizeHTTPAddrRewritesEmptyHostToLoopback(t *testing.T) {
	// ":7777" would bind all interfaces; normalization pins it to loopback so
	// the tokenless convenience form is not a network-wide exposure.
	got, err := normalizeHTTPAddr(":7777")
	if err != nil {
		t.Fatalf("normalizeHTTPAddr(:7777): %v", err)
	}
	if got != "127.0.0.1:7777" {
		t.Fatalf("normalizeHTTPAddr(:7777) = %q, want 127.0.0.1:7777", got)
	}
	// End-to-end: normalized empty-host addr passes the auth gate tokenless.
	if err := requireLoopbackOrToken(got, ""); err != nil {
		t.Errorf("normalized :7777 should be tokenless-ok, got %v", err)
	}
	// An explicit all-interfaces bind stays untouched and still needs a token.
	if got, _ := normalizeHTTPAddr("0.0.0.0:7777"); got != "0.0.0.0:7777" {
		t.Errorf("normalizeHTTPAddr(0.0.0.0:7777) = %q, want unchanged", got)
	}
	if _, err := normalizeHTTPAddr("bogus"); err == nil {
		t.Error("normalizeHTTPAddr(bogus) should error on a missing port")
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
