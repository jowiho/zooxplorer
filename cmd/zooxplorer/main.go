package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jowiho/zooxplorer/internal/snapshot"
	"github.com/jowiho/zooxplorer/internal/tui"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <snapshot-file>\n", os.Args[0])
		os.Exit(2)
	}

	tree, err := snapshot.ParseFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse snapshot: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(tui.NewModel(tree), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start tui: %v\n", err)
		os.Exit(1)
	}
}
