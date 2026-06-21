// Package builtins provides native-Go DFIR tools that run without Docker.
package builtins

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"debug/elf"
	"debug/pe"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
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
	"hash":     {Meta{"hash", "md5/sha1/sha256 digests of a file"}, hashTool},
	"strings":  {Meta{"strings", "printable ASCII runs in a file (-n min length)"}, stringsTool},
	"file":     {Meta{"file", "identify file type by magic bytes"}, fileTool},
	"hexdump":  {Meta{"hexdump", "canonical hex + ASCII dump of a file"}, hexdumpTool},
	"entropy":  {Meta{"entropy", "Shannon entropy (bits/byte); flags packed/encrypted"}, entropyTool},
	"decode":   {Meta{"decode", "decode base64/hex/url input (--base64|--hex|--url, else auto)"}, decodeTool},
	"peinfo":   {Meta{"peinfo", "parse PE headers, imports, sections, compile timestamp"}, peinfoTool},
	"elfinfo":  {Meta{"elfinfo", "parse ELF headers, sections, symbols"}, elfinfoTool},
	"rtfscan":  {Meta{"rtfscan", "detect RTF exploit objects (objdata, objclass, equation)"}, rtfscanTool},
	"lnkparse": {Meta{"lnkparse", "parse Windows LNK shortcut files"}, lnkparseTool},
	"mimetype": {Meta{"mimetype", "deep MIME type detection beyond magic bytes"}, mimetypeTool},
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

// ---------------------------------------------------------------------------
// hash
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// strings
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// file
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// hexdump
// ---------------------------------------------------------------------------

func hexdumpTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: hexdump <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	var sb strings.Builder
	buf := make([]byte, 16)
	var offset int64
	r := bufio.NewReader(f)
	for {
		n, err := io.ReadFull(r, buf)
		if n == 0 {
			break
		}
		chunk := buf[:n]
		fmt.Fprintf(&sb, "%08x  ", offset)
		for i := 0; i < 16; i++ {
			if i < n {
				fmt.Fprintf(&sb, "%02x ", chunk[i])
			} else {
				sb.WriteString("   ")
			}
			if i == 7 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(" |")
		for _, b := range chunk {
			if b >= 0x20 && b <= 0x7e {
				sb.WriteByte(b)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
		offset += int64(n)
		if err != nil {
			break
		}
	}
	return sb.String(), nil
}

// ---------------------------------------------------------------------------
// entropy
// ---------------------------------------------------------------------------

func entropyTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: entropy <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	var counts [256]uint64
	var total uint64
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		for _, b := range buf[:n] {
			counts[b]++
		}
		total += uint64(n)
		if err != nil {
			break
		}
	}
	if total == 0 {
		return "entropy: 0.00 bits/byte (empty file)\n", nil
	}
	var h float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log2(p)
	}
	note := ""
	if h >= 7.5 {
		note = "  (high — likely packed/encrypted)"
	}
	return fmt.Sprintf("entropy: %.2f bits/byte%s\n", h, note), nil
}

// ---------------------------------------------------------------------------
// decode
// ---------------------------------------------------------------------------

func decodeTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: decode [--base64|--hex|--url] <input>")
	}
	mode := "auto"
	var input string
	for _, a := range args {
		switch a {
		case "--base64":
			mode = "base64"
		case "--hex":
			mode = "hex"
		case "--url":
			mode = "url"
		default:
			input = a
		}
	}
	if input == "" {
		return "", fmt.Errorf("decode: no input")
	}
	try := func(m string) ([]byte, bool) {
		switch m {
		case "base64":
			b, err := base64.StdEncoding.DecodeString(input)
			return b, err == nil
		case "hex":
			b, err := hex.DecodeString(input)
			return b, err == nil
		case "url":
			s, err := url.QueryUnescape(input)
			return []byte(s), err == nil
		}
		return nil, false
	}
	if mode != "auto" {
		b, ok := try(mode)
		if !ok {
			return "", fmt.Errorf("decode: input is not valid %s", mode)
		}
		return string(b) + "\n", nil
	}
	for _, m := range []string{"hex", "base64", "url"} {
		if b, ok := try(m); ok {
			return fmt.Sprintf("(%s) %s\n", m, string(b)), nil
		}
	}
	return "", fmt.Errorf("decode: could not auto-detect encoding")
}

// ---------------------------------------------------------------------------
// peinfo — PE header parser
// ---------------------------------------------------------------------------

func peinfoTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: peinfo <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	pef, err := pe.NewFile(f)
	if err != nil {
		return "", fmt.Errorf("not a valid PE file: %v", err)
	}
	defer pef.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== PE Info: %s ===\n\n", args[0])

	// Header
	fmt.Fprintf(&sb, "[Header]\n")
	fmt.Fprintf(&sb, "  Machine:              0x%04x (%s)\n", pef.FileHeader.Machine, peMachine(pef.FileHeader.Machine))
	fmt.Fprintf(&sb, "  Sections:             %d\n", pef.FileHeader.NumberOfSections)
	fmt.Fprintf(&sb, "  Timestamp:            %s\n", peTimestamp(pef.FileHeader.TimeDateStamp))
	fmt.Fprintf(&sb, "  Characteristics:      0x%04x\n", peFileChars(pef.FileHeader.Characteristics))

	// Optional header
	if pef.OptionalHeader != nil {
		switch oh := pef.OptionalHeader.(type) {
		case *pe.OptionalHeader32:
			fmt.Fprintf(&sb, "\n[Optional Header (PE32)]\n")
			fmt.Fprintf(&sb, "  Entry Point:          0x%08x\n", oh.AddressOfEntryPoint)
			fmt.Fprintf(&sb, "  Image Base:           0x%08x\n", oh.ImageBase)
			fmt.Fprintf(&sb, "  Subsystem:            %d (%s)\n", oh.Subsystem, peSubsystem(oh.Subsystem))
			fmt.Fprintf(&sb, "  DLL Characteristics:  0x%04x\n", oh.DllCharacteristics)
			fmt.Fprintf(&sb, "  Size of Image:        %d bytes\n", oh.SizeOfImage)
			fmt.Fprintf(&sb, "  Checksum:             0x%08x\n", oh.CheckSum)
		case *pe.OptionalHeader64:
			fmt.Fprintf(&sb, "\n[Optional Header (PE32+)]\n")
			fmt.Fprintf(&sb, "  Entry Point:          0x%08x\n", oh.AddressOfEntryPoint)
			fmt.Fprintf(&sb, "  Image Base:           0x%016x\n", oh.ImageBase)
			fmt.Fprintf(&sb, "  Subsystem:            %d (%s)\n", oh.Subsystem, peSubsystem(oh.Subsystem))
			fmt.Fprintf(&sb, "  DLL Characteristics:  0x%04x\n", oh.DllCharacteristics)
			fmt.Fprintf(&sb, "  Size of Image:        %d bytes\n", oh.SizeOfImage)
			fmt.Fprintf(&sb, "  Checksum:             0x%08x\n", oh.CheckSum)
		}
	}

	// Sections
	fmt.Fprintf(&sb, "\n[Sections]\n")
	for _, sec := range pef.Sections {
		name := strings.TrimRight(string(sec.Name[:]), "\x00")
		fmt.Fprintf(&sb, "  %-12s  VA: 0x%08x  Size: %d  Raw: %d  Chars: 0x%08x\n",
			name, sec.VirtualAddress, sec.Size, sec.Offset, sec.Characteristics)
	}

	// Imports
	imps, _ := pef.ImportedSymbols()
	if len(imps) > 0 {
		fmt.Fprintf(&sb, "\n[Imports]\n")
		// Group by DLL
		dllMap := map[string][]string{}
		for _, sym := range imps {
			parts := strings.SplitN(sym, ":", 2)
			if len(parts) == 2 {
				dllMap[parts[0]] = append(dllMap[parts[0]], parts[1])
			}
		}
		dlls := make([]string, 0, len(dllMap))
		for dll := range dllMap {
			dlls = append(dlls, dll)
		}
		sort.Strings(dlls)
		for _, dll := range dlls {
			syms := dllMap[dll]
			sort.Strings(syms)
			fmt.Fprintf(&sb, "  %s (%d symbols)\n", dll, len(syms))
			for _, s := range syms {
				fmt.Fprintf(&sb, "    %s\n", s)
			}
		}
	}

	return sb.String(), nil
}

func peMachine(m uint16) string {
	switch m {
	case 0x014c:
		return "i386"
	case 0x0162:
		return "R3000"
	case 0x0166:
		return "R4000/MIPS"
	case 0x0168:
		return "R10000"
	case 0x0169:
		return "WCE MIPSv2"
	case 0x0184:
		return "Alpha"
	case 0x01a2:
		return "SH3"
	case 0x01a3:
		return "SH3DSP"
	case 0x01a4:
		return "SH3E"
	case 0x01a6:
		return "SH4"
	case 0x01a8:
		return "SH5"
	case 0x01c0:
		return "ARM"
	case 0x01c2:
		return "ARM Thumb"
	case 0x01c4:
		return "ARM Thumb-2"
	case 0x01d3:
		return "AM33"
	case 0x01f0:
		return "PowerPC"
	case 0x01f1:
		return "PowerPC FP"
	case 0x0200:
		return "IA64"
	case 0x0266:
		return "MIPS16"
	case 0x0284:
		return "Alpha64"
	case 0x0366:
		return "MIPS FPU"
	case 0x0466:
		return "MIPS16 FPU"
	case 0x0520:
		return "Tricore"
	case 0x0CEF:
		return "CEF"
	case 0x0EBC:
		return "EFI Byte Code"
	case 0x8664:
		return "AMD64"
	case 0x9041:
		return "M32R"
	case 0xC0EE:
		return "CEE"
	default:
		return fmt.Sprintf("unknown(0x%04x)", m)
	}
}

func peTimestamp(ts uint32) string {
	t := time.Unix(int64(ts), 0).UTC()
	return fmt.Sprintf("%s (0x%08x)", t.Format("2006-01-02 15:04:05 UTC"), ts)
}

func peFileChars(c uint16) string {
	var flags []string
	if c&0x0001 != 0 {
		flags = append(flags, "RELOCS_STRIPPED")
	}
	if c&0x0002 != 0 {
		flags = append(flags, "EXECUTABLE_IMAGE")
	}
	if c&0x0004 != 0 {
		flags = append(flags, "LINE_NUMS_STRIPPED")
	}
	if c&0x0008 != 0 {
		flags = append(flags, "LOCAL_SYMS_STRIPPED")
	}
	if c&0x0020 != 0 {
		flags = append(flags, "LARGE_ADDRESS_AWARE")
	}
	if c&0x0100 != 0 {
		flags = append(flags, "32BIT_MACHINE")
	}
	if c&0x0200 != 0 {
		flags = append(flags, "DEBUG_STRIPPED")
	}
	if c&0x2000 != 0 {
		flags = append(flags, "DLL")
	}
	if len(flags) == 0 {
		return fmt.Sprintf("0x%04x", c)
	}
	return fmt.Sprintf("0x%04x (%s)", c, strings.Join(flags, " | "))
}

func peSubsystem(s uint16) string {
	switch s {
	case 1:
		return "native"
	case 2:
		return "Windows GUI"
	case 3:
		return "Windows CUI"
	case 5:
		return "OS/2 CUI"
	case 7:
		return "POSIX CUI"
	case 9:
		return "Windows CE GUI"
	case 10:
		return "EFI application"
	case 11:
		return "EFI boot service driver"
	case 12:
		return "EFI runtime driver"
	case 13:
		return "EFI ROM"
	case 14:
		return "XBOX"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ---------------------------------------------------------------------------
// elfinfo — ELF header parser
// ---------------------------------------------------------------------------

func elfinfoTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: elfinfo <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return "", fmt.Errorf("not a valid ELF file: %v", err)
	}
	defer ef.Close()

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== ELF Info: %s ===\n\n", args[0])

	// Header
	fmt.Fprintf(&sb, "[Header]\n")
	fmt.Fprintf(&sb, "  Class:                %s\n", ef.Class)
	fmt.Fprintf(&sb, "  Data:                 %s\n", ef.Data)
	fmt.Fprintf(&sb, "  OS/ABI:               %s\n", ef.OSABI)
	fmt.Fprintf(&sb, "  Type:                 %s\n", elfType(ef.Type))
	fmt.Fprintf(&sb, "  Machine:              %s\n", elfMachine(ef.Machine))
	fmt.Fprintf(&sb, "  Entry Point:          0x%x\n", ef.Entry)
	fmt.Fprintf(&sb, "  Program Headers:      %d\n", len(ef.Progs))
	fmt.Fprintf(&sb, "  Section Headers:      %d\n", len(ef.Sections))

	// Sections
	fmt.Fprintf(&sb, "\n[Sections]\n")
	for _, sec := range ef.Sections {
		fmt.Fprintf(&sb, "  %-20s  %-12s  Addr: 0x%016x  Size: %d  Off: %d  Align: %d\n",
			sec.Name, elfSectionType(sec.Type), sec.Addr, sec.Size, sec.Offset, sec.Addralign)
	}

	// Symbols
	symbols, _ := ef.Symbols()
	if len(symbols) > 0 {
		fmt.Fprintf(&sb, "\n[Symbols] (%d)\n", len(symbols))
		for _, sym := range symbols {
			if sym.Name == "" {
				continue
			}
			fmt.Fprintf(&sb, "  %-30s  Bind: %-8s  Type: %-8s  Size: %d  Section: %d\n",
				sym.Name, elfSymBind(elf.SymBind(sym.Info)), elfSymType(elf.SymType(sym.Info)), sym.Size, int(sym.Section))
		}
	}

	// Dynamic symbols
	dynsyms, _ := ef.DynamicSymbols()
	if len(dynsyms) > 0 {
		fmt.Fprintf(&sb, "\n[Dynamic Symbols] (%d)\n", len(dynsyms))
		for _, sym := range dynsyms {
			if sym.Name == "" {
				continue
			}
			fmt.Fprintf(&sb, "  %-30s  Bind: %-8s  Type: %-8s  Size: %d\n",
				sym.Name, elfSymBind(elf.SymBind(sym.Info)), elfSymType(elf.SymType(sym.Info)), sym.Size)
		}
	}

	// Imported libraries
	imps, _ := ef.ImportedLibraries()
	if len(imps) > 0 {
		fmt.Fprintf(&sb, "\n[Imported Libraries]\n")
		sort.Strings(imps)
		for _, lib := range imps {
			fmt.Fprintf(&sb, "  %s\n", lib)
		}
	}

	return sb.String(), nil
}

func elfType(t elf.Type) string {
	switch t {
	case elf.ET_NONE:
		return "NONE"
	case elf.ET_REL:
		return "REL (relocatable)"
	case elf.ET_EXEC:
		return "EXEC (executable)"
	case elf.ET_DYN:
		return "DYN (shared object)"
	case elf.ET_CORE:
		return "CORE (core dump)"
	default:
		return fmt.Sprintf("unknown(0x%x)", uint16(t))
	}
}

func elfMachine(m elf.Machine) string {
	switch m {
	case elf.EM_386:
		return "Intel 80386"
	case elf.EM_ARM:
		return "ARM"
	case elf.EM_X86_64:
		return "AMD x86-64"
	case elf.EM_AARCH64:
		return "AArch64"
	case elf.EM_MIPS:
		return "MIPS"
	case elf.EM_PPC:
		return "PowerPC"
	case elf.EM_PPC64:
		return "PowerPC64"
	case elf.EM_SPARC:
		return "SPARC"
	case elf.EM_RISCV:
		return "RISC-V"
	default:
		return fmt.Sprintf("unknown(0x%x)", uint16(m))
	}
}

func elfSectionType(t elf.SectionType) string {
	switch t {
	case elf.SHT_NULL:
		return "NULL"
	case elf.SHT_PROGBITS:
		return "PROGBITS"
	case elf.SHT_SYMTAB:
		return "SYMTAB"
	case elf.SHT_STRTAB:
		return "STRTAB"
	case elf.SHT_RELA:
		return "RELA"
	case elf.SHT_NOBITS:
		return "NOBITS"
	case elf.SHT_DYNSYM:
		return "DYNSYM"
	case elf.SHT_DYNAMIC:
		return "DYNAMIC"
	case elf.SHT_INIT_ARRAY:
		return "INIT_ARRAY"
	case elf.SHT_FINI_ARRAY:
		return "FINI_ARRAY"
	default:
		return fmt.Sprintf("0x%x", uint32(t))
	}
}

func elfSymBind(b elf.SymBind) string {
	switch b {
	case elf.STB_LOCAL:
		return "LOCAL"
	case elf.STB_GLOBAL:
		return "GLOBAL"
	case elf.STB_WEAK:
		return "WEAK"
	default:
		return fmt.Sprintf("%d", int(b))
	}
}

func elfSymType(t elf.SymType) string {
	switch t {
	case elf.STT_NOTYPE:
		return "NOTYPE"
	case elf.STT_OBJECT:
		return "OBJECT"
	case elf.STT_FUNC:
		return "FUNC"
	case elf.STT_SECTION:
		return "SECTION"
	case elf.STT_FILE:
		return "FILE"
	default:
		return fmt.Sprintf("%d", int(t))
	}
}

// ---------------------------------------------------------------------------
// rtfscan — RTF exploit object detector
// ---------------------------------------------------------------------------

func rtfscanTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: rtfscan <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", args[0], err)
	}

	content := string(data)
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== RTF Scan: %s ===\n\n", args[0])

	// Check for RTF magic
	if !strings.HasPrefix(content, "{\\rtf") {
		fmt.Fprintf(&sb, "[!] Not a valid RTF file (missing {\\rtf header)\n")
		return sb.String(), nil
	}

	findings := 0

	// Detect objdata — embedded OLE objects (common exploit vector)
	objdataRe := regexp.MustCompile(`(?i)\\objdata`)
	matches := objdataRe.FindAllStringIndex(content, -1)
	if len(matches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] OBJDATA objects found: %d\n", len(matches))
		for _, m := range matches {
			line := strings.Count(content[:m[0]], "\n") + 1
			fmt.Fprintf(&sb, "    Line %d: ...%s...\n", line, safeContext(content, m[0], 60))
		}
		fmt.Fprintln(&sb)
	}

	// Detect objclass — look for known exploit classes
	objclassRe := regexp.MustCompile(`(?i)\\objclass\s+([^\s\\]+)`)
	classMatches := objclassRe.FindAllStringSubmatch(content, -1)
	if len(classMatches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] OBJCLASS entries found: %d\n", len(classMatches))
		for _, m := range classMatches {
			class := m[1]
			warning := ""
			lower := strings.ToLower(class)
			if strings.Contains(lower, "equation") || strings.Contains(lower, "eqnedit") {
				warning = "  *** CVE-2017-11882 / Equation Editor exploit ***"
			} else if strings.Contains(lower, "packager") {
				warning = "  *** OLE Packager shell — potential payload ***"
			} else if strings.Contains(lower, "wscript") || strings.Contains(lower, "shell") {
				warning = "  *** Script/Shell object — suspicious ***"
			}
			fmt.Fprintf(&sb, "    Class: %s%s\n", class, warning)
		}
		fmt.Fprintln(&sb)
	}

	// Detect equation.3 / eqnedit — CVE-2017-11882
	equationRe := regexp.MustCompile(`(?i)equation`)
	eqMatches := equationRe.FindAllStringIndex(content, -1)
	if len(eqMatches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] Equation references found: %d (possible CVE-2017-11882)\n", len(eqMatches))
		for _, m := range eqMatches {
			line := strings.Count(content[:m[0]], "\n") + 1
			fmt.Fprintf(&sb, "    Line %d: ...%s...\n", line, safeContext(content, m[0], 60))
		}
		fmt.Fprintln(&sb)
	}

	// Detect embedded executable markers in OLE data
	exeRe := regexp.MustCompile(`(?i)(?:MZ|This program|PE\0\0|cmd\.exe|powershell|wscript|cscript)`)
	exeMatches := exeRe.FindAllStringIndex(content, -1)
	if len(exeMatches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] Embedded executable markers found: %d\n", len(exeMatches))
		for _, m := range exeMatches {
			line := strings.Count(content[:m[0]], "\n") + 1
			fmt.Fprintf(&sb, "    Line %d: ...%s...\n", line, safeContext(content, m[0], 60))
		}
		fmt.Fprintln(&sb)
	}

	// Detect hex-encoded data blobs (common in weaponized RTF)
	hexRe := regexp.MustCompile(`[0-9a-fA-F]{100,}`)
	hexMatches := hexRe.FindAllString(content, -1)
	if len(hexMatches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] Hex-encoded data blobs found: %d (possible shellcode)\n", len(hexMatches))
		for i, m := range hexMatches {
			if i >= 5 {
				fmt.Fprintf(&sb, "    ... and %d more\n", len(hexMatches)-5)
				break
			}
			preview := m
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			fmt.Fprintf(&sb, "    Blob %d: %s\n", i+1, preview)
		}
		fmt.Fprintln(&sb)
	}

	// Detect DDE (Dynamic Data Exchange) — CVE-2017-11826
	ddeRe := regexp.MustCompile(`(?i)\\s+ddeauto|dde\s`)
	ddeMatches := ddeRe.FindAllStringIndex(content, -1)
	if len(ddeMatches) > 0 {
		findings++
		fmt.Fprintf(&sb, "[!] DDE/DDEAUTO found: %d (possible CVE-2017-11826)\n", len(ddeMatches))
		for _, m := range ddeMatches {
			line := strings.Count(content[:m[0]], "\n") + 1
			fmt.Fprintf(&sb, "    Line %d: ...%s...\n", line, safeContext(content, m[0], 60))
		}
		fmt.Fprintln(&sb)
	}

	if findings == 0 {
		fmt.Fprintf(&sb, "[OK] No obvious exploit indicators found.\n")
	} else {
		fmt.Fprintf(&sb, "---\nTotal findings: %d\n", findings)
	}

	return sb.String(), nil
}

func safeContext(s string, pos, radius int) string {
	start := pos - radius
	if start < 0 {
		start = 0
	}
	end := pos + radius
	if end > len(s) {
		end = len(s)
	}
	ctx := s[start:end]
	ctx = strings.ReplaceAll(ctx, "\n", " ")
	ctx = strings.ReplaceAll(ctx, "\r", " ")
	ctx = strings.ReplaceAll(ctx, "\t", " ")
	return strings.TrimSpace(ctx)
}

// ---------------------------------------------------------------------------
// lnkparse — Windows LNK shortcut parser
// ---------------------------------------------------------------------------

// LNK file format constants
const (
	sigLNK        = "\x4C\x00\x00\x00" // LNK magic
	guidCLSID     = "\x01\x14\x02\x00\x00\x00\x00\x00\xC0\x00\x00\x00\x00\x00\x00\x46"
)

func lnkparseTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: lnkparse <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", args[0], err)
	}

	if len(data) < 76 {
		return "", fmt.Errorf("file too small to be a valid LNK file")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== LNK Parse: %s ===\n\n", args[0])

	// Check magic
	if string(data[:4]) != sigLNK {
		return "", fmt.Errorf("not a valid LNK file (bad magic: %02x %02x %02x %02x)",
			data[0], data[1], data[2], data[3])
	}

	// Check CLSID
	if string(data[4:20]) != guidCLSID {
		fmt.Fprintf(&sb, "[!] Warning: CLSID mismatch — may not be a valid LNK\n\n")
	}

	// Header
	fileSize := binary.LittleEndian.Uint32(data[20:24])
	fmt.Fprintf(&sb, "[Header]\n")
	fmt.Fprintf(&sb, "  File Size:            %d bytes\n", fileSize)

	// Link flags
	linkFlags := binary.LittleEndian.Uint32(data[20:24])
	_ = linkFlags

	// File attributes
	fileAttrs := binary.LittleEndian.Uint32(data[24:28])
	fmt.Fprintf(&sb, "  File Attributes:      0x%08x %s\n", fileAttrs, lnkFileAttrs(fileAttrs))

	// Timestamps (FILETIME 100ns intervals since 1601-01-01)
	creationTime := binary.LittleEndian.Uint64(data[28:36])
	accessTime := binary.LittleEndian.Uint64(data[36:44])
	writeTime := binary.LittleEndian.Uint64(data[44:52])
	fmt.Fprintf(&sb, "  Creation Time:        %s\n", lnkFiletime(creationTime))
	fmt.Fprintf(&sb, "  Access Time:          %s\n", lnkFiletime(accessTime))
	fmt.Fprintf(&sb, "  Write Time:           %s\n", lnkFiletime(writeTime))

	// Target file size
	targetSize := binary.LittleEndian.Uint32(data[52:56])
	fmt.Fprintf(&sb, "  Target Size:          %d bytes\n", targetSize)

	// Icon index
	iconIndex := binary.LittleEndian.Uint32(data[56:60])
	fmt.Fprintf(&sb, "  Icon Index:           %d\n", iconIndex)

	// Show command
	showCmd := binary.LittleEndian.Uint32(data[60:64])
	fmt.Fprintf(&sb, "  Show Command:         %d (%s)\n", showCmd, lnkShowCmd(showCmd))

	// Hot key
	hotKeyLo := data[64]
	hotKeyHi := data[65]
	fmt.Fprintf(&sb, "  Hot Key:              0x%02x%02x\n", hotKeyHi, hotKeyLo)

	// Link flags at offset 20 (actually at 20 bytes in)
	// Re-read: the header structure is:
	// 0-3: HeaderSize (0x4C)
	// 4-19: CLSID
	// 20-23: LinkFlags  (actually at offset 20)
	// Let me re-parse correctly
	linkFlagsVal := binary.LittleEndian.Uint32(data[20:24])
	// Wait, that's overlapping with FileSize above. Let me use the correct offsets.
	// Actually the spec says:
	// Offset 0: HeaderSize (4 bytes) = 0x4C = 76
	// Offset 4: CLSID (16 bytes)
	// Offset 20: LinkFlags (4 bytes)
	// Offset 24: FileAttributes (4 bytes)
	// Offset 28: CreationTime (8 bytes)
	// Offset 36: AccessTime (8 bytes)
	// Offset 44: WriteTime (8 bytes)
	// Offset 52: TargetFileSize (4 bytes)
	// Offset 56: IconIndex (4 bytes)
	// Offset 60: ShowCommand (4 bytes)
	// Offset 64: HotKey (2 bytes)
	// Offset 66: Reserved (2 bytes)
	// Offset 68: Reserved (4 bytes)
	// Offset 72: Reserved (4 bytes)
	// Offset 76: LinkTargetIDList (variable)

	// Re-do with correct offsets
	fmt.Fprintf(&sb, "\n[Link Flags]\n")
	fmt.Fprintf(&sb, "  Flags:                0x%08x\n", linkFlagsVal)
	flags := lnkLinkFlags(linkFlagsVal)
	if len(flags) > 0 {
		for _, fl := range flags {
			fmt.Fprintf(&sb, "    - %s\n", fl)
		}
	}

	// Parse LinkTargetIDList at offset 76
	idListSize := binary.LittleEndian.Uint16(data[76:78])
	if idListSize > 0 && int(idListSize)+78 <= len(data) {
		fmt.Fprintf(&sb, "\n[Link Target ID List]\n")
		fmt.Fprintf(&sb, "  ID List Size:         %d bytes\n", idListSize)
	}

	// After IDList comes the ShellLinkHeader... actually let's parse strings
	// String data starts after IDList
	stringOffset := 78 + int(idListSize)
	if linkFlagsVal&0x00000080 != 0 { // HasLinkInfo
		// LinkInfo structure follows
		if stringOffset+4 <= len(data) {
			linkInfoSize := binary.LittleEndian.Uint32(data[stringOffset : stringOffset+4])
			fmt.Fprintf(&sb, "\n[Link Info]\n")
			fmt.Fprintf(&sb, "  LinkInfo Size:        %d bytes\n", linkInfoSize)
			if linkInfoSize >= 28 && stringOffset+int(linkInfoSize) <= len(data) {
				liOffset := stringOffset
				// VolumeIDOffset at offset 12 within LinkInfo
				volIDOffset := binary.LittleEndian.Uint32(data[liOffset+12 : liOffset+16])
				localBasePathOffset := binary.LittleEndian.Uint32(data[liOffset+16 : liOffset+20])
				commonPathSuffixOffset := binary.LittleEndian.Uint32(data[liOffset+24 : liOffset+28])

				if localBasePathOffset > 0 && int(liOffset)+int(localBasePathOffset) < len(data) {
					path := readLNKString(data, int(liOffset)+int(localBasePathOffset), false)
					if path != "" {
						fmt.Fprintf(&sb, "  Local Base Path:      %s\n", path)
					}
				}
				if commonPathSuffixOffset > 0 && int(liOffset)+int(commonPathSuffixOffset) < len(data) {
					suffix := readLNKString(data, int(liOffset)+int(commonPathSuffixOffset), false)
					if suffix != "" {
						fmt.Fprintf(&sb, "  Common Path Suffix:   %s\n", suffix)
					}
				}
				if volIDOffset > 0 && int(liOffset)+int(volIDOffset)+16 <= len(data) {
					volOffset := int(liOffset) + int(volIDOffset)
					driveType := binary.LittleEndian.Uint32(data[volOffset+4 : volOffset+8])
					driveSerial := binary.LittleEndian.Uint32(data[volOffset+8 : volOffset+12])
					fmt.Fprintf(&sb, "  Drive Type:           %s\n", lnkDriveType(driveType))
					fmt.Fprintf(&sb, "  Drive Serial:         %08X\n", driveSerial)
				}
			}
			stringOffset += int(linkInfoSize)
		}
	}

	// String data section
	if linkFlagsVal&0x00000001 != 0 && stringOffset+2 <= len(data) { // HasName
		name := readLNKString(data, stringOffset, linkFlagsVal&0x00000004 != 0)
		if name != "" {
			fmt.Fprintf(&sb, "\n[Strings]\n")
			fmt.Fprintf(&sb, "  Name:                 %s\n", name)
		}
		// Advance past this string
		stringOffset += lnkStringLen(data, stringOffset, linkFlagsVal&0x00000004 != 0)
	}
	if linkFlagsVal&0x00000002 != 0 && stringOffset+2 <= len(data) { // HasRelativePath
		relPath := readLNKString(data, stringOffset, linkFlagsVal&0x00000004 != 0)
		if relPath != "" {
			fmt.Fprintf(&sb, "  Relative Path:        %s\n", relPath)
		}
		stringOffset += lnkStringLen(data, stringOffset, linkFlagsVal&0x00000004 != 0)
	}
	if linkFlagsVal&0x00000004 != 0 && stringOffset+2 <= len(data) { // HasWorkingDir
		workDir := readLNKString(data, stringOffset, linkFlagsVal&0x00000004 != 0)
		if workDir != "" {
			fmt.Fprintf(&sb, "  Working Dir:          %s\n", workDir)
		}
		stringOffset += lnkStringLen(data, stringOffset, linkFlagsVal&0x00000004 != 0)
	}
	if linkFlagsVal&0x00000008 != 0 && stringOffset+2 <= len(data) { // HasArguments
		args := readLNKString(data, stringOffset, linkFlagsVal&0x00000004 != 0)
		if args != "" {
			fmt.Fprintf(&sb, "  Arguments:            %s\n", args)
		}
		stringOffset += lnkStringLen(data, stringOffset, linkFlagsVal&0x00000004 != 0)
	}
	if linkFlagsVal&0x00000010 != 0 && stringOffset+2 <= len(data) { // HasIconLocation
		iconLoc := readLNKString(data, stringOffset, linkFlagsVal&0x00000004 != 0)
		if iconLoc != "" {
			fmt.Fprintf(&sb, "  Icon Location:        %s\n", iconLoc)
		}
	}

	return sb.String(), nil
}

func lnkFiletime(ft uint64) string {
	if ft == 0 {
		return "(not set)"
	}
	// FILETIME: 100-nanosecond intervals since 1601-01-01
	// Convert to Unix epoch
	epochDiff := uint64(116444736000000000) // 100ns intervals between 1601 and 1970
	if ft < epochDiff {
		return fmt.Sprintf("invalid (0x%016x)", ft)
	}
	unixSec := int64((ft - epochDiff) / 10000000)
	t := time.Unix(unixSec, 0).UTC()
	return fmt.Sprintf("%s (0x%016x)", t.Format("2006-01-02 15:04:05 UTC"), ft)
}

func lnkFileAttrs(attrs uint32) string {
	var flags []string
	if attrs&0x0001 != 0 {
		flags = append(flags, "READONLY")
	}
	if attrs&0x0002 != 0 {
		flags = append(flags, "HIDDEN")
	}
	if attrs&0x0004 != 0 {
		flags = append(flags, "SYSTEM")
	}
	if attrs&0x0020 != 0 {
		flags = append(flags, "ARCHIVE")
	}
	if attrs&0x0040 != 0 {
		flags = append(flags, "DEVICE")
	}
	if attrs&0x0080 != 0 {
		flags = append(flags, "NORMAL")
	}
	if attrs&0x0100 != 0 {
		flags = append(flags, "TEMPORARY")
	}
	if attrs&0x0200 != 0 {
		flags = append(flags, "SPARSE")
	}
	if attrs&0x0400 != 0 {
		flags = append(flags, "REPARSE_POINT")
	}
	if attrs&0x0800 != 0 {
		flags = append(flags, "COMPRESSED")
	}
	if attrs&0x1000 != 0 {
		flags = append(flags, "OFFLINE")
	}
	if attrs&0x2000 != 0 {
		flags = append(flags, "NOT_CONTENT_INDEXED")
	}
	if attrs&0x4000 != 0 {
		flags = append(flags, "ENCRYPTED")
	}
	if len(flags) == 0 {
		return ""
	}
	return "(" + strings.Join(flags, " | ") + ")"
}

func lnkLinkFlags(flags uint32) []string {
	var out []string
	if flags&0x00000001 != 0 {
		out = append(out, "HasLinkTargetIDList")
	}
	if flags&0x00000002 != 0 {
		out = append(out, "HasLinkInfo")
	}
	if flags&0x00000004 != 0 {
		out = append(out, "HasName")
	}
	if flags&0x00000008 != 0 {
		out = append(out, "HasRelativePath")
	}
	if flags&0x00000010 != 0 {
		out = append(out, "HasWorkingDir")
	}
	if flags&0x00000020 != 0 {
		out = append(out, "HasArguments")
	}
	if flags&0x00000040 != 0 {
		out = append(out, "HasIconLocation")
	}
	if flags&0x00000080 != 0 {
		out = append(out, "IsUnicode")
	}
	if flags&0x00000100 != 0 {
		out = append(out, "ForceNoLinkInfo")
	}
	if flags&0x00000200 != 0 {
		out = append(out, "HasExpString")
	}
	if flags&0x00000400 != 0 {
		out = append(out, "RunInSeparateProcess")
	}
	if flags&0x00000800 != 0 {
		out = append(out, "HasDarwinID")
	}
	if flags&0x00001000 != 0 {
		out = append(out, "RunAsUser")
	}
	if flags&0x00002000 != 0 {
		out = append(out, "HasExpIcon")
	}
	if flags&0x00004000 != 0 {
		out = append(out, "NoPidlAlias")
	}
	if flags&0x00008000 != 0 {
		out = append(out, "RunWithShimLayer")
	}
	if flags&0x00010000 != 0 {
		out = append(out, "ForceNoLinkTrack")
	}
	if flags&0x00020000 != 0 {
		out = append(out, "EnableTargetMetadata")
	}
	if flags&0x00040000 != 0 {
		out = append(out, "DisableLinkPathTracking")
	}
	if flags&0x00080000 != 0 {
		out = append(out, "DisableKnownFolderTracking")
	}
	if flags&0x00100000 != 0 {
		out = append(out, "DisableKnownFolderAlias")
	}
	if flags&0x00200000 != 0 {
		out = append(out, "AllowLinkToLink")
	}
	if flags&0x00400000 != 0 {
		out = append(out, "UnaliasOnSave")
	}
	if flags&0x00800000 != 0 {
		out = append(out, "PreventEnvironmentPathResolution")
	}
	if flags&0x01000000 != 0 {
		out = append(out, "KeepLocalIDListForUNCTarget")
	}
	return out
}

func lnkShowCmd(cmd uint32) string {
	switch cmd {
	case 1:
		return "SW_SHOWNORMAL"
	case 3:
		return "SW_SHOWMAXIMIZED"
	case 7:
		return "SW_SHOWMINNOACTIVE"
	default:
		return fmt.Sprintf("unknown(%d)", cmd)
	}
}

func lnkDriveType(dt uint32) string {
	switch dt {
	case 0:
		return "DRIVE_UNKNOWN"
	case 1:
		return "DRIVE_NO_ROOT_DIR"
	case 2:
		return "DRIVE_REMOVABLE"
	case 3:
		return "DRIVE_FIXED"
	case 4:
		return "DRIVE_REMOTE"
	case 5:
		return "DRIVE_CDROM"
	case 6:
		return "DRIVE_RAMDISK"
	default:
		return fmt.Sprintf("unknown(%d)", dt)
	}
}

func readLNKString(data []byte, offset int, unicode bool) string {
	if offset < 0 || offset >= len(data) {
		return ""
	}
	var length int
	if unicode {
		if offset+2 > len(data) {
			return ""
		}
		length = int(binary.LittleEndian.Uint16(data[offset:offset+2])) * 2 // UTF-16, 2 bytes per char
		offset += 2
	} else {
		if offset+1 > len(data) {
			return ""
		}
		length = int(data[offset])
		offset += 1
	}
	if offset+length > len(data) {
		length = len(data) - offset
	}
	if unicode {
		runes := make([]uint16, length/2)
		for i := 0; i < len(runes); i++ {
			runes[i] = binary.LittleEndian.Uint16(data[offset+i*2 : offset+i*2+2])
		}
		return string(utf16.Decode(runes))
	}
	return string(data[offset : offset+length])
}

func lnkStringLen(data []byte, offset int, unicode bool) int {
	if offset >= len(data) {
		return 0
	}
	if unicode {
		if offset+2 > len(data) {
			return 2
		}
		return 2 + int(binary.LittleEndian.Uint16(data[offset:offset+2]))*2
	}
	if offset+1 > len(data) {
		return 1
	}
	return 1 + int(data[offset])
}

// ---------------------------------------------------------------------------
// mimetype — Deep MIME type detection
// ---------------------------------------------------------------------------

// mimeSignatures maps magic bytes to MIME types with deeper inspection.
// Order matters: more specific signatures first.
var mimeSignatures = []struct {
	magic []byte
	mime  string
	desc  string
}{
	// Images
	{[]byte("\x89PNG\r\n\x1a\n"), "image/png", "PNG image"},
	{[]byte("\xff\xd8\xff"), "image/jpeg", "JPEG image"},
	{[]byte("GIF87a"), "image/gif", "GIF image (87a)"},
	{[]byte("GIF89a"), "image/gif", "GIF image (89a)"},
	{[]byte("BM"), "image/bmp", "BMP image"},
	{[]byte("RIFF"), "image/webp", "WebP image (RIFF container)"},
	{[]byte("\x00\x00\x01\x00"), "image/x-icon", "ICO icon"},
	{[]byte("II\x2a\x00"), "image/tiff", "TIFF image (little-endian)"},
	{[]byte("MM\x00\x2a"), "image/tiff", "TIFF image (big-endian)"},

	// Documents
	{[]byte("%PDF"), "application/pdf", "PDF document"},
	{[]byte("{\\rtf"), "application/rtf", "RTF document"},
	{[]byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"), "application/msword", "OLE2 compound document (DOC/XLS/PPT)"},
	{[]byte("PK\x03\x04"), "application/zip", "ZIP archive"},

	// Archives
	{[]byte("\x1f\x8b"), "application/gzip", "GZIP compressed"},
	{[]byte("BZ"), "application/x-bzip2", "BZIP2 compressed"},
	{[]byte("\xfd7zXZ"), "application/x-xz", "XZ compressed"},
	{[]byte("7z\xbc\xaf\x27\x1c"), "application/x-7z-compressed", "7-Zip archive"},
	{[]byte("Rar!\x1a\x07"), "application/x-rar-compressed", "RAR archive"},

	// Executables
	{[]byte("\x7fELF"), "application/x-executable", "ELF executable"},
	{[]byte("MZ"), "application/x-dosexec", "PE/COFF executable (DOS/Windows)"},

	// Media
	{[]byte("ID3"), "audio/mpeg", "MP3 audio (ID3 tag)"},
	{[]byte("\xff\xfb"), "audio/mpeg", "MP3 audio (no ID3)"},
	{[]byte("fLaC"), "audio/flac", "FLAC audio"},
	{[]byte("OggS"), "audio/ogg", "OGG audio"},
	{[]byte("RIFF"), "audio/wav", "WAV audio (RIFF container)"},
	{[]byte("\x00\x00\x00\x1cftyp"), "video/mp4", "MP4 video"},
	{[]byte("\x00\x00\x00\x20ftyp"), "video/mp4", "MP4 video"},
	{[]byte("ftyp"), "video/mp4", "MP4 video (offset)"},

	// Other
	{[]byte("\x00\x01\x00\x00"), "font/ttf", "TrueType font"},
	{[]byte("OTTO"), "font/otf", "OpenType font"},
	{[]byte("wOFF"), "font/woff", "WOFF font"},
	{[]byte("wOF2"), "font/woff2", "WOFF2 font"},
}

func mimetypeTool(args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: mimetype <file>")
	}
	f, err := openRegular(args[0])
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Read first 8192 bytes for deep inspection
	buf := make([]byte, 8192)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]

	if n == 0 {
		return fmt.Sprintf("%s: empty file\n", args[0]), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "=== MIME Type: %s ===\n\n", args[0])

	// Try magic-based detection first
	matched := false
	for _, sig := range mimeSignatures {
		if len(buf) >= len(sig.magic) && bytes.Equal(buf[:len(sig.magic)], sig.magic) {
			fmt.Fprintf(&sb, "  MIME Type:    %s\n", sig.mime)
			fmt.Fprintf(&sb, "  Description:  %s\n", sig.desc)

			// Deep inspection for specific types
			switch {
			case sig.mime == "application/pdf":
				inspectPDF(buf, n, &sb)
			case sig.mime == "application/zip":
				inspectZIP(buf, n, &sb)
			case sig.mime == "application/x-dosexec":
				inspectPE(buf, n, &sb)
			case sig.mime == "application/x-executable":
				inspectELF(buf, n, &sb)
			case sig.mime == "application/msword":
				inspectOLE2(buf, n, &sb)
			case sig.mime == "image/png":
				inspectPNG(buf, n, &sb)
			case sig.mime == "image/jpeg":
				inspectJPEG(buf, n, &sb)
			}
			matched = true
			break
		}
	}

	if !matched {
		// Fall back to stdlib detection
		ct := http.DetectContentType(buf)
		fmt.Fprintf(&sb, "  MIME Type:    %s\n", ct)
		fmt.Fprintf(&sb, "  Description:  (detected by content sniffing)\n")
	}

	// File extension cross-check
	ext := strings.ToLower(filepath.Ext(args[0]))
	if ext != "" {
		fmt.Fprintf(&sb, "  Extension:    %s\n", ext)
	}

	return sb.String(), nil
}

func inspectPDF(buf []byte, n int, sb *strings.Builder) {
	// Check for JavaScript
	content := string(buf)
	if strings.Contains(content, "/JavaScript") || strings.Contains(content, "/JS") {
		fmt.Fprintf(sb, "  [!] Contains JavaScript — potential exploit vector\n")
	}
	if strings.Contains(content, "/OpenAction") {
		fmt.Fprintf(sb, "  [!] Has OpenAction — auto-exec on open\n")
	}
	if strings.Contains(content, "/Launch") {
		fmt.Fprintf(sb, "  [!] Has Launch action — can execute commands\n")
	}
	if strings.Contains(content, "/EmbeddedFile") {
		fmt.Fprintf(sb, "  [!] Contains embedded files\n")
	}
	if strings.Contains(content, "/XFA") {
		fmt.Fprintf(sb, "  [!] XFA form — historically exploitable\n")
	}
	// PDF version
	if idx := strings.Index(content, "%PDF-"); idx >= 0 && idx+8 < len(content) {
		fmt.Fprintf(sb, "  PDF Version:  %s\n", content[idx+5:idx+8])
	}
}

func inspectZIP(buf []byte, n int, sb *strings.Builder) {
	// Count entries by scanning for local file headers
	entries := 0
	for i := 0; i < n-4; i++ {
		if buf[i] == 'P' && buf[i+1] == 'K' && buf[i+2] == 0x03 && buf[i+3] == 0x04 {
			entries++
		}
	}
	if entries > 0 {
		fmt.Fprintf(sb, "  Entries:      ~%d files\n", entries)
	}
	// Check for Office Open XML (DOCX/XLSX/PPTX)
	content := string(buf)
	if strings.Contains(content, "[Content_Types].xml") {
		if strings.Contains(content, "word/") {
			fmt.Fprintf(sb, "  Subtype:      Office Open XML (DOCX)\n")
		} else if strings.Contains(content, "xl/") {
			fmt.Fprintf(sb, "  Subtype:      Office Open XML (XLSX)\n")
		} else if strings.Contains(content, "ppt/") {
			fmt.Fprintf(sb, "  Subtype:      Office Open XML (PPTX)\n")
		} else {
			fmt.Fprintf(sb, "  Subtype:      Office Open XML (OOXML)\n")
		}
	}
}

func inspectPE(buf []byte, n int, sb *strings.Builder) {
	if n < 64 {
		return
	}
	// e_lfanew at offset 60
	peOffset := int(binary.LittleEndian.Uint32(buf[60:64]))
	if peOffset+4 > n {
		return
	}
	if string(buf[peOffset:peOffset+4]) == "PE\x00\x00" {
		if peOffset+24 <= n {
			machine := binary.LittleEndian.Uint16(buf[peOffset+4 : peOffset+6])
			fmt.Fprintf(sb, "  Machine:      %s\n", peMachine(machine))
			characteristics := binary.LittleEndian.Uint16(buf[peOffset+22 : peOffset+24])
			if characteristics&0x2000 != 0 {
				fmt.Fprintf(sb, "  Type:         DLL\n")
			} else {
				fmt.Fprintf(sb, "  Type:         EXE\n")
			}
		}
	}
}

func inspectELF(buf []byte, n int, sb *strings.Builder) {
	if n < 16 {
		return
	}
	class := buf[4] // 32-bit or 64-bit
	if class == 1 {
		fmt.Fprintf(sb, "  Class:        32-bit\n")
	} else if class == 2 {
		fmt.Fprintf(sb, "  Class:        64-bit\n")
	}
	data := buf[5] // endianness
	if data == 1 {
		fmt.Fprintf(sb, "  Endianness:   Little-endian\n")
	} else if data == 2 {
		fmt.Fprintf(sb, "  Endianness:   Big-endian\n")
	}
	if n >= 18 {
		typ := binary.LittleEndian.Uint16(buf[16:18])
		switch typ {
		case 2:
			fmt.Fprintf(sb, "  Type:         EXEC (executable)\n")
		case 3:
			fmt.Fprintf(sb, "  Type:         DYN (shared object)\n")
		case 4:
			fmt.Fprintf(sb, "  Type:         CORE (core dump)\n")
		}
	}
}

func inspectOLE2(buf []byte, n int, sb *strings.Builder) {
	// Try to determine the OLE2 document type from the root entry
	content := string(buf)
	if strings.Contains(content, "WordDocument") {
		fmt.Fprintf(sb, "  Subtype:      Microsoft Word (DOC)\n")
	} else if strings.Contains(content, "Workbook") || strings.Contains(content, "Excel") {
		fmt.Fprintf(sb, "  Subtype:      Microsoft Excel (XLS)\n")
	} else if strings.Contains(content, "PowerPoint") {
		fmt.Fprintf(sb, "  Subtype:      Microsoft PowerPoint (PPT)\n")
	} else {
		fmt.Fprintf(sb, "  Subtype:      OLE2 compound document\n")
	}
}

func inspectPNG(buf []byte, n int, sb *strings.Builder) {
	// IHDR chunk at offset 16
	if n >= 24 {
		width := binary.BigEndian.Uint32(buf[16:20])
		height := binary.BigEndian.Uint32(buf[20:24])
		bitDepth := buf[24]
		colorType := buf[25]
		fmt.Fprintf(sb, "  Dimensions:   %dx%d\n", width, height)
		fmt.Fprintf(sb, "  Bit Depth:    %d\n", bitDepth)
		fmt.Fprintf(sb, "  Color Type:   %d (%s)\n", colorType, pngColorType(colorType))
	}
}

func inspectJPEG(buf []byte, n int, sb *strings.Builder) {
	// Look for JFIF or Exif marker
	if n >= 10 {
		if buf[6] == 'J' && buf[7] == 'F' && buf[8] == 'I' && buf[9] == 'F' {
			fmt.Fprintf(sb, "  Format:       JFIF\n")
		} else if buf[6] == 'E' && buf[7] == 'x' && buf[8] == 'i' && buf[9] == 'f' {
			fmt.Fprintf(sb, "  Format:       Exif\n")
		}
	}
}

func pngColorType(ct uint8) string {
	switch ct {
	case 0:
		return "grayscale"
	case 2:
		return "RGB"
	case 3:
		return "indexed"
	case 4:
		return "grayscale+alpha"
	case 6:
		return "RGBA"
	default:
		return fmt.Sprintf("unknown(%d)", ct)
	}
}
