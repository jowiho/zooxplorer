package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jowiho/zooxplorer/internal/snapshot"
	"github.com/jowiho/zooxplorer/internal/tui"
)

type loadProgressMsg struct {
	read  int64
	total int64
}

type loadDoneMsg struct {
	tree *snapshot.Tree
	err  error
}

type appModel struct {
	snapshotPath string
	events       chan tea.Msg
	loading      bool
	loadErr      error
	readBytes    int64
	totalBytes   int64
	width        int
	height       int
	ui           tea.Model
}

var (
	loadTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	loadTextStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	loadBarEmpty   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	loadBarFill    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	loadErrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

func newAppModel(snapshotPath string) appModel {
	return appModel{
		snapshotPath: snapshotPath,
		events:       make(chan tea.Msg, 256),
		loading:      true,
	}
}

func (m appModel) Init() tea.Cmd {
	return tea.Batch(startLoadCmd(m.snapshotPath, m.events), waitLoadEventCmd(m.events))
}

func startLoadCmd(path string, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			tree, err := snapshot.ParseFileWithProgress(path, func(readBytes, totalBytes int64) {
				msg := loadProgressMsg{read: readBytes, total: totalBytes}
				select {
				case events <- msg:
				default:
				}
			})
			events <- loadDoneMsg{tree: tree, err: err}
		}()
		return nil
	}
}

func waitLoadEventCmd(events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.loading && m.loadErr == nil && m.ui != nil {
		var cmd tea.Cmd
		m.ui, cmd = m.ui.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
		if m.loadErr != nil {
			return m, tea.Quit
		}
	case loadProgressMsg:
		m.readBytes = msg.read
		m.totalBytes = msg.total
		return m, waitLoadEventCmd(m.events)
	case loadDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err
			return m, nil
		}
		ui := tui.NewModel(msg.tree)
		m.ui = ui
		if m.width > 0 && m.height > 0 {
			var cmd tea.Cmd
			m.ui, cmd = m.ui.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, cmd
		}
		return m, nil
	}

	if m.loading {
		return m, waitLoadEventCmd(m.events)
	}
	return m, nil
}

func (m appModel) View() string {
	if !m.loading && m.loadErr == nil && m.ui != nil {
		return m.ui.View()
	}

	if m.loadErr != nil {
		lines := []string{
			loadErrStyle.Render("Failed to parse snapshot"),
			"",
			loadTextStyle.Render(m.loadErr.Error()),
			"",
			loadTextStyle.Render("Press any key to exit."),
		}
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, strings.Join(lines, "\n"))
	}

	width := m.width
	if width < 64 {
		width = 64
	}
	barWidth := width - 24
	if barWidth > 72 {
		barWidth = 72
	}
	if barWidth < 20 {
		barWidth = 20
	}

	progress := 0.0
	if m.totalBytes > 0 {
		progress = float64(m.readBytes) / float64(m.totalBytes)
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	filled := int(math.Round(progress * float64(barWidth)))
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := loadBarFill.Render(strings.Repeat("█", filled)) + loadBarEmpty.Render(strings.Repeat("░", barWidth-filled))
	percent := fmt.Sprintf("%3.0f%%", progress*100)
	details := "Loading snapshot"
	if m.totalBytes > 0 {
		details = fmt.Sprintf("Loading snapshot %s / %s", humanBytes(m.readBytes), humanBytes(m.totalBytes))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("39")).
		Padding(1, 2).
		Render(strings.Join([]string{
			loadTitleStyle.Render("Zooxplorer"),
			"",
			loadTextStyle.Render(details),
			bar + "  " + loadTextStyle.Render(percent),
		}, "\n"))

	return lipgloss.Place(width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func humanBytes(v int64) string {
	if v < 1024 {
		return fmt.Sprintf("%d B", v)
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(v)
	u := 0
	for size >= 1024 && u < len(units)-1 {
		size /= 1024
		u++
	}
	return fmt.Sprintf("%.1f %s", size, units[u])
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <snapshot-file>\n", os.Args[0])
		os.Exit(2)
	}

	p := tea.NewProgram(newAppModel(os.Args[1]), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start tui: %v\n", err)
		os.Exit(1)
	}
	if app, ok := finalModel.(appModel); ok && app.loadErr != nil {
		fmt.Fprintf(os.Stderr, "failed to parse snapshot: %v\n", app.loadErr)
		os.Exit(1)
	}
}
