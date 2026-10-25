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

// volImage is the Docker image Mimir's volatility catalog template builds.
// Kept in sync with internal/catalog/templates/volatility/mimir.toml.
const volImage = "dfir-volatility:latest"

// cmdVolatility runs a Volatility 3 plugin against the current case's memory
// dump. It is the first-class memory-forensics command — no need to go through
// the generic sandbox. Usage:
//
//	memory <plugin> [-f <dump>] [plugin args...]
//
// The dump is resolved from -f (relative to the case evidence dir or absolute),
// or auto-selected from the most recent evidence tagged "memory-dump", else the
// first evidence item. The dump is bind-mounted read-only into the container and
// `vol <plugin> -f /evidence/<name> ...` is executed. Stdout is printed, captured
// to a timeline event, and scanned for IOCs.
func (a *App) cmdVolatility(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case open — run 'case open <name>' first")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: memory <plugin> [-f <dump>] [--offline] [args...]")
	}

	// Split out -f <dump> and --offline if present; the rest is the vol plugin + args.
	pluginArgs := args
	var dumpName string
	offline := false
	for i := 0; i < len(pluginArgs); {
		switch {
		case pluginArgs[i] == "-f" && i+1 < len(pluginArgs):
			dumpName = pluginArgs[i+1]
			pluginArgs = append(pluginArgs[:i], pluginArgs[i+2:]...)
		case strings.HasPrefix(pluginArgs[i], "-f="):
			dumpName = strings.TrimPrefix(pluginArgs[i], "-f=")
			pluginArgs = append(pluginArgs[:i], pluginArgs[i+1:]...)
		case pluginArgs[i] == "--offline":
			offline = true
			pluginArgs = append(pluginArgs[:i], pluginArgs[i+1:]...)
		default:
			i++
		}
	}
	if len(pluginArgs) == 0 {
		return fmt.Errorf("usage: memory <plugin> [-f <dump>] [--offline] [args...]")
	}

	dumpPath, err := resolveDump(c, dumpName)
	if err != nil {
		return err
	}

	base := filepath.Base(dumpPath)
	containerPath := "/evidence/" + base

	// Ensure the volatility image exists.
	if !dockerImageExists(volImage) {
		return fmt.Errorf("image %s not found — run 'tool build volatility' first (builds it from the catalog template)", volImage)
	}

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none",
		"-v", dumpPath + ":" + containerPath + ":ro",
		// The image's ENTRYPOINT is "vol" (see volatility/Dockerfile), so the
		// first positional arg is the plugin — do NOT pass "vol" again.
		volImage,
	}
	dockerArgs = append(dockerArgs, pluginArgs...)
	if offline {
		dockerArgs = append(dockerArgs, "-u") // no symbol download (airgapped/cached)
	}
	dockerArgs = append(dockerArgs, "-f", containerPath)

	cmd := exec.Command("docker", dockerArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out := stdout.String()
	if runErr != nil {
		msg := out
		if msg == "" {
			msg = stderr.String()
		}
		fmt.Printf("%s[volatility %s]%s\n%s\n", colorCyan, pluginArgs[0], colorReset, msg)
		return fmt.Errorf("vol %s: %v", pluginArgs[0], runErr)
	}

	fmt.Printf("%s[volatility %s]%s\n%s\n", colorCyan, pluginArgs[0], colorReset, out)

	// Capture to timeline + IOCs (shared with the generic sandbox
	// runner via captureInto — see its doc for the exact behavior).
	if err := captureInto(c, "volatility", "vol "+strings.Join(pluginArgs, " "), base, out); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s capture: %v\n", colorYellow, colorReset, err)
	}
	return nil
}

// volHint is a plugin-specific indicator (command / credential hash).
type volHint struct {
	typ string
	val string
}

// volatilityHints extracts plugin-specific findings from Volatility output
// that the generic IOC regex extractor would miss.
func volatilityHints(plugin, out string) []volHint {
	var hints []volHint
	low := strings.ToLower(plugin)
	switch {
	case strings.Contains(low, "cmdscan"), strings.Contains(low, "cmdline"),
		strings.Contains(low, "consoles"):
		// Capture executed commands (lines that look like a command invocation).
		for _, line := range strings.Split(out, "\n") {
			t := strings.TrimSpace(line)
			if t == "" {
				continue
			}
			// Heuristic: a prompt line ending in '>' or a path-like token.
			if strings.HasSuffix(t, ">") || strings.Contains(t, "\\") ||
				strings.HasPrefix(t, "C:") || strings.HasPrefix(t, "cmd.exe") {
				hints = append(hints, volHint{typ: "command", val: t})
			}
		}
	case strings.Contains(low, "hashdump"):
		// NTLM hashes: lines like "user:rid:lmhash:ntlmhash:::"
		re := regexp.MustCompile(`(?m)^[^\s:]+:\d+:[0-9a-fA-F]{32}:([0-9a-fA-F]{32}):`)
		for _, m := range re.FindAllStringSubmatch(out, -1) {
			hints = append(hints, volHint{typ: "ntlm", val: m[1]})
		}
	}
	return hints
}

// resolveDump returns the absolute path to the memory dump to analyze.
// If name is empty, it auto-selects from the case evidence.
func resolveDump(c *cases.Case, name string) (string, error) {
	if name != "" {
		// Absolute path wins; otherwise resolve under the case evidence dir.
		if filepath.IsAbs(name) {
			if _, err := os.Stat(name); err != nil {
				return "", fmt.Errorf("dump not found: %s", name)
			}
			return name, nil
		}
		candidate := filepath.Join(c.Path, "evidence", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		// fall back to treating name as a basename anywhere in evidence
		entries, _ := os.ReadDir(filepath.Join(c.Path, "evidence"))
		for _, e := range entries {
			if e.Name() == name || strings.Contains(strings.ToLower(e.Name()), strings.ToLower(name)) {
				return filepath.Join(c.Path, "evidence", e.Name()), nil
			}
		}
		return "", fmt.Errorf("dump not found in case evidence: %s", name)
	}

	// Auto-select: prefer evidence tagged "memory-dump", else most recent.
	var evs = c.Evidence()
	if len(evs) == 0 {
		return "", fmt.Errorf("no evidence in case — add a dump with 'evidence add <path> --tag memory-dump'")
	}
	var chosen *cases.Evidence
	for i := range evs {
		for _, t := range evs[i].Tags {
			if t == "memory-dump" {
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

// dockerImageExists reports whether a Docker image is present locally.
func dockerImageExists(image string) bool {
	cmd := exec.Command("docker", "image", "inspect", image)
	return cmd.Run() == nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
