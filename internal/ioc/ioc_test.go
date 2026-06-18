package ioc

import "testing"

func has(inds []Indicator, typ, val string) bool {
	for _, i := range inds {
		if i.Type == typ && i.Value == val {
			return true
		}
	}
	return false
}

func TestExtractTypes(t *testing.T) {
	data := []byte(`contact bad@actor.io from 185.12.34.56 visiting http://evil.example.com/x
also 2001:db8::1 and hash d41d8cd98f00b204e9800998ecf8427e
sha256 e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`)
	inds := Extract(data)
	checks := []struct{ typ, val string }{
		{"ipv4", "185.12.34.56"},
		{"email", "bad@actor.io"},
		{"url", "http://evil.example.com/x"},
		{"domain", "evil.example.com"},
		{"ipv6", "2001:db8::1"},
		{"md5", "d41d8cd98f00b204e9800998ecf8427e"},
		{"sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
	}
	for _, c := range checks {
		if !has(inds, c.typ, c.val) {
			t.Errorf("missing %s %q in %v", c.typ, c.val, inds)
		}
	}
}

func TestExtractDedupe(t *testing.T) {
	inds := Extract([]byte("1.2.3.4 1.2.3.4 1.2.3.4"))
	n := 0
	for _, i := range inds {
		if i.Type == "ipv4" && i.Value == "1.2.3.4" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want 1 deduped ipv4, got %d", n)
	}
}
