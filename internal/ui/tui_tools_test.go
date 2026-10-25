package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
