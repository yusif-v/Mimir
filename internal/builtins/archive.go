package builtins

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	azip "github.com/alexmullins/zip"
	"github.com/bodgit/sevenzip"
)

// ---------------------------------------------------------------------------
// unzip — list or extract ZIP and 7-Zip archives, including password-protected
// (WinZip AES / method 99) archives.
// ---------------------------------------------------------------------------

const unzipUsage = `usage: unzip <archive> [-l | -x [dest]] [-p <password>]

List or extract a ZIP or 7-Zip archive.

  -l            list archive contents (default)
  -x [dest]     extract all entries into dest (default: "<archive>.extracted")
  -p <password> password for encrypted archives (WinZip AES, method 99, 7z)

Examples:
  unzip evidence.zip
  unzip -p secret evidence.zip
  unzip -x -p secret evidence.zip /cases/1/evidence`

func unzipTool(args []string) (string, error) {
	var archive, dest, password string
	extract := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-l", "--list":
			// default behavior
		case "-x", "--extract":
			extract = true
		case "-p", "--password":
			if i+1 >= len(args) {
				return "", fmt.Errorf("unzip: -p requires an argument")
			}
			i++
			password = args[i]
		case "-h", "--help":
			return unzipUsage, nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", fmt.Errorf("unzip: unknown option %q", args[i])
			}
			if archive == "" {
				archive = args[i]
			} else if extract && dest == "" {
				dest = args[i]
			} else {
				return "", fmt.Errorf("unzip: unexpected argument %q", args[i])
			}
		}
	}
	if archive == "" {
		return "", errors.New(unzipUsage)
	}
	if dest == "" {
		dest = archive + ".extracted"
	}

	kind, err := sniffArchive(archive)
	if err != nil {
		return "", err
	}

	switch kind {
	case "zip":
		if extract {
			return extractZip(archive, dest, password)
		}
		return listZip(archive)
	case "7z":
		if extract {
			return extract7z(archive, dest, password)
		}
		return list7z(archive, password)
	}
	return "", fmt.Errorf("unzip: unsupported archive kind %q", kind)
}

// sniffArchive classifies an archive by its magic bytes.
func sniffArchive(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var head [6]byte
	n, _ := io.ReadFull(f, head[:])
	b := head[:n]

	switch {
	case n >= 4 && b[0] == 'P' && b[1] == 'K' &&
		((b[2] == 0x03 && b[3] == 0x04) || (b[2] == 0x05 && b[3] == 0x06)):
		return "zip", nil
	case n >= 6 && string(b[:6]) == "7z\xbc\xaf\x27\x1c":
		return "7z", nil
	}
	return "", fmt.Errorf("unzip: %s is not a ZIP or 7-Zip archive", path)
}

// archiveEntry is a normalized listing row shared by the zip and 7z backends.
type archiveEntry struct {
	name      string
	size      uint64
	method    string
	encrypted bool
}

func formatEntries(path, kind string, entries []archiveEntry, note string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== %s Archive: %s ===\n", kind, path)
	if note != "" {
		fmt.Fprintf(&sb, "  %s\n", note)
	}
	enc := 0
	for _, e := range entries {
		if e.encrypted {
			enc++
		}
	}
	fmt.Fprintf(&sb, "  Entries: %d\n", len(entries))
	if enc > 0 {
		fmt.Fprintf(&sb, "  Encrypted: %d\n", enc)
	}
	sb.WriteString("  Name                                    Size        Method\n")
	for _, e := range entries {
		mark := ""
		if e.encrypted {
			mark = "  [encrypted]"
		}
		fmt.Fprintf(&sb, "  %-40s %12d  %s%s\n", e.name, e.size, e.method, mark)
	}
	return sb.String()
}

func zipMethodName(m uint16) string {
	switch m {
	case azip.Store:
		return "store"
	case azip.Deflate:
		return "deflate"
	default:
		return fmt.Sprintf("method-%d", m)
	}
}

// ---------------------------------------------------------------------------
// ZIP backend (alexmullins/zip, WinZip AES / method 99 capable)
// ---------------------------------------------------------------------------

func listZip(path string) (string, error) {
	zr, err := azip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("unzip: %v", err)
	}
	defer zr.Close()

	entries := make([]archiveEntry, 0, len(zr.File))
	for _, f := range zr.File {
		entries = append(entries, archiveEntry{
			name:      f.Name,
			size:      f.UncompressedSize64,
			method:    zipMethodName(f.Method),
			encrypted: f.IsEncrypted(),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return formatEntries(path, "ZIP", entries, ""), nil
}

func extractZip(path, dest, password string) (string, error) {
	zr, err := azip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("unzip: %v", err)
	}
	defer zr.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Extracting ZIP: %s ===\n", path)

	extracted := 0
	var warnings []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.IsEncrypted() {
			if password == "" {
				warnings = append(warnings, fmt.Sprintf("skipped %s: encrypted (provide a password with -p)", f.Name))
				continue
			}
			f.SetPassword(password)
		}

		target, err := safeJoin(dest, f.Name)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}

		rc, err := f.Open()
		if err != nil {
			if errors.Is(err, azip.ErrPassword) {
				warnings = append(warnings, fmt.Sprintf("failed %s: incorrect password", f.Name))
			} else {
				warnings = append(warnings, fmt.Sprintf("failed %s: %v", f.Name, err))
			}
			continue
		}
		if err := writeEntry(target, rc); err != nil {
			rc.Close()
			warnings = append(warnings, fmt.Sprintf("failed %s: %v", f.Name, err))
			continue
		}
		rc.Close()
		extracted++
	}

	return finishExtract(&sb, "ZIP", path, dest, extracted, warnings), nil
}

// ---------------------------------------------------------------------------
// 7-Zip backend (bodgit/sevenzip)
// ---------------------------------------------------------------------------

func list7z(path, password string) (string, error) {
	r, err := sevenzip.OpenReaderWithPassword(path, password)
	if err != nil {
		return "", fmt.Errorf("unzip: 7z: %v", err)
	}
	defer r.Close()

	entries := make([]archiveEntry, 0, len(r.File))
	for _, f := range r.File {
		entries = append(entries, archiveEntry{
			name:   f.Name,
			size:   f.UncompressedSize,
			method: "7z",
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	note := ""
	if password != "" {
		note = "Decrypting with the supplied password."
	}
	return formatEntries(path, "7-Zip", entries, note), nil
}

func extract7z(path, dest, password string) (string, error) {
	r, err := sevenzip.OpenReaderWithPassword(path, password)
	if err != nil {
		return "", fmt.Errorf("unzip: 7z: %v", err)
	}
	defer r.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== Extracting 7-Zip: %s ===\n", path)

	extracted := 0
	var warnings []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		target, err := safeJoin(dest, f.Name)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}

		rc, err := f.Open()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("failed %s: %v", f.Name, err))
			continue
		}
		if err := writeEntry(target, rc); err != nil {
			rc.Close()
			var re *sevenzip.ReadError
			if errors.As(err, &re) && re.Encrypted {
				warnings = append(warnings, fmt.Sprintf("failed %s: decryption error (wrong password?)", f.Name))
			} else {
				warnings = append(warnings, fmt.Sprintf("failed %s: %v", f.Name, err))
			}
			continue
		}
		rc.Close()
		extracted++
	}

	return finishExtract(&sb, "7-Zip", path, dest, extracted, warnings), nil
}

// finishExtract renders the extraction summary. It is only called once the
// archive has been fully walked, so the returned string always describes the
// result even when some entries failed.
func finishExtract(sb *strings.Builder, kind, path, dest string, extracted int, warnings []string) string {
	fmt.Fprintf(sb, "  Extracted %d files to %s\n", extracted, dest)
	for _, w := range warnings {
		fmt.Fprintf(sb, "  [!] %s\n", w)
	}
	if extracted == 0 && len(warnings) > 0 {
		fmt.Fprintf(sb, "  Result: nothing extracted (%d problem(s))\n", len(warnings))
	}
	return sb.String()
}

// writeEntry streams an archive entry into target via a temp file that is
// renamed into place only after a fully successful read. This prevents partial
// or empty files from being left behind when decryption fails partway through
// (e.g. a wrong password on an encrypted 7z).
func writeEntry(target string, rc io.Reader) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".unzip-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// safeJoin resolves an archive entry name under dest, rejecting traversal
// attempts (zip-slip) and absolute paths.
func safeJoin(dest, name string) (string, error) {
	name = filepath.Clean(strings.ReplaceAll(name, "\\", "/"))
	if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	target := filepath.Join(dest, name)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return target, nil
}
