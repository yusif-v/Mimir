package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yusif-v/mimir/internal/theme"
	"github.com/yusif-v/mimir/internal/tools"
)

func TestToolExplorerView(t *testing.T) {
	defs := []*tools.Definition{
		{
			Name:        "hash",
			Description: "MD5/SHA1/SHA256",
			Tags:        []string{"built-in"},
		},
		{
			Name:        "volatility",
			Description: "Memory forensics",
			Tags:        []string{"docker", "memory"},
		},
	}

	statuses := map[string]tools.Status{
		"hash":       tools.StatusReady,
		"volatility": tools.StatusNotBuilt,
	}

	runCounts := map[string]int{
		"hash":       5,
		"volatility": 0,
	}

	model := NewToolExplorer(defs, statuses, runCounts)

	// Verify all filter modes work
	for _, filter := range []string{"all", "ready", "notready"} {
		model.SetActiveKey(filter)
		view := model.View()
		if !strings.Contains(view, "Tools") {
			t.Errorf("filter %s: view missing header", filter)
		}
		if !strings.Contains(view, "[1]") {
			t.Errorf("filter %s: view missing filter hint", filter)
		}
	}

	// Verify "all" shows both tools
	model.SetActiveKey("all")
	view := model.View()
	if !strings.Contains(view, "hash") {
		t.Error("view should contain 'hash'")
	}
	if !strings.Contains(view, "volatility") {
		t.Error("view should contain 'volatility'")
	}
	if !strings.Contains(view, "×5") {
		t.Error("view should contain run count for hash")
	}

	// Verify "ready" filter shows only hash
	model.SetActiveKey("ready")
	view = model.View()
	if !strings.Contains(view, "hash") {
		t.Error("ready filter should show hash")
	}
	if strings.Contains(view, "volatility") {
		t.Error("ready filter should not show volatility")
	}

	// Verify "notready" filter shows only volatility
	model.SetActiveKey("notready")
	view = model.View()
	if !strings.Contains(view, "volatility") {
		t.Error("notready filter should show volatility")
	}
	if strings.Contains(view, "hash") {
		t.Error("notready filter should not show hash")
	}
}

func explorerFixture() (*ToolExplorer, []*tools.Definition, map[string]tools.Status, map[string]int) {
	defs := []*tools.Definition{
		{
			Name:        "hash",
			Description: "MD5/SHA1/SHA256",
			Category:    "built-in",
			Tags:        []string{"built-in"},
		},
		{
			Name:        "volatility",
			Description: "Memory forensics",
			Category:    "forensics",
			Tags:        []string{"docker", "memory"},
		},
		{
			Name:        "cuckoo",
			Description: "Automated malware analysis",
			Category:    "malware",
			Tags:        []string{"docker", "dynamic"},
		},
	}
	statuses := map[string]tools.Status{
		"hash":       tools.StatusReady,
		"volatility": tools.StatusNotBuilt,
		"cuckoo":     tools.StatusNotBuilt,
	}
	runCounts := map[string]int{"hash": 5}
	model := NewToolExplorer(defs, statuses, runCounts)
	return model, defs, statuses, runCounts
}

func TestToolExplorerCategoryFilter(t *testing.T) {
	model, _, _, _ := explorerFixture()

	view := model.View()
	if !strings.Contains(view, "hash") || !strings.Contains(view, "volatility") || !strings.Contains(view, "cuckoo") {
		t.Fatal("all-categories view should show every tool")
	}

	// Enter + c cycles to the first category.
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	view = model.View()
	if !strings.Contains(view, "built-in") {
		t.Errorf("category filter should show active category label, got:\n%s", view)
	}
	if !strings.Contains(view, "hash") {
		t.Errorf("built-in filter should show hash, got:\n%s", view)
	}
	if strings.Contains(view, "volatility") || strings.Contains(view, "cuckoo") {
		t.Errorf("built-in filter should hide other categories, got:\n%s", view)
	}

	// A second c advances to the next category.
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	view = model.View()
	if !strings.Contains(view, "forensics") || !strings.Contains(view, "volatility") {
		t.Errorf("forensics filter should show volatility, got:\n%s", view)
	}

	// Cycling past the last category wraps back to all.
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // -> malware
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}) // -> all
	view = model.View()
	if !strings.Contains(view, "cuckoo") {
		t.Errorf("wrapping back to all should show cuckoo, got:\n%s", view)
	}
}

func TestToolExplorerEnterSelectsToolAndQuits(t *testing.T) {
	model, _, _, _ := explorerFixture()

	m, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a quit command on Enter")
	}
	explorer, ok := m.(*ToolExplorer)
	if !ok {
		t.Fatalf("expected *ToolExplorer, got %T", m)
	}
	if explorer.SelectedTool() != "hash" {
		t.Errorf("SelectedTool = %q, want %q", explorer.SelectedTool(), "hash")
	}

	// Moving the cursor then pressing Enter selects the highlighted tool.
	model2, _, _, _ := explorerFixture()
	model2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2, _ := model2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	explorer2 := m2.(*ToolExplorer)
	if explorer2.SelectedTool() != "volatility" {
		t.Errorf("SelectedTool after down = %q, want %q", explorer2.SelectedTool(), "volatility")
	}
}

func TestToolExplorerRefreshUpdatesStatuses(t *testing.T) {
	model, defs, statuses, runCounts := explorerFixture()

	// Initial state: everything not-built except hash.
	if model.tools[1].Status != tools.StatusNotBuilt {
		t.Fatalf("expected volatility not-built initially")
	}

	// Resolver simulates a refresh where volatility becomes ready.
	model.SetResolver(func() (map[string]tools.Status, bool) {
		statuses["volatility"] = tools.StatusReady
		return statuses, true
	})

	cmd := refreshToolsCmd(model.resolver)
	if cmd == nil {
		t.Fatal("expected refresh command with resolver set")
	}
	msg := cmd()
	refreshed, ok := msg.(statusesRefreshed)
	if !ok {
		t.Fatalf("expected statusesRefreshed, got %T", msg)
	}
	if !refreshed.dockerUp {
		t.Error("expected dockerUp true")
	}

	m, _ := model.Update(refreshed)
	explorer := m.(*ToolExplorer)
	if explorer.tools[1].Status != tools.StatusReady {
		t.Errorf("volatility status not refreshed: %v", explorer.tools[1].Status)
	}
	_ = defs
	_ = runCounts
}

func TestToolExplorerViewportClipping(t *testing.T) {
	model, _, _, _ := explorerFixture()

	// Small height forces the view to clip to fewer rows.
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	view := model.View()
	if !strings.Contains(view, "Showing") {
		t.Errorf("expected scroll indicator, got:\n%s", view)
	}
	if !strings.Contains(view, "scroll with j/k") {
		t.Errorf("expected scroll hint, got:\n%s", view)
	}
}

func TestToolExplorerInitFiresRefreshWhenResolved(t *testing.T) {
	model, _, _, _ := explorerFixture()
	model.SetResolver(func() (map[string]tools.Status, bool) {
		return map[string]tools.Status{}, false
	})

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init should return a refresh command when a resolver is set")
	}
	msg := cmd()
	if _, ok := msg.(statusesRefreshed); !ok {
		t.Fatalf("expected statusesRefreshed from Init, got %T", msg)
	}

	// Without a resolver, Init stays inert.
	model2, _, _, _ := explorerFixture()
	if cmd := model2.Init(); cmd != nil {
		t.Errorf("Init without resolver should return nil, got %T", cmd)
	}
}

func TestToolExplorerShowsSourceLabels(t *testing.T) {
	entries := []ToolEntry{
		{Name: "hash", Description: "digests", Source: SourceBuiltin, Status: tools.StatusReady},
		{Name: "yara", Description: "pattern matching", Source: SourceAvailable, Status: tools.StatusNotBuilt},
		{Name: "tshark", Description: "packet capture", Source: SourceInstalled, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)

	view := model.View()
	for _, want := range []string{"[builtin]", "[available]", "[installed]"} {
		if !strings.Contains(view, want) {
			t.Errorf("view should label source %q, got:\n%s", want, view)
		}
	}
}

func TestToolExplorerEmptyStateKeepsChrome(t *testing.T) {
	entries := []ToolEntry{
		{Name: "volatility", Description: "memory", Source: SourceAvailable, Status: tools.StatusNotBuilt},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.SetActiveKey("ready")

	view := model.View()
	for _, want := range []string{"Tools", "[1] All", "[q] Quit", "0 tools"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty state should keep chrome %q, got:\n%s", want, view)
		}
	}
	if strings.Contains(view, "No tools match the current filter") {
		t.Errorf("empty state should not drop back to a bare message, got:\n%s", view)
	}
}

func TestToolExplorerWidthBounded(t *testing.T) {
	entries := []ToolEntry{
		{Name: "hash", Description: "md5/sha1/sha256 digests of a file", Category: "hashing", Source: SourceBuiltin, Status: tools.StatusReady, Runs: 3},
		{Name: "volatility", Description: strings.Repeat("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima ", 3) + "zulu", Category: "forensics", Source: SourceAvailable, Status: tools.StatusNotBuilt, Tags: []string{"docker", "memory"}},
		{Name: "日本語", Description: "wide tool name alignment check", Category: "forensics", Source: SourceBuiltin, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)

	for _, width := range []int{80, 120, 200} {
		model.Update(tea.WindowSizeMsg{Width: width, Height: 60})
		view := model.View()
		for n, line := range strings.Split(view, "\n") {
			if got := displayWidth(line); got > width {
				t.Errorf("width %d: line %d is %d columns: %q", width, n, got, line)
			}
		}
	}

	model.Update(tea.WindowSizeMsg{Width: 80, Height: 60})
	view := model.View()
	if !strings.Contains(view, "zulu") {
		t.Fatalf("wrapped description lost its final word:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "zulu") {
			if leading := len(line) - len(strings.TrimLeft(line, " ")); leading <= explorerIndent {
				t.Errorf("continuation line should be indented past the row indent, got %q", line)
			}
		}
	}
}

func TestToolExplorerCJKNameAlignment(t *testing.T) {
	entries := []ToolEntry{
		{Name: "hash", Description: "digests", Category: "forensics", Source: SourceBuiltin, Status: tools.StatusReady},
		{Name: "日本語", Description: "wide name", Category: "forensics", Source: SourceBuiltin, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	view := model.View()

	var columns []int
	for _, line := range strings.Split(view, "\n") {
		if idx := strings.Index(line, "[builtin]"); idx >= 0 {
			columns = append(columns, displayWidth(line[:idx]))
		}
	}
	if len(columns) != 2 {
		t.Fatalf("expected two source columns, got %v in:\n%s", columns, view)
	}
	if columns[0] != columns[1] {
		t.Errorf("CJK name broke column alignment: %v", columns)
	}
}

func TestToolExplorerThemeColours(t *testing.T) {
	entries := []ToolEntry{
		{Name: "hash", Description: "digests", Category: "hashing", Source: SourceBuiltin, Status: tools.StatusReady, Runs: 2},
		{Name: "volatility", Description: "memory", Category: "forensics", Source: SourceAvailable, Status: tools.StatusNotBuilt},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})

	def := model.View()
	model.SetTheme(theme.KaliTheme())
	kali := model.View()
	if def == kali {
		t.Error("default and kali themes rendered identically")
	}
	if !strings.Contains(def, "\033[34m") {
		t.Errorf("default theme should use the default brand colour, got %q", def)
	}
	if !strings.Contains(kali, "\033[96m") {
		t.Errorf("kali theme should use bright cyan, got %q", kali)
	}

	model.SetTheme(theme.NoColorTheme())
	plain := model.View()
	if strings.Contains(plain, "\033[") {
		t.Errorf("no-color theme should emit no ANSI escapes, got %q", plain)
	}

	model.SetTheme(theme.Theme{Name: "partial", Colors: theme.ColorMap{Brand: "magenta"}})
	partial := model.View()
	if !strings.Contains(partial, "\033[35m") {
		t.Errorf("partial theme should keep its brand colour, got %q", partial)
	}
	if strings.Contains(partial, "\033[m") || strings.Contains(partial, "\033[;m") {
		t.Errorf("partial theme produced an empty colour sequence: %q", partial)
	}
}

func TestToolExplorerNoColorRespected(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	entries := []ToolEntry{
		{Name: "hash", Description: "digests", Category: "hashing", Source: SourceBuiltin, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	model.SetTheme(theme.DefaultTheme())

	view := model.View()
	if strings.Contains(view, "\033[") {
		t.Errorf("NO_COLOR should suppress all ANSI escapes, got %q", view)
	}
}

func TestToolExplorerSearch(t *testing.T) {
	model, _, _, _ := explorerFixture()

	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !model.searching {
		t.Fatal("slash should open the search input")
	}
	for _, r := range "DYNAMIC" {
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	view := model.View()
	if !strings.Contains(view, "Search: DYNAMIC") {
		t.Errorf("header should show the active search term, got:\n%s", view)
	}
	if !strings.Contains(view, "cuckoo") {
		t.Errorf("search should match tags case-insensitively, got:\n%s", view)
	}
	if strings.Contains(view, "hash") || strings.Contains(view, "volatility") {
		t.Errorf("search should filter out non-matching tools, got:\n%s", view)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.query != "" || model.searching {
		t.Fatalf("esc should clear the search, got query %q searching %v", model.query, model.searching)
	}
	view = model.View()
	if !strings.Contains(view, "hash") || !strings.Contains(view, "volatility") || !strings.Contains(view, "cuckoo") {
		t.Errorf("clearing the search should restore the full list, got:\n%s", view)
	}
}

func TestToolExplorerSearchFields(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		{"vol", "volatility"},
		{"Malware", "cuckoo"},
		{"forensics", "volatility"},
		{"DOCKER", "volatility"},
	}
	for _, tt := range tests {
		model, _, _, _ := explorerFixture()
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		for _, r := range tt.query {
			model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if view := model.View(); !strings.Contains(view, tt.want) {
			t.Errorf("query %q should match %q, got:\n%s", tt.query, tt.want, view)
		}
	}
}

func TestToolExplorerSearchConsumesShortcutKeys(t *testing.T) {
	entries := []ToolEntry{
		{Name: "jhash", Description: "j digests", Category: "one", Source: SourceBuiltin, Status: tools.StatusReady},
		{Name: "jvolatility", Description: "j memory", Category: "two", Source: SourceAvailable, Status: tools.StatusNotBuilt},
		{Name: "jcuckoo", Description: "j malware", Category: "three", Source: SourceInstalled, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model.cursor = 1

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cmd != nil {
		t.Error("typing in search should not return a command")
	}
	if model.cursor != 1 {
		t.Errorf("typing j in search moved the cursor to %d", model.cursor)
	}
	if model.query != "j" {
		t.Errorf("typing j in search should append to the query, got %q", model.query)
	}

	for _, r := range "ckr" {
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if model.query != "jckr" {
		t.Errorf("shortcut keys should type into search, got %q", model.query)
	}
	if model.activeKey != "all" || model.category != "" {
		t.Errorf("search keys changed filters: activeKey=%q category=%q", model.activeKey, model.category)
	}
}

func TestToolExplorerSearchNoMatchesKeepsChrome(t *testing.T) {
	model, _, _, _ := explorerFixture()
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "zzz" {
		model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	view := model.View()
	for _, want := range []string{"Tools", "[1] All", "[q] Quit", "0 tools", "Search: zzz"} {
		if !strings.Contains(view, want) {
			t.Errorf("empty search should keep chrome %q, got:\n%s", want, view)
		}
	}
}

func TestToolExplorerDetailExpandsAndCollapses(t *testing.T) {
	desc := "memory forensics with volatility three including process enumeration and " +
		"network artefacts and a distinctive expansiontail marker"
	entries := []ToolEntry{
		{Name: "volatility", Description: desc, Category: "forensics", Source: SourceAvailable,
			Status: tools.StatusNotBuilt, Runs: 7, Tags: []string{"docker", "memory"}},
		{Name: "hash", Description: "digests", Category: "hashing", Source: SourceBuiltin, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	collapsed := model.View()
	if strings.Contains(collapsed, "Description:") {
		t.Fatalf("detail labels shown before expansion:\n%s", collapsed)
	}

	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	expanded := model.View()
	for _, want := range []string{"Description:", "Tags:", "Source:", "Status:", "Runs:", "Category:", "expansiontail"} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded row missing %q:\n%s", want, expanded)
		}
	}
	for _, want := range []string{"forensics", "available", "not built", "7", "docker, memory"} {
		if !strings.Contains(expanded, want) {
			t.Errorf("expanded detail missing %q:\n%s", want, expanded)
		}
	}
	for n, line := range strings.Split(expanded, "\n") {
		if got := displayWidth(line); got > 100 {
			t.Errorf("expanded line %d is %d columns: %q", n, got, line)
		}
	}

	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	collapsed = model.View()
	if strings.Contains(collapsed, "Description:") {
		t.Errorf("detail labels still shown after collapse:\n%s", collapsed)
	}
}

func TestToolExplorerExpandedRowScrollsAndKeepsChrome(t *testing.T) {
	var entries []ToolEntry
	for i := 0; i < 25; i++ {
		entries = append(entries, ToolEntry{
			Name:        fmt.Sprintf("tool%02d", i),
			Description: "short description",
			Category:    "forensics",
			Source:      SourceAvailable,
			Status:      tools.StatusNotBuilt,
		})
	}
	// Give the first row a detail block taller than the remaining viewport.
	entries[0].Description = strings.Repeat("alpha bravo charlie delta echo foxtrot ", 12)
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	model.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 20 {
		t.Fatalf("view is %d lines for height 20:\n%s", len(lines), view)
	}
	if len(lines) < 2 || !strings.Contains(lines[1], "Tools") {
		t.Fatalf("header was pushed off the top:\n%s", view)
	}
	if !strings.Contains(view, "Description:") {
		t.Errorf("expanded row detail not visible:\n%s", view)
	}
	for n, line := range lines {
		if got := displayWidth(line); got > 80 {
			t.Errorf("expanded line %d is %d columns: %q", n, got, line)
		}
	}

	// Moving the cursor scrolls the list without losing the expanded row's
	// identity or breaking the frame height.
	for i := 0; i < 24; i++ {
		model.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	view = model.View()
	lines = strings.Split(view, "\n")
	if len(lines) > 20 {
		t.Fatalf("view is %d lines after scrolling for height 20:\n%s", len(lines), view)
	}
	if !strings.Contains(lines[1], "Tools") {
		t.Fatalf("header disappeared while scrolling:\n%s", view)
	}
}

func TestToolExplorerViewportFitsAtVariousSizes(t *testing.T) {
	var entries []ToolEntry
	for i := 0; i < 60; i++ {
		entries = append(entries, ToolEntry{
			Name:        fmt.Sprintf("tool%02d", i),
			Description: "short description",
			Category:    "forensics",
			Source:      SourceAvailable,
			Status:      tools.StatusReady,
		})
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)

	sizes := []struct {
		width, height int
	}{
		{80, 24},
		{100, 30},
		{120, 40},
	}
	for _, size := range sizes {
		for _, cursor := range []int{0, 30, 59} {
			model.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			model.cursor = cursor
			view := model.View()
			lines := strings.Split(view, "\n")
			if len(lines) > size.height {
				t.Errorf("%dx%d cursor %d: view is %d lines:\n%s",
					size.width, size.height, cursor, len(lines), view)
			}
			if len(lines) < 2 || strings.TrimSpace(lines[0]) != "" || !strings.Contains(lines[1], "Tools") {
				t.Errorf("%dx%d cursor %d: header not at the top:\n%s",
					size.width, size.height, cursor, view)
			}
			// Every content row ends with its complete status suffix, so no
			// row was cut off at the bottom edge.
			lastRow := ""
			for _, line := range lines[2:] {
				if strings.Contains(line, " — ") {
					lastRow = line
				}
			}
			if !strings.Contains(lastRow, "ready") {
				t.Errorf("%dx%d cursor %d: last visible row is incomplete: %q\n%s",
					size.width, size.height, cursor, lastRow, view)
			}
			for n, line := range lines {
				if got := displayWidth(line); got > size.width {
					t.Errorf("%dx%d cursor %d: line %d is %d columns: %q",
						size.width, size.height, cursor, n, got, line)
				}
			}
		}
	}
}

func TestToolExplorerRefreshShowsInFlightState(t *testing.T) {
	model, _, statuses, _ := explorerFixture()
	model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model.SetResolver(func() (map[string]tools.Status, bool) {
		return statuses, true
	})

	cmd := model.Init()
	if !model.refreshing {
		t.Fatal("Init must set the in-flight flag synchronously")
	}
	if cmd == nil {
		t.Fatal("Init should return a refresh command")
	}
	view := model.View()
	if !strings.Contains(view, "refreshing…") {
		t.Errorf("footer should read refreshing…, got:\n%s", view)
	}
	if !strings.Contains(view, "⟳") {
		t.Errorf("rows should show a refresh glyph, got:\n%s", view)
	}
	if strings.Contains(view, "✓") || strings.Contains(view, "○") {
		t.Errorf("stale status glyphs should be replaced during refresh:\n%s", view)
	}

	model.Update(cmd())
	view = model.View()
	if model.refreshing {
		t.Error("refreshing flag should clear when statuses arrive")
	}
	if strings.Contains(view, "refreshing…") {
		t.Errorf("footer still says refreshing after completion:\n%s", view)
	}
	if !strings.Contains(view, "✓") {
		t.Errorf("fresh status glyph should be shown after refresh:\n%s", view)
	}

	// A failed refresh also leaves the in-flight state.
	model.Update(statusesRefreshFailed{err: fmt.Errorf("boom")})
	if model.refreshing {
		t.Error("refreshing flag should clear on failure")
	}
	if view := model.View(); !strings.Contains(view, "Refresh failed: boom") {
		t.Errorf("failure should be surfaced in the footer:\n%s", view)
	}
}

func TestToolExplorerRefreshKeySetsInFlight(t *testing.T) {
	model, _, statuses, _ := explorerFixture()
	model.SetResolver(func() (map[string]tools.Status, bool) {
		return statuses, true
	})

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("r should return a refresh command")
	}
	if !model.refreshing {
		t.Error("r should set the in-flight flag before the command runs")
	}
	if view := model.View(); !strings.Contains(view, "refreshing…") {
		t.Errorf("footer should read refreshing… after r, got:\n%s", view)
	}
}

func TestToolExplorerDetailLongWordStaysComplete(t *testing.T) {
	word := strings.Repeat("abcdefghij", 20)
	entries := []ToolEntry{
		{Name: "long", Description: word, Source: SourceBuiltin, Status: tools.StatusReady},
	}
	model := NewToolExplorerFromEntries(entries, nil, nil)
	model.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	model.Update(tea.KeyMsg{Type: tea.KeyTab})

	view := model.View()
	if !strings.Contains(view, word[len(word)-10:]) {
		t.Errorf("expanded detail truncated a long word:\n%s", view)
	}
	for n, line := range strings.Split(view, "\n") {
		if got := displayWidth(line); got > 40 {
			t.Errorf("line %d is %d columns: %q", n, got, line)
		}
	}
}
