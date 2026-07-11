package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yusif-v/mimir/internal/cases"
	"github.com/yusif-v/mimir/internal/ioc"
)

// volImage is the Docker image Mimir's volatility catalog template builds.
// Kept in sync with internal/catalog/templates/volatility/mimir.toml.
const volImage = "dfir-volatility:latest"

// cmdVolatility runs a Volatility 3 plugin against the current case's memory
// dump. It is the first-class memory-forensics command — no need to go through
// the generic sandbox. Usage:
//
//	volatility <plugin> [-f <dump>] [plugin args...]
//	mem       <plugin> [-f <dump>] [plugin args...]
//
// The dump is resolved from -f (relative to the case evidence dir or absolute),
// or auto-selected from the most recent evidence tagged "memory-dump", else the
// first evidence item. The dump is bind-mounted read-only into the container and
// `vol <plugin> -f /evidence/<name> ...` is executed. Stdout is printed, captured
// to a timeline event, and scanned for IOCs.
func (a *App) cmdVolatility(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case open — run 'case -o <name>' first")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: volatility <plugin> [-f <dump>] [args...]")
	}

	// Split out -f <dump> if present; the rest is the vol plugin + args.
	pluginArgs := args
	var dumpName string
	for i := 0; i < len(pluginArgs); i++ {
		if pluginArgs[i] == "-f" && i+1 < len(pluginArgs) {
			dumpName = pluginArgs[i+1]
			pluginArgs = append(pluginArgs[:i], pluginArgs[i+2:]...)
			break
		}
		if strings.HasPrefix(pluginArgs[i], "-f=") {
			dumpName = strings.TrimPrefix(pluginArgs[i], "-f=")
			pluginArgs = append(pluginArgs[:i], pluginArgs[i+1:]...)
			break
		}
	}
	if len(pluginArgs) == 0 {
		return fmt.Errorf("usage: volatility <plugin> [-f <dump>] [args...]")
	}

	dumpPath, err := resolveDump(c, dumpName)
	if err != nil {
		return err
	}

	base := filepath.Base(dumpPath)
	containerPath := "/evidence/" + base

	// Ensure the volatility image exists.
	if !dockerImageExists(volImage) {
		return fmt.Errorf("image %s not found — run 'build volatility' first (builds it from the catalog template)", volImage)
	}

	dockerArgs := []string{
		"run", "--rm",
		"--network", "none",
		"-v", dumpPath + ":" + containerPath + ":ro",
		volImage, "vol",
	}
	dockerArgs = append(dockerArgs, pluginArgs...)
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

	// Capture to timeline.
	now := time.Now().Format(time.RFC3339)
	if err := c.AppendEvent(cases.TimelineEvent{
		Type:      "tool_run",
		Timestamp: now,
		Payload: map[string]any{
			"tool":    "volatility",
			"command": "vol " + strings.Join(pluginArgs, " "),
			"dump":    base,
			"output":  truncate(out, 2000),
		},
	}); err != nil {
		fmt.Fprintf(os.Stderr, "%swarning:%s append timeline: %v\n", colorYellow, colorReset, err)
	}

	// Auto-extract IOCs from the output.
	inds := ioc.Extract([]byte(out))
	if len(inds) > 0 {
		for _, ind := range inds {
			if err := c.AppendIOC(cases.IOCRecord{
				Type:   ind.Type,
				Value:  ind.Value,
				Source: "volatility:" + pluginArgs[0] + " (" + base + ")",
				Time:   now,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "%swarning:%s append ioc: %v\n", colorYellow, colorReset, err)
				break
			}
		}
		fmt.Printf("%s%d IOC(s) extracted from output.%s\n", colorGreen, len(inds), colorReset)
	}
	return nil
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
		chosen = &evs[len(evs)-1] // most recent
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
