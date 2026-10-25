# TUI Redesign — Design

Status: agreed 2026-10-25. Code is written by orchestrated agents; this document
defines the design and acceptance criteria only.

Scope decision: **single-panel, information-dense.** Not a multi-pane dashboard,
not a whole-investigation TUI. Rationale: PLAN.md puts panes, tabs, splits and
layout out of scope as "a terminal multiplexer", and Mimir already has a
multiplexer-shaped feature on the roadmap (the W3 case daemon). The TUI stays a
viewer and launcher for tools, and gets *denser*, not wider.

## Current state — verified defects

Every item below was confirmed by reading the code or by running it, not inferred.

1. **The explorer is empty on a fresh install.** `cmdDashboard`
   (`internal/shell/shell.go`) passes only `a.Tools.List("")` — the *installed*
   tool registry — into `NewToolExplorer`. `Registry.List("")`
   (`internal/tools/tools.go`) returns only registered tools, so the 12 native
   builtins and the 12 catalog entries are never offered. Verified by running it:
   on a machine with no tools installed the TUI renders exactly
   `No tools match the current filter`.

2. **The empty state drops all chrome.** `ToolExplorer.View()` returns a bare
   string when `len(visible) == 0` (`internal/ui/tui_tools.go:227`), so there is no
   header, no key help and no footer. The user sees one line and nothing else.

3. **No width awareness.** `m.width` is captured from `tea.WindowSizeMsg` and never
   read again. Each row is a single unbounded `fmt.Sprintf` line
   (`tui_tools.go:274`), so a long description runs past the right edge and the
   status label after it lands off-screen.

4. **Hardcoded colours.** Rows use literal 256-colour numbers (`"34"`, `"9"`,
   `"3"`, `"39"`, `"230"`, `"242"`). `internal/theme` exists with four presets and
   `internal/ui` never imports it, so themes do not reach the dashboard.

5. **No search.** Filtering is by status (`1`/`2`/`3`) and category (`c`) only.
   There is no way to find a tool by name, description or tag.

6. **No detail view.** The description is inline and gets clipped by the terminal
   edge, so a long description is unreadable and there is nowhere to see tags,
   run count or the last output path together.

7. **Refresh is one-shot.** `Init` resolves statuses once; `r` re-runs it. There is
   no in-flight state, so a slow Docker probe looks identical to an idle screen.

8. **Viewport maths is wrong at some sizes.** The window is
   `m.height - 4` (`tui_tools.go:235`) on the assumption that the header and footer
   occupy four lines. The header is one line and the footer is one or two, so the
   scroll window is off by one or two rows depending on state.

9. **Running a tool discards the TUI.** `Enter` sets `selected` and quits
   (`tui_tools.go:160`); `cmdDashboard` then runs the tool in the shell. The
   alternate screen is torn down, so the result is read in the REPL and the
   explorer cannot show it.

10. **No case context.** The explorer shows tools and nothing about the open case.

## Design

### Row model

One row per tool, merged from three sources into a single list. Each entry
carries its source so the user can tell an installed tool from a builtin or an
installable catalog entry.

```
STATUS NAME            SOURCE     CATEGORY        RUNS  DESCRIPTION…
✓      hash            builtin    hashing               md5/sha1/sha256 digests of a file
○      volatility      available  forensics         ×3  Memory forensics with Volatility 3
```

STATUS is the existing `statusIcon` glyph. SOURCE is one of `builtin`,
`installed`, `available` — this is what fixes defect 1.

### Layout

Width-bounded, computed from the real terminal width via `displayWidth`, which is
now ANSI-safe after W4. Long descriptions wrap to a continuation line indented to
the description column rather than being cut mid-word; if the row still exceeds
the width it truncates with an ellipsis. `m.width` is actually used.

Header (one line): title, active case name, filter/search state, key help.

Footer (one or two lines): visible/total counts, scroll position, refresh error.

### Keybindings

| Key | Action |
|-----|--------|
| `j` / `k`, `↑` / `↓` | Move selection |
| `1` `2` `3` | Filter: all / ready / needs attention |
| `c` | Cycle category |
| `/` | Open search input |
| `esc` | Clear search |
| `tab` or `d` | Expand/collapse detail for the selected row |
| `r` | Refresh statuses |
| `enter` | Run the selected tool |
| `q`, `ctrl+c` | Quit |

Search filters across name, description, category and tags, case-insensitively.

### Detail expansion

`tab` expands the selected row in place — the list scrolls to accommodate it, no
second panel. The expansion shows the full description unwrapped, tags, source,
status, run count, and the resolved command or image. This keeps the
single-panel scope while making long descriptions readable (defect 6).

### Theming

Read colours from `internal/theme` instead of literals, so `theme set` affects the
dashboard (defect 4). Fall back to the current literals if the theme package
cannot supply a value, so a missing theme degrades rather than breaking.

### Refresh

Keep the existing one-shot resolve plus `r`, and add an in-flight flag. While a
refresh is running the footer reads `refreshing…` and the status column shows a
spinner glyph instead of stale values (defect 7).

### Viewport

Compute the window from the actual rendered header and footer line counts rather
than a constant (defect 8). The scroll indicator stays in the footer.

### Out of scope

- Multiple panes, tabs, splits, resizable layout — that is a terminal multiplexer.
- Showing tool *output* inside the TUI. Running a tool still quits to the shell;
  the TUI is a launcher, not a log viewer.
- Editing cases, evidence or the timeline from the TUI.

## Workstream W5 — tasks

- **T5.1** Merge all three tool sources into the explorer.

  **AC:** with nothing installed, the explorer lists the 12 builtins and the 12
  catalog entries, each labelled with its source.
  **AC:** `dashboard` and `tools` agree on the tool count.

- **T5.2** Always render header and footer chrome, including the empty state.

  **AC:** filtering to a set with no matches still shows the header, the key help
  and a footer reading `0 tools` — not a bare one-line string.

- **T5.3** Width-bounded rendering using `displayWidth`.

  **AC:** at 80, 120 and 200 columns no row exceeds the terminal width.
  **AC:** a description too long for the row wraps to an indented continuation
  line; a description too long for the remaining space truncates with an ellipsis.
  **AC:** wide (CJK) tool names do not break the column alignment.

- **T5.4** Theme-aware colours.

  **AC:** `theme set` changes the dashboard's colours, verified by capturing the
  frame under two different themes.
  **AC:** a missing or partial theme falls back without rendering garbage.

- **T5.5** Search.

  **AC:** `/` opens an input; typing filters by name, description, category and
  tag, case-insensitively.
  **AC:** `esc` clears the search and restores the full list.
  **AC:** the header shows the active search term.

- **T5.6** Inline detail expansion.

  **AC:** `tab` expands the selected row in place showing the full description,
  tags, source, status, run count and resolved command or image.
  **AC:** `tab` again collapses it, and the list scrolls correctly around the
  expanded row.

- **T5.7** Viewport and refresh state.

  **AC:** the scroll window matches the real header/footer heights at several
  terminal sizes — the last visible row is never clipped.
  **AC:** while a refresh runs the footer reads `refreshing…` and stale status
  glyphs are replaced, not left to read as current.

- **T5.8** Documentation.

  **AC:** `docs/reporting.md`'s dashboard section describes the merged source
  list, search, expansion and theming.
  **AC:** `CHANGELOG.md` `## [Unreleased]` records W0 and W4.

## Acceptance for the workstream

- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- The dashboard is captured in a terminal at 80, 120 and 200 columns and at two
  themes; each frame is inspected, not assumed.
- `dashboard` on a machine with no tools installed shows a populated list.
