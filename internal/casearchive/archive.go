// Package casearchive exports and imports cases as portable tar.gz bundles.
package casearchive

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusif-v/mimir/internal/cases"
)

type Manifest struct {
	Name     string            `json:"name"`
	Exported string            `json:"exported"`
	Evidence map[string]string `json:"evidence_sha256"`
}

// Export writes caseDir to outPath as a gzipped tar with a manifest.json at the
// root. When includeOutput is false the output/ subtree is skipped.
func Export(caseDir, outPath string, includeOutput bool) error {
	tmp := outPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	name := filepath.Base(caseDir)
	man := Manifest{Name: name, Evidence: map[string]string{}}

	walkErr := filepath.Walk(caseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(caseDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if !includeOutput && (rel == "output" || strings.HasPrefix(rel, "output"+string(os.PathSeparator))) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(filepath.Join(name, rel))
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		if strings.HasPrefix(rel, "evidence"+string(os.PathSeparator)) {
			h := sha256.New()
			if _, err := io.Copy(io.MultiWriter(tw, h), src); err != nil {
				return err
			}
			man.Evidence[filepath.Base(rel)] = hex.EncodeToString(h.Sum(nil))
			return nil
		}
		_, err = io.Copy(tw, src)
		return err
	})
	if walkErr != nil {
		tw.Close()
		gz.Close()
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("archive case: %w", walkErr)
	}

	// manifest.json at archive root
	manBytes, _ := json.MarshalIndent(man, "", "  ")
	hdr := &tar.Header{Name: filepath.ToSlash(filepath.Join(name, "manifest.json")), Mode: 0644, Size: int64(len(manBytes))}
	if err := tw.WriteHeader(hdr); err == nil {
		tw.Write(manBytes)
	}

	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, outPath)
}

// ExportJSON writes a self-contained metadata document (no binary files).
func ExportJSON(c *cases.Case, outPath string) error {
	doc := map[string]any{
		"case":     c,
		"timeline": c.Timeline(),
		"evidence": c.Evidence(),
		"iocs":     c.IOCs(),
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, b, 0644)
}

// Import extracts an archive into casesDir. The case name comes from asName (if
// set) else the archive's top-level dir; it is auto-suffixed to avoid
// collisions. Evidence hashes are verified against manifest.json (warnings only).
// Returns the final case name.
func Import(archivePath, casesDir, asName string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// Peek the top-level dir name from the first header.
	headers := []*tar.Header{}
	var bodies [][]byte
	var topName string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar: %w", err)
		}
		parts := strings.SplitN(filepath.ToSlash(hdr.Name), "/", 2)
		if topName == "" {
			topName = parts[0]
		}
		var body []byte
		if hdr.Typeflag == tar.TypeReg {
			body, err = io.ReadAll(tr)
			if err != nil {
				return "", err
			}
		}
		headers = append(headers, hdr)
		bodies = append(bodies, body)
	}

	target := asName
	if target == "" {
		target = topName
	}
	target = uniqueName(casesDir, target)
	root := filepath.Join(casesDir, target)

	for i, hdr := range headers {
		rel := strings.SplitN(filepath.ToSlash(hdr.Name), "/", 2)
		if len(rel) < 2 {
			continue // the top dir itself
		}
		dest := filepath.Join(root, filepath.FromSlash(rel[1]))
		// zip-slip guard
		if !strings.HasPrefix(dest, filepath.Clean(root)+string(os.PathSeparator)) && dest != root {
			return "", fmt.Errorf("archive entry escapes target: %s", hdr.Name)
		}
		if hdr.Typeflag == tar.TypeDir {
			os.MkdirAll(dest, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(dest), 0755)
		if err := os.WriteFile(dest, bodies[i], 0644); err != nil {
			return "", err
		}
	}

	verifyEvidence(root)
	return target, nil
}

func verifyEvidence(root string) {
	manBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return
	}
	var man Manifest
	if json.Unmarshal(manBytes, &man) != nil {
		return
	}
	for name, want := range man.Evidence {
		path := filepath.Join(root, "evidence", name)
		f, err := os.Open(path)
		if err != nil {
			fmt.Printf("warning: evidence %q missing after import\n", name)
			continue
		}
		h := sha256.New()
		io.Copy(h, f)
		f.Close()
		if hex.EncodeToString(h.Sum(nil)) != want {
			fmt.Printf("warning: evidence %q hash mismatch after import\n", name)
		}
	}
}

func uniqueName(dir, name string) string {
	if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
		return name
	}
	base := name + "-imported"
	cand := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, cand)); os.IsNotExist(err) {
			return cand
		}
		cand = fmt.Sprintf("%s-%d", base, i)
	}
}
