package builtins

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Test fixtures ---

func writeFixture(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

// minimalPE creates a minimal valid PE32 file.
func minimalPE(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer

	// DOS header (64 bytes)
	dos := make([]byte, 64)
	copy(dos, "MZ")
	binary.LittleEndian.PutUint32(dos[60:], 64) // e_lfanew
	buf.Write(dos)

	// PE signature
	buf.WriteString("PE\x00\x00")

	// COFF header (20 bytes)
	coff := make([]byte, 20)
	binary.LittleEndian.PutUint16(coff[0:], 0x014c)   // Machine: i386
	binary.LittleEndian.PutUint16(coff[2:], 1)        // NumberOfSections
	binary.LittleEndian.PutUint32(coff[4:], 0x5A000000) // TimeDateStamp
	binary.LittleEndian.PutUint32(coff[8:], 0)        // PointerToSymbolTable
	binary.LittleEndian.PutUint32(coff[12:], 0)       // NumberOfSymbols
	binary.LittleEndian.PutUint16(coff[16:], 0x00e0)  // SizeOfOptionalHeader
	binary.LittleEndian.PutUint16(coff[18:], 0x0102)  // Characteristics
	buf.Write(coff)

	// Optional header PE32 (224 bytes = 96 fixed + 4 NumberOfRvaAndSizes + 128 DataDirectory[16])
	opt := make([]byte, 224)
	binary.LittleEndian.PutUint16(opt[0:], 0x10b)   // Magic: PE32
	opt[2] = 1                                       // MajorLinkerVersion
	binary.LittleEndian.PutUint32(opt[4:], 0x1000)  // SizeOfCode
	binary.LittleEndian.PutUint32(opt[16:], 0x1000) // AddressOfEntryPoint
	binary.LittleEndian.PutUint32(opt[20:], 0x1000) // BaseOfCode
	binary.LittleEndian.PutUint32(opt[24:], 0x2000) // BaseOfData
	binary.LittleEndian.PutUint32(opt[28:], 0x00400000) // ImageBase
	binary.LittleEndian.PutUint32(opt[32:], 0x1000) // SectionAlignment
	binary.LittleEndian.PutUint32(opt[36:], 0x200)  // FileAlignment
	binary.LittleEndian.PutUint16(opt[40:], 6)      // MajorOSVersion
	binary.LittleEndian.PutUint16(opt[44:], 0)      // MinorOSVersion
	binary.LittleEndian.PutUint16(opt[46:], 0)      // MajorImageVersion
	binary.LittleEndian.PutUint16(opt[48:], 0)      // MinorImageVersion
	binary.LittleEndian.PutUint16(opt[50:], 0)      // MajorSubsystemVersion
	binary.LittleEndian.PutUint16(opt[52:], 0)      // MinorSubsystemVersion
	binary.LittleEndian.PutUint32(opt[56:], 0x3000) // SizeOfImage
	binary.LittleEndian.PutUint32(opt[60:], 0x200)  // SizeOfHeaders
	binary.LittleEndian.PutUint32(opt[64:], 0)      // CheckSum
	binary.LittleEndian.PutUint16(opt[68:], 5)      // Subsystem: Windows CUI
	binary.LittleEndian.PutUint16(opt[70:], 0)      // DllCharacteristics
	binary.LittleEndian.PutUint32(opt[72:], 0x100000) // SizeOfStackReserve
	binary.LittleEndian.PutUint32(opt[76:], 0x1000) // SizeOfStackCommit
	binary.LittleEndian.PutUint32(opt[80:], 0x100000) // SizeOfHeapReserve
	binary.LittleEndian.PutUint32(opt[84:], 0x1000) // SizeOfHeapCommit
	binary.LittleEndian.PutUint32(opt[88:], 0)      // LoaderFlags
	binary.LittleEndian.PutUint32(opt[92:], 16)     // NumberOfRvaAndSizes = 16
	// DataDirectory[16] at offset 96, 128 bytes, all zeros
	buf.Write(opt)

	// Section table (40 bytes)
	sec := make([]byte, 40)
	copy(sec, ".text\x00\x00\x00")
	binary.LittleEndian.PutUint32(sec[8:], 0x200)    // VirtualSize
	binary.LittleEndian.PutUint32(sec[12:], 0x1000)  // VirtualAddress
	binary.LittleEndian.PutUint32(sec[16:], 0x200)   // SizeOfRawData
	binary.LittleEndian.PutUint32(sec[20:], 0x200)   // PointerToRawData
	binary.LittleEndian.PutUint32(sec[36:], 0x60000020) // Characteristics
	buf.Write(sec)

	// Pad to file size
	data := buf.Bytes()
	for len(data) < 0x400 {
		data = append(data, 0)
	}

	return writeFixture(t, "test.exe", data)
}

// minimalELF creates a minimal valid ELF64 file.
func minimalELF(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer

	// ELF header (64 bytes)
	buf.Write([]byte{0x7f, 'E', 'L', 'F'}) // Magic
	buf.WriteByte(2)                        // Class: 64-bit
	buf.WriteByte(1)                        // Data: little-endian
	buf.WriteByte(1)                        // Version
	buf.WriteByte(0)                        // OS/ABI
	buf.Write(make([]byte, 8))              // Padding
	binary.Write(&buf, binary.LittleEndian, uint16(2))    // Type: ET_EXEC
	binary.Write(&buf, binary.LittleEndian, uint16(0x3e)) // Machine: x86-64
	binary.Write(&buf, binary.LittleEndian, uint32(1))    // Version
	binary.Write(&buf, binary.LittleEndian, uint64(0x400078)) // Entry
	binary.Write(&buf, binary.LittleEndian, uint64(64))   // Phoff
	binary.Write(&buf, binary.LittleEndian, uint64(0))    // Shoff
	binary.Write(&buf, binary.LittleEndian, uint32(0))    // Flags
	binary.Write(&buf, binary.LittleEndian, uint16(64))   // Ehsize
	binary.Write(&buf, binary.LittleEndian, uint16(56))   // Phentsize
	binary.Write(&buf, binary.LittleEndian, uint16(1))    // Phnum
	binary.Write(&buf, binary.LittleEndian, uint16(0))    // Shentsize
	binary.Write(&buf, binary.LittleEndian, uint16(0))    // Shnum
	binary.Write(&buf, binary.LittleEndian, uint16(0))    // Shstrndx

	// Program header (56 bytes)
	binary.Write(&buf, binary.LittleEndian, uint32(1))       // Type: PT_LOAD
	binary.Write(&buf, binary.LittleEndian, uint32(5))       // Flags: PF_R|PF_X
	binary.Write(&buf, binary.LittleEndian, uint64(0))       // Offset
	binary.Write(&buf, binary.LittleEndian, uint64(0x400000)) // Vaddr
	binary.Write(&buf, binary.LittleEndian, uint64(0x400000)) // Paddr
	binary.Write(&buf, binary.LittleEndian, uint64(120))     // Filesz
	binary.Write(&buf, binary.LittleEndian, uint64(120))     // Memsz
	binary.Write(&buf, binary.LittleEndian, uint64(0x1000))  // Align

	// Code bytes
	buf.Write([]byte{0x48, 0x89, 0xf8, 0xc3}) // mov rax, rdi; ret

	return writeFixture(t, "test.elf", buf.Bytes())
}

// maliciousRTF creates an RTF with exploit indicators.
func maliciousRTF(t *testing.T) string {
	t.Helper()
	rtf := "{\\rtf1\\ansi\\deff0\n{\\object\\objemb\\objupdate{\\*\\objclass Equation.3}\\objw360\\objh120{\\*\\objdata\n01050000020000000B0000004571756174696F6E2E3300\n}}}\n}"
	return writeFixture(t, "evil.rtf", []byte(rtf))
}

// cleanRTF creates a benign RTF.
func cleanRTF(t *testing.T) string {
	t.Helper()
	return writeFixture(t, "clean.rtf", []byte("{\\rtf1\\ansi\\deff0\nHello World\n}"))
}

// ---------------------------------------------------------------------------
// peinfo tests
// ---------------------------------------------------------------------------

func TestPEInfoValidFile(t *testing.T) {
	p := minimalPE(t)
	out, err := Run("peinfo", []string{p})
	if err != nil {
		t.Fatalf("peinfo: %v", err)
	}
	if !strings.Contains(out, "PE32") {
		t.Errorf("expected PE32 in output:\n%s", out)
	}
	if !strings.Contains(out, "i386") {
		t.Errorf("expected i386 machine in output:\n%s", out)
	}
	if !strings.Contains(out, ".text") {
		t.Errorf("expected .text section in output:\n%s", out)
	}
	if !strings.Contains(out, "EXECUTABLE_IMAGE") {
		t.Errorf("expected EXECUTABLE_IMAGE flag in output:\n%s", out)
	}
}

func TestPEInfoNotPE(t *testing.T) {
	p := writeFixture(t, "notpe.txt", []byte("hello world"))
	_, err := Run("peinfo", []string{p})
	if err == nil {
		t.Fatal("expected error for non-PE file")
	}
}

func TestPEInfoMissingFile(t *testing.T) {
	_, err := Run("peinfo", []string{"/no/such/file"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// elfinfo tests
// ---------------------------------------------------------------------------

func TestELFInfoValidFile(t *testing.T) {
	p := minimalELF(t)
	out, err := Run("elfinfo", []string{p})
	if err != nil {
		t.Fatalf("elfinfo: %v", err)
	}
	if !strings.Contains(out, "x86-64") {
		t.Errorf("expected x86-64 in output:\n%s", out)
	}
	if !strings.Contains(out, "EXEC") {
		t.Errorf("expected EXEC type in output:\n%s", out)
	}
	if !strings.Contains(out, "ELFCLASS64") {
		t.Errorf("expected ELFCLASS64 in output:\n%s", out)
	}
}

func TestELFInfoNotELF(t *testing.T) {
	p := writeFixture(t, "notelf.txt", []byte("hello world"))
	_, err := Run("elfinfo", []string{p})
	if err == nil {
		t.Fatal("expected error for non-ELF file")
	}
}

func TestELFInfoMissingFile(t *testing.T) {
	_, err := Run("elfinfo", []string{"/no/such/file"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// rtfscan tests
// ---------------------------------------------------------------------------

func TestRTFScanMalicious(t *testing.T) {
	p := maliciousRTF(t)
	out, err := Run("rtfscan", []string{p})
	if err != nil {
		t.Fatalf("rtfscan: %v", err)
	}
	if !strings.Contains(out, "OBJDATA") {
		t.Errorf("expected OBJDATA detection:\n%s", out)
	}
	if !strings.Contains(out, "Equation") {
		t.Errorf("expected Equation detection:\n%s", out)
	}
	if !strings.Contains(out, "CVE-2017-11882") {
		t.Errorf("expected CVE-2017-11882 reference:\n%s", out)
	}
}

func TestRTFScanClean(t *testing.T) {
	p := cleanRTF(t)
	out, err := Run("rtfscan", []string{p})
	if err != nil {
		t.Fatalf("rtfscan: %v", err)
	}
	if !strings.Contains(out, "No obvious exploit") {
		t.Errorf("expected clean result:\n%s", out)
	}
}

func TestRTFScanNotRTF(t *testing.T) {
	p := writeFixture(t, "notrtf.txt", []byte("not an rtf file"))
	out, err := Run("rtfscan", []string{p})
	if err != nil {
		t.Fatalf("rtfscan: %v", err)
	}
	if !strings.Contains(out, "Not a valid RTF") {
		t.Errorf("expected not-rtf warning:\n%s", out)
	}
}

func TestRTFScanMissingFile(t *testing.T) {
	_, err := Run("rtfscan", []string{"/no/such/file"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// lnkparse tests
// ---------------------------------------------------------------------------

func TestLNKParseInvalidFile(t *testing.T) {
	p := writeFixture(t, "notlnk.txt", []byte("not a lnk file"))
	_, err := Run("lnkparse", []string{p})
	if err == nil {
		t.Fatal("expected error for non-LNK file")
	}
}

func TestLNKParseTooSmall(t *testing.T) {
	p := writeFixture(t, "small.bin", []byte{0x4C, 0x00, 0x00, 0x00, 0x01, 0x02})
	_, err := Run("lnkparse", []string{p})
	if err == nil {
		t.Fatal("expected error for too-small file")
	}
}

func TestLNKParseMissingFile(t *testing.T) {
	_, err := Run("lnkparse", []string{"/no/such/file"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// mimetype tests
// ---------------------------------------------------------------------------

func TestMimetypePE(t *testing.T) {
	p := minimalPE(t)
	out, err := Run("mimetype", []string{p})
	if err != nil {
		t.Fatalf("mimetype: %v", err)
	}
	if !strings.Contains(out, "PE/COFF") {
		t.Errorf("expected PE/COFF in output:\n%s", out)
	}
}

func TestMimetypeELF(t *testing.T) {
	p := minimalELF(t)
	out, err := Run("mimetype", []string{p})
	if err != nil {
		t.Fatalf("mimetype: %v", err)
	}
	if !strings.Contains(out, "ELF") {
		t.Errorf("expected ELF in output:\n%s", out)
	}
}

func TestMimetypeText(t *testing.T) {
	p := writeFixture(t, "test.txt", []byte("hello world\n"))
	out, err := Run("mimetype", []string{p})
	if err != nil {
		t.Fatalf("mimetype: %v", err)
	}
	if !strings.Contains(out, "text/plain") {
		t.Errorf("expected text/plain in output:\n%s", out)
	}
}

func TestMimetypeEmpty(t *testing.T) {
	p := writeFixture(t, "empty.bin", []byte{})
	out, err := Run("mimetype", []string{p})
	if err != nil {
		t.Fatalf("mimetype: %v", err)
	}
	if !strings.Contains(out, "empty file") {
		t.Errorf("expected empty file message:\n%s", out)
	}
}

func TestMimetypeMissingFile(t *testing.T) {
	_, err := Run("mimetype", []string{"/no/such/file"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// ---------------------------------------------------------------------------
// Registry tests for new builtins
// ---------------------------------------------------------------------------

func TestNewBuiltinsInRegistry(t *testing.T) {
	for _, name := range []string{"peinfo", "elfinfo", "rtfscan", "lnkparse", "mimetype"} {
		if !Has(name) {
			t.Errorf("Has(%q) = false", name)
		}
	}
}

func TestNewBuiltinsInList(t *testing.T) {
	tools := List()
	toolNames := map[string]bool{}
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"peinfo", "elfinfo", "rtfscan", "lnkparse", "mimetype"} {
		if !toolNames[name] {
			t.Errorf("List() missing %s", name)
		}
	}
	// Total should be 11 (6 original + 5 new)
	if len(tools) != 11 {
		t.Errorf("expected 11 builtins, got %d", len(tools))
	}
}
