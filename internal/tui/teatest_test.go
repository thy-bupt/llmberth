package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestTeaRenderSmoke runs the real bubbletea program against the stub admin
// API and waits for rendered frames containing the dashboard sections. This
// is the teatest gate (plan §7: TUI is teatest-tested).
func TestTeaRenderSmoke(t *testing.T) {
	dir := stubProject(t, stubMux())
	model, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(90, 28))
	defer tm.Quit() //nolint:errcheck

	// First frame: dashboard sections appear after the initial render. The
	// refresh tick is 3s, so the wait must exceed one tick to see a re-rendered
	// frame that includes the usage sections.
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "Stack health") && strings.Contains(string(bts), "Usage")
	}, teatest.WithDuration(6*time.Second))

	// Tab to keys and run the create flow through the real Update pipeline.
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	time.Sleep(150 * time.Millisecond)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	tm.Type("accept")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	// The once-only full key must appear in a frame (and be dismissible).
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), "lbt_live_FULLKEYONCE")
	}, teatest.WithDuration(3*time.Second))
}
