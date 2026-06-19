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

func TestHashIndependence(t *testing.T) {
	// Three distinct hash strings on separate lines; all three must be extracted.
	sha256val := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sha1val := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	md5val := "d41d8cd98f00b204e9800998ecf8427e"
	data := []byte(sha256val + "\n" + sha1val + "\n" + md5val)
	inds := Extract(data)
	if !has(inds, "sha256", sha256val) {
		t.Errorf("missing sha256 %q in %v", sha256val, inds)
	}
	if !has(inds, "sha1", sha1val) {
		t.Errorf("missing sha1 %q in %v", sha1val, inds)
	}
	if !has(inds, "md5", md5val) {
		t.Errorf("missing md5 %q in %v", md5val, inds)
	}
}

func TestLoneSHA256NotAlsoHashedShorter(t *testing.T) {
	sha256val := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	inds := Extract([]byte(sha256val))
	sha256count, sha1count, md5count := 0, 0, 0
	for _, i := range inds {
		switch i.Type {
		case "sha256":
			sha256count++
		case "sha1":
			sha1count++
		case "md5":
			md5count++
		}
	}
	if sha256count != 1 {
		t.Errorf("want 1 sha256, got %d", sha256count)
	}
	if sha1count != 0 {
		t.Errorf("want 0 sha1, got %d (word-boundary anchors should prevent false match)", sha1count)
	}
	if md5count != 0 {
		t.Errorf("want 0 md5, got %d (word-boundary anchors should prevent false match)", md5count)
	}
}

func TestIPv6NoFalsePositiveOnSingleColon(t *testing.T) {
	inds := Extract([]byte("connect to host at 10.0.0.5:8080 now"))
	for _, i := range inds {
		if i.Type == "ipv6" {
			t.Errorf("unexpected ipv6 indicator %q for single-colon host:port text", i.Value)
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
