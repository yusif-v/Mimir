package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ToolStatus represents the lifecycle state of a tool.
type ToolStatus int

const (
	ToolNotInstalled ToolStatus = iota
	ToolInstalled
	ToolBuilding
	ToolReady
)

// ToolEntry represents one tool in the explorer.
type ToolEntry struct {
	Name        string
	Description string
	Status      ToolStatus
	Runs        int      // number of times executed
	LastOutput  string   // brief last output snippet
	Tags        []string
}

// toolExplorer is the Bubble Tea model for the tool explorer.
type toolExplorer struct {
	tools     []ToolEntry
	cursor    int
	width     int
	height    int
	activeKey string // filter: "all", "installed", "notinstalled", "running"
}

// Init starts the model — no initial commands needed.
func (m toolExplorer) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m toolExplorer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
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
			m.activeKey = "installed"
			m.cursor = 0
		case "3":
			m.activeKey = "notinstalled"
			m.cursor = 0
		case "r":
			// Refresh action — would trigger re-discovery in real impl
			return m, refreshTools()
		case "enter":
			// Execute the selected tool
			filtered := m.filtered()
			if m.cursor < len(filtered) {
				idx := m.cursor
				tool := filtered[idx]
				return m, runTool(tool.Name)
			}
		}
	}
	return m, nil
}

// View renders the tool explorer.
func (m toolExplorer) View() string {
	visible := m.filtered()
	if len(visible) == 0 {
		return "No tools match the current filter"
	}

	var items []string
	for i, tool := range visible {
		style := lipgloss.NewStyle().PaddingLeft(2)
		if i == m.cursor {
			style = style.Background(lipgloss.Color("39")).Foreground(lipgloss.Color("230"))
		}

		statusIcon := "●"
		switch tool.Status {
		case ToolNotInstalled:
			statusIcon = "○"
		case ToolBuilding:
			statusIcon = "…"
		case ToolReady:
			statusIcon = "✓"
		}

		tags := ""
		if len(tool.Tags) > 0 {
			tags = fmt.Sprintf(" [%s]", strings.Join(tool.Tags, ", "))
		}

		runs := ""
		if tool.Runs > 0 {
			runs = fmt.Sprintf(" (×%d)", tool.Runs)
		}

		line := fmt.Sprintf("%s %s%s — %s%s",
			statusIcon,
			tool.Name,
			tags,
			tool.Description,
			runs,
		)

		items = append(items, style.Render(line))
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		Render("Tools  [1] All  [2] Installed  [3] Not Installed  [r] Refresh  [Enter] Run  [q] Quit")

	body := lipgloss.JoinVertical(lipgloss.Left, items...)
	return lipgloss.NewStyle().Padding(1, 2).Render(header + "\n" + body)
}

// filtered returns the tools matching the active filter.
func (m *toolExplorer) filtered() []ToolEntry {
	switch m.activeKey {
	case "installed":
		var out []ToolEntry
		for _, t := range m.tools {
			if t.Status == ToolInstalled || t.Status == ToolReady {
				out = append(out, t)
			}
		}
		return out
	case "notinstalled":
		var out []ToolEntry
		for _, t := range m.tools {
			if t.Status == ToolNotInstalled {
				out = append(out, t)
			}
		}
		return out
	default:
		return m.tools
	}
}

// refreshTools is a placeholder command for re-discovering tools.
func refreshTools() tea.Cmd {
	return func() tea.Msg { return nil }
}

// runTool is a placeholder command for executing a tool.
func runTool(name string) tea.Cmd {
	return func() tea.Msg { return nil }
}

// NewToolExplorer creates a new tool explorer model with sample data.
func NewToolExplorer() toolExplorer {
	return toolExplorer{
		tools: []ToolEntry{
			{Name: "hash", Description: "MD5/SHA1/SHA256", Status: ToolReady, Runs: 12, Tags: []string{"built-in"}},
			{Name: "strings", Description: "Printable ASCII extraction", Status: ToolReady, Runs: 8, Tags: []string{"built-in"}},
			{Name: "yara", Description: "Pattern matching engine", Status: ToolInstalled, Runs: 3, Tags: []string{"docker", "scanner"}},
			{Name: "volatility", Description: "Memory forensics", Status: ToolInstalled, Runs: 5, Tags: []string{"docker", "memory"}},
			{Name: "bulk_extractor", Description: "Disk image carving", Status: ToolNotInstalled, Tags: []string{"docker", "disk"}},
			{Name: "oletools", Description: "Office document analysis", Status: ToolNotInstalled, Tags: []string{"docker", "office"}},
		},
		activeKey: "all",
	}
}

// SetActiveKey sets the active filter key.
func (m *toolExplorer) SetActiveKey(key string) {
	m.activeKey = key
	m.cursor = 0
}

// SetToolStatus sets the status for a tool by name.
func (m *toolExplorer) SetToolStatus(name string, status ToolStatus) {
	for i := range m.tools {
		if m.tools[i].Name == name {
			m.tools[i].Status = status
		}
	}
}
