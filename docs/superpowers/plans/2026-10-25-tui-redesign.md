# TUI Redesign — Implementation Plan

Design: `docs/superpowers/specs/2026-10-25-tui-redesign-design.md`.
Workstream: W5 in `PLAN.md`. Agreed 2026-10-25.

Eight tasks. T5.1–T5.2 are the correctness fixes; T5.3–T5.7 are the rendering and
interaction work; T5.8 is documentation. T5.1 must land before T5.3, because the
merged source list is what the width-aware renderer has to lay out.

## Phase 1 — correctness

### T5.1 Merge all three tool sources

`cmdDashboard` currently passes `a.Tools.List("")` only. The explorer needs
installed tools, native builtins and catalog entries in one list, each carrying
its source.

- **Do:** build the merged entry list in `cmdDashboard` and pass it to
  `NewToolExplorer`; add a `Source` field to `ToolEntry`.
- **Done when:** with nothing installed, the explorer lists 12 builtins and 12
  catalog entries labelled by source; `dashboard` and `tools` agree on the count.
- **Watch out for:** `builtins.List()` returns `[]Meta`, not `[]*tools.Definition`
  — the two shapes differ, so the merge needs an adapter rather than a type
  assertion. Catalog entries come from `catalog.List()`.
- **Learn:** why a UI that reads one registry silently hides two-thirds of the
  tool surface.

### T5.2 Always render chrome

`View()` returns a bare string when the filtered list is empty.

- **Do:** restructure `View()` so the header and footer are computed first and the
  body is empty-tolerant.
- **Done when:** a filter with no matches still shows header, key help and a
  footer reading `0 tools`.
- **Watch out for:** the footer's scroll-position text must not assume a non-empty
  window.
- **Learn:** an empty state is a state like any other and still needs its chrome.

## Phase 2 — rendering

### T5.3 Width-bounded rows

`m.width` is captured and never used; rows are unbounded `fmt.Sprintf` lines.

- **Do:** compute column widths from `m.width` using `displayWidth` (ANSI-safe
  since W4); wrap long descriptions to an indented continuation line; truncate
  with an ellipsis when the remaining space is too small.
- **Done when:** at 80, 120 and 200 columns no row exceeds the terminal width;
  CJK names do not break alignment.
- **Watch out for:** `displayWidth` counts East Asian wide runes as two columns —
  padding with `len()` instead of `displayWidth` will misalign exactly the rows
  that look fine in testing.
- **Learn:** display width versus byte length versus rune count, and why a table
  renderer has to pick one.

### T5.4 Theme-aware colours

Literal colour numbers throughout; `internal/theme` is never imported by
`internal/ui`.

- **Do:** read colours from the active theme, falling back to the current literals
  when the theme cannot supply a value.
- **Done when:** `theme set` changes the dashboard colours, verified by capturing
  frames under two themes; a partial theme degrades without garbage output.
- **Watch out for:** the theme package's colour type is not a raw string — check
  `internal/theme/color.go` before assuming a `lipgloss.Color` conversion.
- **Learn:** what a theme abstraction buys, and where a hardcoded fallback belongs.

### T5.5 Search

- **Do:** add a `/` search input filtering name, description, category and tags
  case-insensitively; `esc` clears.
- **Done when:** the header shows the active term; clearing restores the full
  list; a search matching nothing shows the T5.2 empty state.
- **Watch out for:** the search input needs its own key handling branch so typing
  `j`/`k`/`c`/`r` filters instead of moving the cursor.
- **Learn:** modal input inside a single-panel TUI without a second pane.

### T5.6 Inline detail expansion

- **Do:** `tab` expands the selected row in place — full description unwrapped,
  tags, source, status, run count, resolved command or image.
- **Done when:** expand and collapse both work; the list scrolls correctly around
  an expanded row; the viewport accounts for the extra lines.
- **Watch out for:** this interacts with T5.7's viewport maths. Land T5.7 first
  or do both together, or the scroll window will be wrong again.
- **Learn:** in-place expansion as the single-panel answer to "I need more detail".

### T5.7 Viewport and refresh state

- **Do:** compute the scroll window from real header/footer heights; add an
  in-flight flag that renders `refreshing…` and replaces stale status glyphs.
- **Done when:** at several terminal sizes the last visible row is never clipped;
  during a refresh the footer reads `refreshing…`.
- **Watch out for:** `Init` returns the refresh command, so the in-flight flag has
  to be set synchronously in `Init` — a flag set inside the returned `tea.Cmd`
  runs too late.
- **Learn:** why a spinner needs state that exists before the async work starts.

## Phase 3 — documentation

### T5.8 Docs

- **Do:** rewrite the dashboard section of `docs/reporting.md`; fill in
  `CHANGELOG.md` `## [Unreleased]` for W0 and W4.
- **Done when:** the docs describe the merged source list, search, expansion and
  theming; the changelog entry names the go.sum tracking, the go.mod
  reconciliation and the tools-table work.
- **Watch out for:** `docs/reporting.md` currently describes the dashboard as a
  plain "tool explorer" — the description needs to change, not just grow.
- **Learn:** documentation that describes behaviour that no longer exists is
  worse than none.

## Sequencing

```
T5.1 ─► T5.2 ─► T5.3 ─► T5.4 ─► T5.5 ─► T5.6 ─► T5.7 ─► T5.8
                  │
                  └─ T5.6 and T5.7 both change the viewport; do them together
                     or land T5.7 first
```

T5.1 and T5.2 are independent of everything else and can run in parallel. From
T5.3 onward the tasks all edit `internal/ui/tui_tools.go` and must be sequential.

## Acceptance for the workstream

- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- Frames captured at 80, 120 and 200 columns and under two themes, each inspected.
- `dashboard` on a machine with no tools installed shows a populated list.

## Report back

The agent reports: files changed, commands run, and the captured frame at each
width. A claim that the TUI "looks right" without a captured frame is not
evidence — the frame is the deliverable.
