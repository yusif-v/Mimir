// Package builtins provides native-Go DFIR tools that run without Docker.
package builtins

import (
	"bufio"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
)

// Fn is a built-in tool implementation.
type Fn func(args []string) (string, error)

// Meta describes a built-in tool for listing.
type Meta struct {
	Name        string
	Description string
}

type entry struct {
	meta Meta
	fn   Fn
}

var registry = map[string]entry{
	"hash":    {Meta{"hash", "md5/sha1/sha256 digests of a file"}, hashTool},
	"strings": {Meta{"strings", "printable ASCII runs in a file (-n min length)"}, stringsTool},
	"file":    {Meta{"file", "identify file type by magic bytes"}, fileTool},
}

// Has reports whether name is a built-in tool.
func Has(name string) bool {
	_, ok := registry[name]
	return ok
}

// Run executes a built-in tool by name.
func Run(name string, args []string) (string, error) {
	e, ok := registry[name]
	if !ok {
		return "", fmt.Errorf("unknown built-in tool: %s", name)
	}
	return e.fn(args)
}

// List returns built-in tool metadata sorted by name.
func List() []Meta {
	out := make([]Meta, 0, len(registry))
	for _, e := range registry {
		out = append(out, e.meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// openRegular validates the path is an existing regular file and opens it.
func openRegular(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.Open(path)
}

func hashTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: hash <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	md5h, sha1h, sha256h := md5.New(), sha1.New(), sha256.New()
	if _, err := io.Copy(io.MultiWriter(md5h, sha1h, sha256h), f); err != nil {
		return "", fmt.Errorf("read %s: %w", args[0], err)
	}
	return fmt.Sprintf("MD5    %x\nSHA1   %x\nSHA256 %x\n",
		md5h.Sum(nil), sha1h.Sum(nil), sha256h.Sum(nil)), nil
}

func stringsTool(args []string) (string, error) {
	minLen := 4
	var path string
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			n := 0
			if _, err := fmt.Sscanf(args[i+1], "%d", &n); err != nil || n < 1 {
				return "", fmt.Errorf("invalid -n value: %s", args[i+1])
			}
			minLen = n
			i++
			continue
		}
		path = args[i]
	}
	if path == "" {
		return "", fmt.Errorf("usage: strings [-n N] <file>")
	}
	f, err := openRegular(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var out, run []byte
	r := bufio.NewReader(f)
	flush := func() {
		if len(run) >= minLen {
			out = append(out, run...)
			out = append(out, '\n')
		}
		run = run[:0]
	}
	for {
		b, err := r.ReadByte()
		if err != nil {
			break
		}
		if b >= 0x20 && b <= 0x7e {
			run = append(run, b)
		} else {
			flush()
		}
	}
	flush()
	return string(out), nil
}

func fileTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: file <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]

	if t := sniffMagic(head); t != "" {
		return fmt.Sprintf("%s: %s\n", args[0], t), nil
	}
	return fmt.Sprintf("%s: %s\n", args[0], http.DetectContentType(head)), nil
}

// sniffMagic returns a human label for known signatures, or "" if unknown.
func sniffMagic(b []byte) string {
	switch {
	case len(b) >= 4 && string(b[:4]) == "\x7fELF":
		return "ELF executable"
	case len(b) >= 2 && b[0] == 'M' && b[1] == 'Z':
		return "PE executable (MS-DOS/Windows)"
	case len(b) >= 4 && b[0] == 0xCF && b[1] == 0xFA && b[2] == 0xED && b[3] == 0xFE:
		return "Mach-O executable"
	case len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n":
		return "PNG image"
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "JPEG image"
	case len(b) >= 4 && string(b[:4]) == "%PDF":
		return "PDF document"
	case len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && b[2] == 0x03 && b[3] == 0x04:
		return "ZIP archive"
	case len(b) >= 2 && b[0] == 0x1F && b[1] == 0x8B:
		return "GZIP compressed data"
	}
	return ""
}
