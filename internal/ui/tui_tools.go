// Package ui provides TUI widgets for Mimir's Bubble Tea dashboard.
package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yusif-v/mimir/internal/theme"
	"github.com/yusif-v/mimir/internal/tools"
)

// Tool source labels, used to tell where an explorer row came from.
const (
	SourceBuiltin   = "builtin"
	SourceInstalled = "installed"
	SourceAvailable = "available"
)

const explorerIndent = 2

// ToolEntry is a single row in the tool explorer.
type ToolEntry struct {
	Name        string
	Description string
	Category    string
	Source      string
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

type explorerPalette struct {
	brand      theme.Color
	statusOK   theme.Color
	statusErr  theme.Color
	statusWarn theme.Color
	dim        theme.Color
}

// paletteFor fills any missing slots from the default theme so a partial theme
// degrades instead of rendering without usable colours. The no-color theme is
// intentionally empty and stays that way.
func paletteFor(t theme.Theme) explorerPalette {
	if t.Name == "no-color" {
		return explorerPalette{}
	}
	def := theme.DefaultTheme()
	pick := func(c, fallback theme.Color) theme.Color {
		if c != "" {
			return c
		}
		return fallback
	}
	return explorerPalette{
		brand:      pick(t.Colors.Brand, def.Colors.Brand),
		statusOK:   pick(t.Colors.StatusOK, def.Colors.StatusOK),
		statusErr:  pick(t.Colors.StatusErr, def.Colors.StatusErr),
		statusWarn: pick(t.Colors.Case, def.Colors.Case),
		dim:        pick(t.Colors.Dim, def.Colors.Dim),
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
	searching  bool
	query      string
	theme      theme.Theme
	resolver   func() (map[string]tools.Status, bool)
	selected   string // tool chosen via Enter; consumed by the shell after quit
	errMsg     string
	expanded   string // name of the row expanded inline by tab; "" = none
	refreshing bool   // a status refresh is in flight
}

// NewToolExplorer creates a new tool explorer from Mimir's installed tool
// registry. Built-in and catalog entries should be passed through
// NewToolExplorerFromEntries instead.
func NewToolExplorer(definitions []*tools.Definition, statuses map[string]tools.Status, runCounts map[string]int) *ToolExplorer {
	entries := make([]ToolEntry, 0, len(definitions))
	for _, def := range definitions {
		entries = append(entries, ToolEntry{
			Name:        def.Name,
			Description: def.Description,
			Category:    def.Category,
			Source:      SourceInstalled,
			Tags:        def.Tags,
		})
	}
	return NewToolExplorerFromEntries(entries, statuses, runCounts)
}

// NewToolExplorerFromEntries creates a tool explorer from pre-merged entries,
// preserving each entry's source (builtin, installed or available). Statuses
// and run counts are applied by name where present.
func NewToolExplorerFromEntries(entries []ToolEntry, statuses map[string]tools.Status, runCounts map[string]int) *ToolExplorer {
	merged := make([]ToolEntry, 0, len(entries))
	catSet := map[string]bool{}
	for _, entry := range entries {
		if status, ok := statuses[entry.Name]; ok {
			entry.Status = status
		}
		if runs, ok := runCounts[entry.Name]; ok {
			entry.Runs = runs
		}
		if entry.Category != "" {
			catSet[entry.Category] = true
		}
		merged = append(merged, entry)
	}
	categories := make([]string, 0, len(catSet))
	for c := range catSet {
		categories = append(categories, c)
	}
	sort.Strings(categories)
	return &ToolExplorer{
		tools:      merged,
		activeKey:  "all",
		categories: categories,
		theme:      theme.DefaultTheme(),
	}
}

// SetTheme sets the theme used to render the explorer.
func (m *ToolExplorer) SetTheme(t theme.Theme) {
	m.theme = t
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

// Init starts the model. The in-flight flag is set synchronously so the first
// frame can render the refresh state; setting it inside the returned command
// would happen after that frame.
func (m *ToolExplorer) Init() tea.Cmd {
	return m.startRefresh()
}

// startRefresh marks a refresh in flight and returns the resolving command.
// With no resolver configured it is a no-op so an inert model stays inert.
func (m *ToolExplorer) startRefresh() tea.Cmd {
	if m.resolver == nil {
		return nil
	}
	m.refreshing = true
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
		m.refreshing = false
		m.errMsg = ""
		for i := range m.tools {
			if s, ok := msg.statuses[m.tools[i].Name]; ok {
				m.tools[i].Status = s
			}
		}

	case statusesRefreshFailed:
		m.refreshing = false
		m.errMsg = msg.err.Error()

	case tea.KeyMsg:
		if m.searching {
			return m, m.updateSearch(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.query != "" {
				m.query = ""
				m.cursor = 0
			} else {
				return m, tea.Quit
			}

		case "/":
			m.searching = true
			m.cursor = 0

		case "tab", "d":
			m.toggleExpanded()

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
			return m, m.startRefresh()
		}
	}
	return m, nil
}

// updateSearch handles keys while the search input is open. It deliberately
// consumes every rune so j/k/c/r type into the query instead of acting as
// navigation or filter shortcuts.
func (m *ToolExplorer) updateSearch(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		m.query = ""
		m.searching = false
		m.cursor = 0
	case tea.KeyEnter:
		m.searching = false
	case tea.KeyBackspace, tea.KeyDelete:
		r := []rune(m.query)
		if len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
		m.clampCursor()
	case tea.KeyCtrlC:
		return tea.Quit
	case tea.KeySpace:
		m.query += " "
		m.clampCursor()
	case tea.KeyRunes:
		m.query += string(msg.Runes)
		m.clampCursor()
	}
	return nil
}

// clampCursor keeps the cursor inside the filtered list without resetting it
// to the top on every keystroke.
func (m *ToolExplorer) clampCursor() {
	n := len(m.filtered())
	if n == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
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

// toggleExpanded expands the selected row inline, or collapses it when that
// same row is already expanded. One row is expanded at a time.
func (m *ToolExplorer) toggleExpanded() {
	visible := m.filtered()
	if m.cursor < 0 || m.cursor >= len(visible) {
		return
	}
	name := visible[m.cursor].Name
	if m.expanded == name {
		m.expanded = ""
		return
	}
	m.expanded = name
}

type rowColumns struct {
	name     int
	source   int
	category int
	desc     int
}

// rowColumns computes the widths of the fixed columns. Width zero means the
// terminal size is unknown, so the natural widths are used without wrapping.
func (m *ToolExplorer) rowColumns(entries []ToolEntry, avail int) rowColumns {
	nameW, sourceW, catW := 0, 0, 0
	for _, t := range entries {
		nameW = maxInt(nameW, displayWidth(t.Name))
		sourceW = maxInt(sourceW, displayWidth(sourceLabel(t)))
		catW = maxInt(catW, displayWidth(t.Category))
	}
	const nameCap, sourceCap, categoryCap = 32, 13, 18
	nameW = minInt(nameW, nameCap)
	sourceW = minInt(sourceW, sourceCap)
	catW = minInt(catW, categoryCap)
	if nameW < 4 {
		nameW = 4
	}
	if avail <= 0 {
		return rowColumns{name: nameW, source: sourceW, category: catW}
	}

	meta := 1 + 1 // status icon + space
	meta += sourceW + 1
	if catW > 0 {
		meta += catW + 1
	}
	meta += 3 // " — "

	descMin := avail / 3
	if descMin < 20 {
		descMin = 20
	}
	if descMin > 60 {
		descMin = 60
	}
	nameBudget := avail - meta - descMin
	if nameW > nameBudget {
		nameW = nameBudget
	}
	if nameW < 4 {
		nameW = 4
	}
	descW := avail - meta - nameW
	if descW < 1 {
		nameW = avail - meta - 1
		if nameW < 1 {
			nameW = 1
		}
		descW = avail - meta - nameW
		if descW < 1 {
			descW = 1
		}
	}
	return rowColumns{name: nameW, source: sourceW, category: catW, desc: descW}
}

// View renders the tool explorer.
func (m *ToolExplorer) View() string {
	visible := m.filtered()
	avail := m.contentWidth()
	cols := m.rowColumns(visible, avail)

	// Render every row first: rows wrap and an expanded row adds detail lines,
	// so the viewport must be measured in rendered lines, not row count.
	rendered := make([][]string, len(visible))
	heights := make([]int, len(visible))
	for i, tool := range visible {
		rendered[i] = m.renderRow(tool, i == m.cursor, cols)
		heights[i] = len(rendered[i])
		if heights[i] < 1 {
			heights[i] = 1
		}
	}

	// The footer is one count/scroll line, plus optional refresh and error
	// lines. Chrome is the leading blank line and the one-line header.
	footerHeight := 1
	if m.refreshing {
		footerHeight++
	}
	if m.errMsg != "" {
		footerHeight++
	}

	start, end := 0, len(visible)
	if m.height > 0 {
		availRows := m.height - 2 - footerHeight
		if availRows < 1 {
			availRows = 1
		}
		start, end = rowWindow(heights, m.cursor, availRows)
	}

	count := fmt.Sprintf("%d tools", len(visible))
	if len(visible) == 1 {
		count = "1 tool"
	}
	footerLines := []string{count}
	if start > 0 || end < len(visible) {
		footerLines = []string{fmt.Sprintf("Showing %d of %d tools (scroll with j/k)",
			end-start, len(visible))}
	}
	if m.refreshing {
		footerLines = append(footerLines, "refreshing…")
	}
	if m.errMsg != "" {
		footerLines = append(footerLines, "Refresh failed: "+m.errMsg)
	}

	var items []string
	for i := start; i < end; i++ {
		items = append(items, rendered[i]...)
	}
	// A single row taller than the viewport (a very long expanded detail) must
	// not push the header off the top.
	if m.height > 0 {
		availRows := m.height - 2 - len(footerLines)
		if availRows < 1 {
			availRows = 1
		}
		if len(items) > availRows {
			items = items[:availRows]
		}
	}

	header := m.renderHeader(avail)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(strings.Repeat(" ", explorerIndent))
	b.WriteString(header)
	if len(items) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(items, "\n"))
	}
	for _, line := range footerLines {
		b.WriteString("\n")
		b.WriteString(strings.Repeat(" ", explorerIndent))
		b.WriteString(m.fitLine(line, avail))
	}
	return b.String()
}

// rowWindow returns the half-open range of rows that fits in avail rendered
// lines while keeping the cursor row visible. Rows are atomic: a wrapped row is
// never split. If the cursor row alone is taller than avail it is returned on
// its own and the caller clips its lines so the chrome stays in place.
func rowWindow(heights []int, cursor, avail int) (int, int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if avail < 1 {
		avail = 1
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}

	prefix := make([]int, n+1)
	for i, h := range heights {
		prefix[i+1] = prefix[i] + h
	}
	if prefix[n] <= avail {
		return 0, n
	}

	cursorH := heights[cursor]
	if cursorH > avail {
		return cursor, cursor + 1
	}

	// Aim to centre the cursor row, then fill downwards from there.
	startLine := prefix[cursor] - (avail-cursorH)/2
	if startLine < 0 {
		startLine = 0
	}
	start := 0
	for start < n && prefix[start+1] <= startLine {
		start++
	}
	if start > cursor {
		start = cursor
	}

	end := start
	used := 0
	for end < n && used+heights[end] <= avail {
		used += heights[end]
		end++
	}
	if end <= cursor {
		start = cursor
		end = start
		used = 0
		for end < n && used+heights[end] <= avail {
			used += heights[end]
			end++
		}
	}
	return start, end
}

func (m *ToolExplorer) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	w := m.width - explorerIndent
	if w < 1 {
		w = 1
	}
	return w
}

func (m *ToolExplorer) renderHeader(avail int) string {
	search := "[/] Search"
	if m.searching {
		search = fmt.Sprintf("[/] Search: %s_", m.query)
	} else if m.query != "" {
		search = fmt.Sprintf("[/] Search: %s", m.query)
	}
	header := fmt.Sprintf("Tools  %s  [1] All  [2] Ready  [3] Needs Attention  [c] Category: %s  [r] Refresh  [Enter] Run  [q] Quit",
		search, categoryLabel(m.category))
	header = m.fitLine(header, avail)
	return theme.Colorize(header, m.palette().brand)
}

func (m *ToolExplorer) fitLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return truncateText(s, width)
}

func (m *ToolExplorer) renderRow(t ToolEntry, selected bool, cols rowColumns) []string {
	name := fitText(t.Name, cols.name)
	source := fitText(sourceLabel(t), cols.source)
	cat := fitText(t.Category, cols.category)

	icon := statusIcon(t.Status)
	if m.refreshing {
		icon = "⟳"
	}

	var meta strings.Builder
	meta.WriteString(strings.Repeat(" ", explorerIndent))
	meta.WriteString(icon)
	meta.WriteString(" ")
	meta.WriteString(name)
	meta.WriteString(" ")
	meta.WriteString(source)
	if cols.category > 0 {
		meta.WriteString(" ")
		meta.WriteString(cat)
	}
	meta.WriteString(" — ")
	metaWidth := displayWidth(meta.String())

	desc := strings.TrimSpace(t.Description)
	if len(t.Tags) > 0 {
		tags := "[" + strings.Join(t.Tags, ", ") + "]"
		if desc == "" {
			desc = tags
		} else {
			desc += " " + tags
		}
	}

	var descLines []string
	if cols.desc > 0 {
		descLines = wrapText(desc, cols.desc)
	} else {
		descLines = []string{desc}
	}
	if len(descLines) == 0 {
		descLines = []string{""}
	}

	suffix := t.Status.String()
	if m.refreshing {
		suffix = "refreshing…"
	}
	if t.Runs > 0 {
		suffix += fmt.Sprintf(" (×%d)", t.Runs)
	}
	if cols.desc > 0 {
		last := len(descLines) - 1
		if displayWidth(descLines[last])+2+displayWidth(suffix) <= cols.desc {
			descLines[last] += "  " + suffix
		} else if displayWidth(suffix) <= cols.desc {
			descLines = append(descLines, suffix)
		} else {
			descLines = append(descLines, truncateText(suffix, cols.desc))
		}
	} else {
		descLines[0] += "  " + suffix
	}

	indent := strings.Repeat(" ", metaWidth)
	lines := make([]string, 0, len(descLines))
	for i, d := range descLines {
		if i == 0 {
			lines = append(lines, meta.String()+d)
			continue
		}
		lines = append(lines, indent+d)
	}

	if selected {
		lines = append(lines, m.renderDetailIfExpanded(t)...)
		pal := m.palette()
		code := theme.ResolveColor(pal.brand)
		if code == "" {
			return lines
		}
		for i := range lines {
			lines[i] = code + "\033[7m" + lines[i] + "\033[0m"
		}
		return lines
	}

	color := m.statusColor(t.Status)
	first := lines[0]
	if strings.HasPrefix(first[explorerIndent:], icon) {
		first = first[:explorerIndent] + theme.Colorize(icon, color) + first[explorerIndent+len(icon):]
	}
	last := lines[len(lines)-1]
	if idx := strings.LastIndex(last, suffix); idx >= 0 {
		last = last[:idx] + theme.Colorize(suffix, color) + last[idx+len(suffix):]
	}
	lines[0] = first
	lines[len(lines)-1] = last
	lines = append(lines, m.renderDetailIfExpanded(t)...)
	return lines
}

// renderDetailIfExpanded returns the extra lines for the selected row when it
// is expanded, and nothing otherwise.
func (m *ToolExplorer) renderDetailIfExpanded(t ToolEntry) []string {
	if m.expanded == "" || t.Name != m.expanded {
		return nil
	}
	return m.renderDetail(t)
}

// renderDetail renders the full-height detail block for an expanded row. It
// uses the whole content width rather than the description column, wrapping
// only when the terminal cannot show the value on one line.
func (m *ToolExplorer) renderDetail(t ToolEntry) []string {
	indent := strings.Repeat(" ", explorerIndent+2)
	avail := m.contentWidth()
	fields := []struct{ label, value string }{
		{"Description", strings.TrimSpace(t.Description)},
		{"Tags", listValue(t.Tags)},
		{"Source", sourceLabel(t)},
		{"Status", m.statusText(t)},
		{"Runs", fmt.Sprintf("%d", t.Runs)},
		{"Category", valueOrNone(t.Category)},
	}

	var lines []string
	for _, f := range fields {
		prefix := indent + f.label + ": "
		value := valueOrNone(f.value)
		width := 0
		if avail > 0 {
			width = avail - displayWidth(prefix)
			if width < 1 {
				width = 1
			}
		}
		wrapped := wrapFullText(value, width)
		contIndent := indent + strings.Repeat(" ", displayWidth(f.label+": "))
		for i, part := range wrapped {
			if i == 0 {
				lines = append(lines, prefix+part)
				continue
			}
			lines = append(lines, contIndent+part)
		}
	}
	return lines
}

func (m *ToolExplorer) statusText(t ToolEntry) string {
	if m.refreshing {
		return "refreshing…"
	}
	return t.Status.String()
}

func valueOrNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func listValue(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func (m *ToolExplorer) palette() explorerPalette {
	return paletteFor(m.theme)
}

func (m *ToolExplorer) statusColor(s tools.Status) theme.Color {
	pal := m.palette()
	if m.refreshing {
		return pal.dim
	}
	switch s {
	case tools.StatusReady:
		return pal.statusOK
	case tools.StatusNotBuilt, tools.StatusMissing:
		return pal.statusErr
	case tools.StatusDockerOff:
		return pal.statusWarn
	default:
		return pal.dim
	}
}

func sourceLabel(t ToolEntry) string {
	source := t.Source
	if source == "" {
		source = SourceInstalled
	}
	return "[" + source + "]"
}

// categoryLabel returns the display label for the active category filter.
func categoryLabel(c string) string {
	if c == "" {
		return "all"
	}
	return c
}

// filtered returns the tools matching the active status, category and search
// filters.
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
		if !matchesQuery(t, m.query) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func matchesQuery(t ToolEntry, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		t.Name,
		t.Description,
		t.Category,
		strings.Join(t.Tags, " "),
	}, " "))
	return strings.Contains(haystack, q)
}

// SetActiveKey sets the active filter.
func (m *ToolExplorer) SetActiveKey(key string) {
	m.activeKey = key
	m.cursor = 0
}

func fitText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) > width {
		s = truncateText(s, width)
	}
	gap := width - displayWidth(s)
	if gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

func truncateText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := displayWidth(string(r))
		if used+w > width-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	b.WriteString("…")
	return b.String()
}

// wrapFullText wraps like wrapText but splits words that are wider than the
// line instead of truncating them, so an expanded description stays complete.
func wrapFullText(s string, width int) []string {
	s = strings.TrimSpace(s)
	if width <= 0 || s == "" {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, word := range words {
		for displayWidth(word) > width {
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			head, rest := splitWidth(word, width)
			lines = append(lines, head)
			word = rest
		}
		if word == "" {
			continue
		}
		if cur == "" {
			cur = word
			continue
		}
		if displayWidth(cur)+1+displayWidth(word) <= width {
			cur += " " + word
			continue
		}
		lines = append(lines, cur)
		cur = word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// splitWidth splits s at the last rune boundary that fits in width.
func splitWidth(s string, width int) (string, string) {
	if width < 1 {
		width = 1
	}
	runes := []rune(s)
	used := 0
	for i, r := range runes {
		w := displayWidth(string(r))
		if used+w > width {
			if i == 0 {
				return string(r), string(runes[1:])
			}
			return string(runes[:i]), string(runes[i:])
		}
		used += w
	}
	return s, ""
}

func wrapText(s string, width int) []string {
	s = strings.TrimSpace(s)
	if width <= 0 || s == "" {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, word := range words {
		if cur == "" {
			cur = truncateText(word, width)
			continue
		}
		if displayWidth(cur)+1+displayWidth(word) <= width {
			cur += " " + word
			continue
		}
		lines = append(lines, cur)
		cur = truncateText(word, width)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
