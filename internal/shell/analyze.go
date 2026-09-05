package shell

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/filetype"
	"github.com/yusif-v/mimir/internal/ioc"
	"github.com/yusif-v/mimir/internal/tools"
	"github.com/yusif-v/mimir/internal/ui"
)

// staticToolsFor maps a detected sample kind to the static tools that apply
// to it. yara runs against everything (a generic pattern match is always
// useful); capa only understands PE/ELF binaries; oletools only understands
// OLE/OOXML Office documents.
var staticToolsFor = map[filetype.Kind][]string{
	filetype.PE:      {"yara", "capa"},
	filetype.ELF:     {"yara", "capa"},
	filetype.OLE:     {"yara", "oletools"},
	filetype.OOXML:   {"yara", "oletools"},
	filetype.Unknown: {"yara"},
}

// staticToolNames returns the tool list for a filetype.Kind given as a raw
// string, so tests (and any future CLI surface) don't need to import
// internal/filetype just to look up the table.
func staticToolNames(kind string) []string {
	return staticToolsFor[filetype.Kind(kind)]
}

// errToolNotInstalled is a sentinel returned by runAnalyzeTool when toolName
// resolves to neither a builtin nor an installed catalog tool. Callers use
// errors.Is against this rather than matching on error text, so a tool's own
// stderr happening to contain the phrase "not installed" can never be
// misread as the skip condition.
var errToolNotInstalled = errors.New("tool not installed")

// analyzeToolResult is one row of the summary table cmdAnalyze prints at the
// end of a run.
type analyzeToolResult struct {
	tool         string
	status       string // ok | failed | skipped
	iocCount     int
	capabilities int
}

// buildToolArgs builds the argument list for one of the fixed static
// analysis tools. rulesContainerPath and containerPath are already
// container-mounted paths (of the form "/evidence/<name>") — analyze always
// runs its tools via Docker (see runAnalyzeTool), so every path handed to a
// tool must resolve inside the container, never on the host filesystem.
// rulesContainerPath is only used by yara and may be "" for capa/oletools.
func buildToolArgs(toolName, rulesContainerPath, containerPath string) []string {
	switch toolName {
	case "yara":
		return []string{rulesContainerPath, containerPath}
	case "capa":
		return []string{"-j", containerPath}
	case "oletools":
		return []string{"olevba", containerPath}
	default:
		return nil
	}
}

func (a *App) cmdAnalyze(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}

	var rulesArg, sample string
	for i := 0; i < len(args); i++ {
		if args[i] == "--rules" && i+1 < len(args) {
			rulesArg = args[i+1]
			i++
			continue
		}
		sample = args[i]
	}
	if sample == "" {
		return fmt.Errorf("usage: analyze <path> [--rules <path>]")
	}
	info, err := os.Stat(sample)
	if err != nil {
		return fmt.Errorf("stat %s: %w", sample, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", sample)
	}

	// Resolve the yara rules file to something that lives under
	// <case>/evidence/, since that's the directory actually bind-mounted to
	// /evidence in the container (see internal/catalog/templates/yara). The
	// default lookup location is <case>/evidence/rules.yar. An explicit
	// --rules path outside that directory is copied in under its basename
	// first — mirroring evidenceAdd's copy-in behavior — so yara always
	// receives a path that resolves inside the container.
	evidenceDir := filepath.Join(c.Path, "evidence")
	rulesContainerPath := ""
	switch {
	case rulesArg != "":
		rulesAbs, _ := filepath.Abs(rulesArg)
		evidenceDirAbs, _ := filepath.Abs(evidenceDir)
		rulesBase := filepath.Base(rulesArg)
		dest := filepath.Join(evidenceDir, rulesBase)
		if filepath.Dir(rulesAbs) != evidenceDirAbs {
			if err := copyFile(rulesArg, dest); err != nil {
				return fmt.Errorf("copy rules file into case evidence: %w", err)
			}
		}
		rulesContainerPath = "/evidence/" + rulesBase
	default:
		candidate := filepath.Join(evidenceDir, "rules.yar")
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			rulesContainerPath = "/evidence/rules.yar"
		}
	}

	// Register the sample as case evidence before analysis. If this exact
	// content is already registered (possibly under a different name than
	// this sample's basename — evidenceAdd dedupes by hash, not name), reuse
	// its existing name rather than re-adding, so containerPath below always
	// points at the file that's actually sitting in <case>/evidence/. A
	// hash mismatch under the same name is a genuine conflict and must stay
	// fatal — do not swallow it, or analyze would silently run tools against
	// the wrong file.
	sum, err := hashFile(sample)
	if err != nil {
		return fmt.Errorf("hash sample: %w", err)
	}
	name := filepath.Base(sample)
	if existing := c.FindEvidenceByHash(sum); existing != nil {
		name = existing.Name
	} else if err := a.evidenceAdd(c, []string{sample}); err != nil {
		return fmt.Errorf("register evidence: %w", err)
	}
	containerPath := "/evidence/" + name

	kind, err := filetype.Detect(sample)
	if err != nil {
		return fmt.Errorf("detect file type: %w", err)
	}
	fmt.Printf("%s: detected %s%s%s\n", name, colorCyan, kind, colorReset)

	var results []analyzeToolResult
	for _, toolName := range staticToolNames(string(kind)) {
		if toolName == "yara" && rulesContainerPath == "" {
			fmt.Fprintf(os.Stderr, "%swarning:%s no yara rules available (pass --rules or place rules.yar in <case>/evidence/) — skipping yara\n", colorYellow, colorReset)
			results = append(results, analyzeToolResult{tool: "yara", status: "skipped"})
			continue
		}

		toolArgs := buildToolArgs(toolName, rulesContainerPath, containerPath)

		out, runErr := a.runAnalyzeTool(toolName, toolArgs)
		if runErr != nil {
			if errors.Is(runErr, errToolNotInstalled) {
				fmt.Fprintf(os.Stderr, "%swarning:%s tool %q not installed — run: install %s\n", colorYellow, colorReset, toolName, toolName)
				results = append(results, analyzeToolResult{tool: toolName, status: "skipped"})
				continue
			}
			fmt.Fprintf(os.Stderr, "%swarning:%s %s failed: %v\n", colorYellow, colorReset, toolName, runErr)
			results = append(results, analyzeToolResult{tool: toolName, status: "failed"})
			continue
		}

		var hints []capaHint
		if toolName == "capa" {
			hints = capaHints(out)
			fmt.Printf("%s: %d capabilit%s matched\n", toolName, len(hints), pluralIES(len(hints)))
		} else {
			fmt.Print(out)
		}

		command := toolName + " " + strings.Join(toolArgs, " ")
		if err := captureInto(c, toolName, command, name, out); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s record %s output: %v\n", colorYellow, colorReset, toolName, err)
		}
		if _, err := a.Output.Record(toolName, out, c.Path); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s save %s output: %v\n", colorYellow, colorReset, toolName, err)
		}

		// Counts here reflect what THIS run's output actually contains, not
		// the case-wide AppendIOC dedup delta — re-running analyze on a
		// sample already in the case would otherwise report 0 for every
		// tool even though everything ran fine.
		res := analyzeToolResult{tool: toolName, status: "ok"}
		if toolName == "capa" {
			res.capabilities = len(hints)
		} else {
			res.iocCount = len(ioc.Extract([]byte(out)))
		}
		results = append(results, res)
	}

	printAnalyzeSummary(results)
	return nil
}

// runAnalyzeTool resolves toolName (builtin or installed catalog tool) and
// runs it, returning its stdout. analyze always builds container-mounted
// evidence paths (/evidence/<name>), so a tool run locally on the host
// (e.g. yara, which has a [local] cmd in its catalog template) would be
// handed a path that doesn't exist on the host filesystem at all. To keep
// execution consistent with the paths analyze constructs, any catalog tool
// is forced onto the Docker path here regardless of local-binary
// availability. A tool that isn't installed/registered returns
// errToolNotInstalled (wrapped) so callers can distinguish a missing tool
// (skip) from an execution failure (warn).
func (a *App) runAnalyzeTool(toolName string, args []string) (string, error) {
	if builtins.Has(toolName) {
		result := a.Runner.RunBuiltin(toolName, args)
		return resultOutput(result)
	}

	tool, ok := a.Tools.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool %q: %w", toolName, errToolNotInstalled)
	}
	runTool := *tool
	runTool.LocalCmd = "" // force Docker: see doc comment above.

	casePath := ""
	if c := a.Cases.Current(); c != nil {
		casePath = c.Path
	}
	result := a.Runner.Run(&runTool, args, casePath)
	return resultOutput(result)
}

// resultOutput unwraps a *tools.Result into (stdout, error), used by both
// the builtin and catalog-tool branches of runAnalyzeTool.
func resultOutput(result *tools.Result) (string, error) {
	if !result.Success() {
		if result.Error != nil {
			return result.Stdout, result.Error
		}
		return result.Stdout, fmt.Errorf("exit code %d: %s", result.ReturnCode, result.Stderr)
	}
	return result.Stdout, nil
}

func printAnalyzeSummary(results []analyzeToolResult) {
	tbl := ui.Table{
		Headers: []string{"TOOL", "STATUS", "IOCS FOUND", "CAPABILITIES FOUND"},
		Align:   []ui.Align{ui.AlignLeft, ui.AlignLeft, ui.AlignRight, ui.AlignRight},
	}
	for _, r := range results {
		iocs, caps := "-", "-"
		if r.status == "ok" {
			iocs = fmt.Sprintf("%d", r.iocCount)
			if r.tool == "capa" {
				caps = fmt.Sprintf("%d", r.capabilities)
				iocs = "-"
			}
		}
		tbl.Rows = append(tbl.Rows, []string{r.tool, r.status, iocs, caps})
	}
	tbl.Render(os.Stdout, ui.TermWidth(os.Stdout), !ui.IsTTY(os.Stdout))
}

func pluralIES(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
