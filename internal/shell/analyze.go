package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusif-v/mimir/internal/builtins"
	"github.com/yusif-v/mimir/internal/filetype"
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

// analyzeToolResult is one row of the summary table cmdAnalyze prints at the
// end of a run.
type analyzeToolResult struct {
	tool         string
	status       string // ok | failed | skipped
	iocCount     int
	capabilities int
}

func (a *App) cmdAnalyze(args []string) error {
	c := a.Cases.Current()
	if c == nil {
		return fmt.Errorf("no case is open")
	}

	var rulesPath, sample string
	for i := 0; i < len(args); i++ {
		if args[i] == "--rules" && i+1 < len(args) {
			rulesPath = args[i+1]
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

	if rulesPath == "" {
		candidate := filepath.Join(c.Path, "rules.yar")
		if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() {
			rulesPath = candidate
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
	for _, toolName := range staticToolsFor[kind] {
		if toolName == "yara" && rulesPath == "" {
			fmt.Fprintf(os.Stderr, "%swarning:%s no yara rules available (pass --rules or place rules.yar in the case dir) — skipping yara\n", colorYellow, colorReset)
			results = append(results, analyzeToolResult{tool: "yara", status: "skipped"})
			continue
		}

		var toolArgs []string
		switch toolName {
		case "yara":
			toolArgs = []string{rulesPath, containerPath}
		case "capa":
			toolArgs = []string{"-j", containerPath}
		case "oletools":
			toolArgs = []string{"olevba", containerPath}
		}

		out, runErr := a.runAnalyzeTool(toolName, toolArgs)
		if runErr != nil {
			if strings.Contains(runErr.Error(), "not installed") {
				fmt.Fprintf(os.Stderr, "%swarning:%s tool %q not installed — run: install %s\n", colorYellow, colorReset, toolName, toolName)
				results = append(results, analyzeToolResult{tool: toolName, status: "skipped"})
				continue
			}
			fmt.Fprintf(os.Stderr, "%swarning:%s %s failed: %v\n", colorYellow, colorReset, toolName, runErr)
			results = append(results, analyzeToolResult{tool: toolName, status: "failed"})
			continue
		}

		if toolName == "capa" {
			hints := capaHints(out)
			fmt.Printf("%s: %d capabilit%s matched\n", toolName, len(hints), pluralIES(len(hints)))
		} else {
			fmt.Print(out)
		}

		beforeIOCs := len(c.IOCs())
		command := toolName + " " + strings.Join(toolArgs, " ")
		if err := captureInto(c, toolName, command, name, out); err != nil {
			fmt.Fprintf(os.Stderr, "%swarning:%s record %s output: %v\n", colorYellow, colorReset, toolName, err)
		}
		added := len(c.IOCs()) - beforeIOCs

		res := analyzeToolResult{tool: toolName, status: "ok", iocCount: added}
		if toolName == "capa" {
			res.capabilities = added
			res.iocCount = 0
		}
		results = append(results, res)
	}

	printAnalyzeSummary(results)
	return nil
}

// runAnalyzeTool resolves toolName (builtin or installed catalog tool) and
// runs it, returning its stdout. A tool that isn't installed/registered
// returns an error containing "not installed" so callers can distinguish a
// missing tool (skip) from an execution failure (warn).
func (a *App) runAnalyzeTool(toolName string, args []string) (string, error) {
	if builtins.Has(toolName) {
		result := a.Runner.RunBuiltin(toolName, args)
		if !result.Success() {
			if result.Error != nil {
				return result.Stdout, result.Error
			}
			return result.Stdout, fmt.Errorf("exit code %d: %s", result.ReturnCode, result.Stderr)
		}
		return result.Stdout, nil
	}

	tool, ok := a.Tools.Get(toolName)
	if !ok {
		return "", fmt.Errorf("tool %q not installed", toolName)
	}
	casePath := ""
	if c := a.Cases.Current(); c != nil {
		casePath = c.Path
	}
	result := a.Runner.Run(tool, args, casePath)
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
