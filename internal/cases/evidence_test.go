package cases

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceFold(t *testing.T) {
	c := newTempCase(t)
	if err := c.AppendEvidence(EvidenceRecord{Op: "add", Name: "a.bin", SHA256: "abc", Size: 10, Tags: []string{"malware"}, Time: "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := c.AppendEvidence(EvidenceRecord{Op: "tag", Name: "a.bin", Tags: []string{"packed"}, Time: "t2"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	ev := reloaded.Evidence()
	if len(ev) != 1 {
		t.Fatalf("want 1 evidence item, got %d", len(ev))
	}
	if ev[0].SHA256 != "abc" || ev[0].Size != 10 {
		t.Errorf("add fields lost: %+v", ev[0])
	}
	if len(ev[0].Tags) != 2 {
		t.Errorf("want folded tags [malware packed], got %v", ev[0].Tags)
	}
}

func TestEvidenceCorruptLineSkipped(t *testing.T) {
	c := newTempCase(t)
	if err := c.AppendEvidence(EvidenceRecord{Op: "add", Name: "a.bin", SHA256: "x", Time: "t1"}); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, c.Path, "evidence.jsonl", "{bad json\n")
	reloaded, err := LoadCase(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Evidence()) != 1 {
		t.Fatalf("corrupt line should be skipped, got %d", len(reloaded.Evidence()))
	}
}

func appendRaw(t *testing.T, dir, name, line string) {
	t.Helper()
	f, err := os_OpenAppend(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
}

func os_OpenAppend(dir, name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}
