package cases

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindEvidenceByHash_Found(t *testing.T) {
	c := &Case{
		evidence: []Evidence{
			{Name: "file1.bin", SHA256: "abc123", AddedAt: "2025-01-01T00:00:00Z"},
			{Name: "file2.bin", SHA256: "def456", AddedAt: "2025-01-02T00:00:00Z"},
		},
	}

	got := c.FindEvidenceByHash("abc123")
	if got == nil {
		t.Fatal("expected to find evidence with hash abc123, got nil")
	}
	if got.Name != "file1.bin" {
		t.Errorf("name: got %q, want %q", got.Name, "file1.bin")
	}
}

func TestFindEvidenceByHash_NotFound(t *testing.T) {
	c := &Case{
		evidence: []Evidence{
			{Name: "file1.bin", SHA256: "abc123", AddedAt: "2025-01-01T00:00:00Z"},
		},
	}

	got := c.FindEvidenceByHash("nonexistent")
	if got != nil {
		t.Errorf("expected nil for nonexistent hash, got %+v", got)
	}
}

func TestFindEvidenceByHash_Empty(t *testing.T) {
	c := &Case{}

	got := c.FindEvidenceByHash("anything")
	if got != nil {
		t.Errorf("expected nil for empty evidence, got %+v", got)
	}
}

func TestFindEvidenceByHash_DuplicateHashes(t *testing.T) {
	now := time.Now().Format(time.RFC3339)
	c := &Case{
		evidence: []Evidence{
			{Name: "first.bin", SHA256: "samehash", AddedAt: now},
			{Name: "second.bin", SHA256: "samehash", AddedAt: now},
		},
	}

	got := c.FindEvidenceByHash("samehash")
	if got == nil {
		t.Fatal("expected to find evidence, got nil")
	}
	// Should return the first match
	if got.Name != "first.bin" {
		t.Errorf("expected first match, got %q", got.Name)
	}
}

func TestSearchOutput_Match(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "nmap.txt"), []byte("PORT STATE\n22/tcp open ssh\n80/tcp open http\n443/tcp open https\n"), 0644)

	c := &Case{Path: dir}
	results := c.SearchOutput("open")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].File != "output/nmap.txt" {
		t.Errorf("file: got %q, want %q", results[0].File, "output/nmap.txt")
	}
	if results[0].Line != 2 {
		t.Errorf("line: got %d, want %d", results[0].Line, 2)
	}
	if results[0].Content != "22/tcp open ssh" {
		t.Errorf("content: got %q, want %q", results[0].Content, "22/tcp open ssh")
	}
}

func TestSearchOutput_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "log.txt"), []byte("ERROR: something failed\nWARNING: check this\nerror: another one\n"), 0644)

	c := &Case{Path: dir}
	results := c.SearchOutput("error")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSearchOutput_NoMatch(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "out.txt"), []byte("nothing interesting here\n"), 0644)

	c := &Case{Path: dir}
	results := c.SearchOutput("zebra")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearchOutput_MissingOutputDir(t *testing.T) {
	dir := t.TempDir()
	// No output/ directory created

	c := &Case{Path: dir}
	results := c.SearchOutput("anything")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for missing output dir, got %d", len(results))
	}
}

func TestSearchOutput_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(filepath.Join(outputDir, "a.txt"), []byte("match me\n"), 0644)
	os.WriteFile(filepath.Join(outputDir, "b.txt"), []byte("match me too\n"), 0644)

	c := &Case{Path: dir}
	results := c.SearchOutput("match")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both files should be found
	files := map[string]bool{}
	for _, r := range results {
		files[r.File] = true
	}
	if !files["output/a.txt"] {
		t.Error("expected output/a.txt in results")
	}
	if !files["output/b.txt"] {
		t.Error("expected output/b.txt in results")
	}
}
