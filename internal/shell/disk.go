package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yusif-v/mimir/internal/cases"
)

// tskImage is the Docker image Mimir's sleuthkit catalog template builds.
// Kept in sync with internal/catalog/templates/sleuthkit/mimir.toml.
const tskImage = "dfir-sleuthkit:latest"

// rrImage is the Docker image Mimir's regripper catalog template builds.
// Kept in sync with internal/catalog/templates/regripper/mimir.toml.
const rrImage = "dfir-regripper:latest"

// tskTools are the SleuthKit binaries the disk command accepts, mirroring
// the passthrough ENTRYPOINT of the dfir-sleuthkit image.
var tskTools = map[string]bool{
	"fls": true, "fsstat": true, "icat": true, "mmls": true,
	"blkcalc": true, "istat": true, "blkls": true, "blkcat": true,
	"dcat": true, "dstat": true, "ffind": true, "hfind": true,
	"ifind": true, "ils": true, "img_stat": true, "jcat": true,
	"jls": true, "sigfind": true, "sstat": true,
}

// cmdDisk runs a SleuthKit tool against a disk image as a first-class
// command — the disk-forensics counterpart of `volatility`. Usage:
//
//	disk <tool> [-i <image>] [tool args...]
//	sleuthkit <tool> [-i <image>] [tool args...]   (alias)
//
// The image is resolved from -i (relative to the case evidence dir or
// absolute), or auto-selected from evidence tagged "disk-image", else the
// most recent evidence item. The image is bind-mounted read-only into the
// dfir-sleuthkit container and `<tool> /evidence/<name> ...` runs. Stdout is
// printed, captured to a timeline event via captureInto, and scanned for
// IOCs plus tool-specific hints (partitions, deleted files).
func (a *App) cmdDisk(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case open — run 'case -o <name>' first")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: disk <tool> [-i <image>] [args...]\nTools: fls, fsstat, icat, mmls, blkcalc, istat, blkls, blkcat, dcat, dstat, ffind, hfind, ifind, ils, img_stat, jcat, jls, sigfind, sstat")
	}

	tool := args[0]
	if !tskTools[tool] {
		return fmt.Errorf("unknown sleuthkit tool: %s", tool)
	}

	// Split out -i <image>; the rest is passed straight to the tool.
	rest := args[1:]
	var imageName string
	for i := 0; i < len(rest); {
		switch {
		case rest[i] == "-i" && i+1 < len(rest):
			imageName = rest[i+1]
			rest = append(rest[:i], rest[i+2:]...)
		case strings.HasPrefix(rest[i], "-i="):
			imageName = strings.TrimPrefix(rest[i], "-i=")
			rest = append(rest[:i], rest[i+1:]...)
		default:
			i++
		}
	}

	imagePath, err := resolveEvidenceFile(c, imageName, "disk-image",
		"add a disk image with 'evidence add <path> --tag disk-image'")
	if err != nil {
		return err
	}
	base := filepath.Base(imagePath)
	containerPath := "/evidence/" + base

	if !dockerImageExists(tskImage) {
		return fmt.Errorf("image %s not found — run 'build sleuthkit' first (builds it from the catalog template)", tskImage)
	}

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none",
		"-v", imagePath + ":" + containerPath + ":ro",
		tskImage,
		tool,
		containerPath,
	}
	dockerArgs = append(dockerArgs, rest...)

	cmd := exec.Command("docker", dockerArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.String()
	command := tool + " " + base + " " + strings.Join(rest, " ")
	if runErr != nil {
		msg := out
		if msg == "" {
			msg = stderr.String()
		}
		fmt.Printf("%s[disk %s]%s\n%s\n", colorCyan, tool, colorReset, msg)
		return fmt.Errorf("%s: %v", tool, runErr)
	}

	fmt.Printf("%s[disk %s]%s\n%s\n", colorCyan, tool, colorReset, out)

	if err := captureInto(c, "sleuthkit", strings.TrimSpace(command), base, out); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s capture: %v\n", colorYellow, colorReset, err)
	}
	return nil
}

// resolveEvidenceFile returns the absolute path to an evidence file by name.
// If name is empty it auto-selects: prefer evidence tagged tag, else the most
// recent item. noHint is shown when the case has no evidence at all.
func resolveEvidenceFile(c *cases.Case, name, tag, noHint string) (string, error) {
	if name != "" {
		if filepath.IsAbs(name) {
			if _, err := os.Stat(name); err != nil {
				return "", fmt.Errorf("file not found: %s", name)
			}
			return name, nil
		}
		candidate := filepath.Join(c.Path, "evidence", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		entries, _ := os.ReadDir(filepath.Join(c.Path, "evidence"))
		for _, e := range entries {
			if e.Name() == name || strings.Contains(strings.ToLower(e.Name()), strings.ToLower(name)) {
				return filepath.Join(c.Path, "evidence", e.Name()), nil
			}
		}
		return "", fmt.Errorf("file not found in case evidence: %s", name)
	}

	evs := c.Evidence()
	if len(evs) == 0 {
		return "", fmt.Errorf("no evidence in case — %s", noHint)
	}
	var chosen *cases.Evidence
	for i := range evs {
		for _, t := range evs[i].Tags {
			if t == tag {
				chosen = &evs[i]
				break
			}
		}
		if chosen != nil {
			break
		}
	}
	if chosen == nil {
		// No tagged match: pick the most recently added evidence
		// (Evidence() is name-sorted, so compare AddedAt timestamps).
		for i := range evs {
			if chosen == nil || evs[i].AddedAt > chosen.AddedAt {
				chosen = &evs[i]
			}
		}
	}
	return filepath.Join(c.Path, "evidence", chosen.Name), nil
}

// cmdRegHives runs RegRipper against a Windows registry hive as a first-class
// command. Usage:
//
//	regripper [hive] [-p <plugin>] [--json]
//
// The hive is resolved positionally or via auto-selection (prefer evidence
// tagged "registry-hive"). Output is printed, captured to the timeline via
// captureInto, and scanned for IOCs plus registry-specific hints (USB
// devices, service binary paths, autostart entries).
func (a *App) cmdRegHives(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case open — run 'case -o <name>' first")
	}

	hiveName := ""
	plugin := ""
	outputJSON := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-p" && i+1 < len(args):
			plugin = args[i+1]
			i++
		case args[i] == "--json":
			outputJSON = true
		case !strings.HasPrefix(args[i], "-"):
			hiveName = args[i]
		}
	}

	hivePath, err := resolveEvidenceFile(c, hiveName, "registry-hive",
		"add a hive with 'evidence add <path> --tag registry-hive'")
	if err != nil {
		return err
	}
	base := filepath.Base(hivePath)

	if !dockerImageExists(rrImage) {
		return fmt.Errorf("image %s not found — run 'build regripper' first (builds it from the catalog template)", rrImage)
	}

	ripArgs := []string{"-r", "/evidence/" + base}
	if plugin != "" {
		ripArgs = append(ripArgs, "-p", plugin)
	}
	if outputJSON {
		ripArgs = append(ripArgs, "-j")
	}

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none",
		"-v", hivePath + ":/evidence/" + base + ":ro",
		rrImage,
	}
	dockerArgs = append(dockerArgs, ripArgs...)

	cmd := exec.Command("docker", dockerArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.String()
	command := "rip.pl -r " + base
	if plugin != "" {
		command += " -p " + plugin
	}
	if runErr != nil {
		msg := out
		if msg == "" {
			msg = stderr.String()
		}
		fmt.Printf("%s[regripper %s]%s\n%s\n", colorCyan, base, colorReset, msg)
		return fmt.Errorf("regripper %s: %v", base, runErr)
	}

	fmt.Printf("%s[regripper %s]%s\n%s\n", colorCyan, base, colorReset, out)

	if err := captureInto(c, "regripper", strings.TrimSpace(command), base, out); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s capture: %v\n", colorYellow, colorReset, err)
	}
	return nil
}

// diskHint is a SleuthKit-specific finding the generic IOC regex extractor
// would miss.
type diskHint struct {
	typ string
	val string
}

// diskHints extracts tool-specific findings from SleuthKit output:
//   - mmls: partition table rows -> "partition" hints (offset + description)
//   - fls: deleted entries (lines prefixed with '*') -> "deleted_file" hints
func diskHints(tool, out string) []diskHint {
	var hints []diskHint
	switch tool {
	case "mmls":
		// Typical row: "000:000    0    1    2048   2047999   NTFS / Partition 2"
		re := regexp.MustCompile(`(?m)^\s*\d+:\d+\s+(\d+)\s+(\d+)\s+(\d+)\s+(.+)$`)
		for _, m := range re.FindAllStringSubmatch(out, -1) {
			desc := strings.TrimSpace(m[4])
			hints = append(hints, diskHint{typ: "partition", val: fmt.Sprintf("start=%s end=%s len=%s %s", m[1], m[2], m[3], desc)})
		}
	case "fls":
		// Deleted entries are marked with '*' before the type flags:
		// "r/r * 12345: Documents/malware.exe"
		re := regexp.MustCompile(`(?m)^[^*\n]*\*[^:\n]*:\s*(.+)$`)
		for _, m := range re.FindAllStringSubmatch(out, -1) {
			hints = append(hints, diskHint{typ: "deleted_file", val: strings.TrimSpace(m[1])})
		}
	}
	return hints
}

// regHint is a RegRipper-specific finding.
type regHint struct {
	typ string
	val string
}

var (
	regUSBRe     = regexp.MustCompile(`USBSTOR\\[^\r\n"]{4,}`)
	regWinPathRe = regexp.MustCompile(`(?:[A-Za-z]:\\\\|[A-Za-z]:\\)[^\s"'<>|]{3,}\.(?:exe|dll|ps1|bat|cmd|vbs|js)`)
	regRunKeyRe  = regexp.MustCompile(`(?im)^[ \t]*(?:Run|RunOnce|RunOnceEx)(?:\b|\s*->)[ \t]*(.*)$`)
)

// regripperHints extracts registry-specific findings from RegRipper output:
//   - USBSTOR device keys -> "usb_device" (USB history)
//   - Executable paths referenced in hive data -> "winpath" (autoruns/services)
//   - Run/RunOnce key lines -> "autostart"
func regripperHints(out string) []regHint {
	var hints []regHint

	seenUSB := map[string]bool{}
	for _, m := range regUSBRe.FindAllStringSubmatch(out, -1) {
		v := strings.TrimRight(m[0], "\\")
		if !seenUSB[v] {
			seenUSB[v] = true
			hints = append(hints, regHint{typ: "usb_device", val: v})
		}
	}

	for _, m := range regWinPathRe.FindAllStringSubmatch(out, -1) {
		hints = append(hints, regHint{typ: "winpath", val: m[0]})
	}

	for _, m := range regRunKeyRe.FindAllStringSubmatch(out, -1) {
		t := strings.TrimSpace(m[0])
		if t != "" {
			hints = append(hints, regHint{typ: "autostart", val: truncate(t, 200)})
		}
	}
	return hints
}
