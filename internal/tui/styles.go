package tui

import "github.com/charmbracelet/lipgloss"

// styles centralizes the lipgloss styling (view-layer only).
var styles = struct {
	tabActive   lipgloss.Style
	tabInactive lipgloss.Style
	statusBar   lipgloss.Style
	title       lipgloss.Style
	ok          lipgloss.Style
	warn        lipgloss.Style
	bad         lipgloss.Style
	muted       lipgloss.Style
	box         lipgloss.Style
	keyFull     lipgloss.Style
}{
	tabActive:   lipgloss.NewStyle().Background(lipgloss.Color("33")).Foreground(lipgloss.Color("15")).Bold(true),
	tabInactive: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	statusBar:   lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("236")).Padding(0, 1),
	title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
	ok:          lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
	warn:        lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	bad:         lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
	muted:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	box:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	keyFull:     lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33")).Bold(true).Padding(0, 1),
}