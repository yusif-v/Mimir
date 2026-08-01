package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yusif-v/mimir/internal/ui"
)

func main() {
	if _, err := tea.NewProgram(
		ui.NewToolExplorer(),
		tea.WithAltScreen(),
	).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
