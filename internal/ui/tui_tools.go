// Package ui provides TUI widgets for Mimir's Bubble Tea dashboard.
package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yusif-v/mimir/internal/tools"
)

// ToolEntry is a single row in the tool explorer.
type ToolEntry struct {
	Name        string
	Description string
	Category    string
	Status      tools.Status
	Runs        int
	Tags        []string
}

// statusIcon returns a one-character indicator for a tool's status.
func statusIcon(s tools.Status) string {
	switch s {
	case tools.StatusReady:
		return "✓"
	case tools.StatusNotBuilt:
		return "○"
	case tools.StatusDockerOff:
		return "⚠"
	case tools.StatusMissing:
		return "✗"
	default:
		return "?"
	}
}

// statusColor returns a lipgloss color for a tool's status.
func statusColor(s tools.Status) string {
	switch s {
	case tools.StatusReady:
		return "34"
	case tools.StatusNotBuilt, tools.StatusMissing:
		return "9"
	case tools.StatusDockerOff:
		return "3"
	default:
		return "242"
	}
}

// statusesRefreshed carries freshly resolved tool statuses back into the model.
type statusesRefreshed struct {
	statuses map[string]tools.Status
	dockerUp bool
}

// statusesRefreshFailed reports a resolver failure so the UI can surface it.
type statusesRefreshFailed struct {
	err error
}

// ToolExplorer is the Bubble Tea model for the interactive tool explorer.
type ToolExplorer struct {
	tools      []ToolEntry
	cursor     int
	width      int
	height     int
	activeKey  string // status filter: "all", "ready", "notready"
	category   string // category filter: "" = all categories
	categories []string
	resolver   func() (map[string]tools.Status, bool)
	selected   string // tool chosen via Enter; consumed by the shell after quit
	errMsg     string
}

// NewToolExplorer creates a new tool explorer from Mimir's tool registry.
func NewToolExplorer(definitions []*tools.Definition, statuses map[string]tools.Status, runCounts map[string]int) *ToolExplorer {
	entries := make([]ToolEntry, 0, len(definitions))
	catSet := map[string]bool{}
	for _, def := range definitions {
		status := statuses[def.Name]
		if def.Category != "" {
			catSet[def.Category] = true
		}
		entries = append(entries, ToolEntry{
			Name:        def.Name,
			Description: def.Description,
			Category:    def.Category,
			Status:      status,
			Runs:        runCounts[def.Name],
			Tags:        def.Tags,
		})
	}
	categories := make([]string, 0, len(catSet))
	for c := range catSet {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	return &ToolExplorer{
		tools:      entries,
		activeKey:  "all",
		categories: categories,
	}
}

// SetResolver wires a live status resolver (used by [r] Refresh and on start).
// Passing nil disables refresh.
func (m *ToolExplorer) SetResolver(fn func() (map[string]tools.Status, bool)) {
	m.resolver = fn
}

// SelectedTool returns the tool the user chose with Enter, or "" if none.
// The shell reads this after the program quits to run the tool.
func (m *ToolExplorer) SelectedTool() string {
	return m.selected
}

// Init starts the model.
func (m *ToolExplorer) Init() tea.Cmd {
	return refreshToolsCmd(m.resolver)
}

// refreshToolsCmd resolves tool statuses off the event loop. It returns nil
// when no resolver is configured so an inert model stays inert.
func refreshToolsCmd(resolve func() (map[string]tools.Status, bool)) tea.Cmd {
	if resolve == nil {
		return nil
	}
	return func() tea.Msg {
		statuses, dockerUp := resolve()
		return statusesRefreshed{statuses: statuses, dockerUp: dockerUp}
	}
}

func (m *ToolExplorer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case statusesRefreshed:
		m.errMsg = ""
		for i := range m.tools {
			if s, ok := msg.statuses[m.tools[i].Name]; ok {
				m.tools[i].Status = s
			}
		}

	case statusesRefreshFailed:
		m.errMsg = msg.err.Error()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit

		case "enter":
			visible := m.filtered()
			if m.cursor < len(visible) {
				m.selected = visible[m.cursor].Name
				return m, tea.Quit
			}

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			filtered := m.filtered()
			if m.cursor < len(filtered)-1 {
				m.cursor++
			}

		case "1":
			m.activeKey = "all"
			m.cursor = 0

		case "2":
			m.activeKey = "ready"
			m.cursor = 0

		case "3":
			m.activeKey = "notready"
			m.cursor = 0

		case "c":
			m.cycleCategory()

		case "r":
			return m, refreshToolsCmd(m.resolver)
		}
	}
	return m, nil
}

// cycleCategory moves through the distinct categories present in the tool list,
// wrapping back to "all categories" after the last one.
func (m *ToolExplorer) cycleCategory() {
	m.cursor = 0
	if len(m.categories) == 0 {
		return
	}
	if m.category == "" {
		m.category = m.categories[0]
		return
	}
	for i, c := range m.categories {
		if c == m.category {
			if i+1 < len(m.categories) {
				m.category = m.categories[i+1]
			} else {
				m.category = ""
			}
			return
		}
	}
	m.category = ""
}

// View renders the tool explorer.
func (m *ToolExplorer) View() string {
	visible := m.filtered()
	if len(visible) == 0 {
		return "No tools match the current filter"
	}

	// Clip to the viewport: show a window around the cursor.
	window := visible
	winStart := 0
	if m.height > 0 {
		lines := m.height - 4 // header + 2 padding lines + scroll indicator
		if lines < 1 {
			lines = 1
		}
		if len(visible) > lines {
			winStart = m.cursor - lines/2
			if winStart < 0 {
				winStart = 0
			}
			if winStart+lines > len(visible) {
				winStart = len(visible) - lines
			}
			window = visible[winStart : winStart+lines]
		}
	}

	var items []string
	for i, tool := range window {
		style := lipgloss.NewStyle().PaddingLeft(2)
		if winStart+i == m.cursor {
			style = style.Background(lipgloss.Color("39")).Foreground(lipgloss.Color("230"))
		}

		icon := statusIcon(tool.Status)
		color := statusColor(tool.Status)
		coloredIcon := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(icon)

		tags := ""
		if len(tool.Tags) > 0 {
			tags = fmt.Sprintf(" [%s]", strings.Join(tool.Tags, ", "))
		}

		runs := ""
		if tool.Runs > 0 {
			runs = fmt.Sprintf(" (×%d)", tool.Runs)
		}

		statusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(tool.Status.String())

		line := fmt.Sprintf("%s %s%s%s — %s  %s%s",
			coloredIcon,
			tool.Name,
			tags,
			categorySuffix(tool),
			tool.Description,
			statusLabel,
			runs,
		)

		items = append(items, style.Render(line))
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Render(fmt.Sprintf("Tools  [1] All  [2] Ready  [3] Needs Attention  [c] Category: %s  [r] Refresh  [Enter] Run  [q] Quit", categoryLabel(m.category)))

	body := lipgloss.JoinVertical(lipgloss.Left, items...)
	footer := ""
	if len(window) < len(visible) {
		footer = fmt.Sprintf("\nShowing %d of %d tools (scroll with j/k)",
			len(window), len(visible))
	} else if len(visible) > 1 {
		footer = fmt.Sprintf("\n%d tools", len(visible))
	}
	if m.errMsg != "" {
		footer += fmt.Sprintf("\nRefresh failed: %s", m.errMsg)
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(header + "\n" + body + footer)
}

// categoryLabel returns the display label for the active category filter.
func categoryLabel(c string) string {
	if c == "" {
		return "all"
	}
	return c
}

func categorySuffix(t ToolEntry) string {
	if t.Category == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", t.Category)
}

// filtered returns the tools matching the active status and category filters.
func (m *ToolExplorer) filtered() []ToolEntry {
	var out []ToolEntry
	for _, t := range m.tools {
		if m.category != "" && t.Category != m.category {
			continue
		}
		switch m.activeKey {
		case "ready":
			if t.Status != tools.StatusReady {
				continue
			}
		case "notready":
			if t.Status == tools.StatusReady {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// SetActiveKey sets the active filter.
func (m *ToolExplorer) SetActiveKey(key string) {
	m.activeKey = key
	m.cursor = 0
}
