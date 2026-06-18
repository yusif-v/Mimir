package cases

import "testing"

func TestIOCDedupe(t *testing.T) {
	c := newTempCase(t)
	for i := 0; i < 2; i++ {
		if err := c.AppendIOC(IOCRecord{Type: "ipv4", Value: "1.2.3.4", Source: "f", Time: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.AppendIOC(IOCRecord{Type: "domain", Value: "evil.com", Source: "f", Time: "t"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.IOCs()) != 2 {
		t.Fatalf("want 2 deduped IOCs, got %d", len(reloaded.IOCs()))
	}
}
