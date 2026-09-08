// Package tui is the bubbletea operator console (plan §3.2). It is a pure
// view layer: every byte of data comes from internal packages
// (admin/runtime/usage) — no second business logic, so CLI and TUI can
// never drift.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thy-bupt/llmberth/internal/admin"
	"github.com/thy-bupt/llmberth/internal/scaffold"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tabs in order; switching is via tab key or digits.
const (
	TabDashboard = iota
	TabKeys
	TabLogs
	tabCount
)

// Model is the top-level TUI state.
type Model struct {
	projectDir string
	client     *admin.Client
	manifest   *scaffold.Manifest

	tab int

	// Dashboard data (refreshed on tick).
	dashErr     string
	svcRows     []runtimeRow
	dashDay     usageSummary
	dashWeek    usageSummary
	dashMonth   usageSummary
	dashBudget  string
	healthOK    bool

	// Keys state.
	keys        []admin.Key
	keysErr     string
	keyAction   string // "", "creating", "show-created", "confirm-revoke"
	keyInput    string
	created     *admin.CreateKeyResult
	revokeIndex int
	revokeSid   string // selected key id

	// Logs state.
	logsBuf    []string
	logsFollow bool
	logFilter  string // "", "ERROR", "WARN", "INFO", "DEBUG"
	logTailMsg string
	cancelLogs context.CancelFunc

	width, height int
}

// runtimeRow is one stack service line (from internal/runtime).
type runtimeRow struct {
	service, state, published string
}

// usageSummary is the subset of internal/usage.Summary the dashboard needs
// (computed by internal/usage — the TUI never aggregates by itself).
type usageSummary struct {
	title    string
	totalReq int64
	totalTok int64
	cost     float64
}

// New builds the TUI model from a project directory (admin client +
// manifest; both resolved through the same internal paths the CLI uses).
func New(projectDir string) (tea.Model, error) {
	m, err := scaffold.LoadManifest(projectDir + "/" + scaffold.ManifestFileName)
	if err != nil {
		return nil, err
	}
	client, err := admin.NewClient(projectDir, m, nil)
	if err != nil {
		return nil, err
	}
	return &Model{
		projectDir: projectDir,
		client:     client,
		manifest:   &m,
		tab:        TabDashboard,
		logFilter:  "INFO",
	}, nil
}

// Init starts the refresh ticker (bubbletea contract).
func (m *Model) Init() tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg { return tickMsg{} })
}

type tickMsg struct{}

// Update handles keys, resize and the refresh tick (bubbletea contract).
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tickMsg:
		m.refreshSilent()
		return m, tea.Tick(3*time.Second, func(_ time.Time) tea.Msg { return tickMsg{} })
	}
	return m, nil
}

func (m *Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.keyAction {
	case "creating":
		return m.handleCreateInput(msg)
	case "confirm-revoke":
		return m.handleRevokeConfirm(msg)
	case "show-created":
		if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
			m.keyAction = ""
			m.created = nil
			m.refreshKeys()
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		m.stopLogs()
		return m, tea.Quit
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % tabCount
		if m.tab == TabLogs {
			m.startLogs()
		} else {
			m.stopLogs()
		}
	case "shift+tab", "left", "h":
		m.tab = (m.tab + tabCount - 1) % tabCount
		if m.tab == TabLogs {
			m.startLogs()
		} else {
			m.stopLogs()
		}
	case "1":
		m.tab = TabDashboard
		m.stopLogs()
	case "2":
		m.tab = TabKeys
		m.refreshKeys()
		m.stopLogs()
	case "3":
		m.tab = TabLogs
		m.startLogs()
	default:
		// Keys page interactions (list navigation + actions).
		if m.tab == TabKeys && m.keyAction == "" {
			m.keysNav(msg)
		}
		if m.tab == TabLogs {
			m.logsNav(msg)
		}
	}
	return m, nil
}

// keysNav handles the keys list keys: n=create, r=revoke, up/down/enter.
func (m *Model) keysNav(msg tea.KeyMsg) {
	switch msg.String() {
	case "n":
		m.keyAction = "creating"
		m.keyInput = ""
	case "r":
		if id := m.selectedKeyID(); id != "" {
			m.revokeSid = id
			m.keyAction = "confirm-revoke"
		}
	case "up", "k":
		if m.revokeIndex > 0 {
			m.revokeIndex--
		}
	case "down", "j":
		if m.revokeIndex < len(m.keys)-1 {
			m.revokeIndex++
		}
	}
}

// logsNav handles the logs page keys: f cycles the level filter.
func (m *Model) logsNav(msg tea.KeyMsg) {
	if msg.String() == "f" {
		levels := []string{"ALL", "ERROR", "WARN", "INFO", "DEBUG"}
		for i, l := range levels {
			if m.logFilter == l {
				m.logFilter = levels[(i+1)%len(levels)]
				return
			}
		}
		m.logFilter = "ALL"
	}
}

func (m *Model) startLogs() {
	if m.logsFollow {
		return
	}
	if m.projectDir == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelLogs = cancel
	m.logsFollow = true
	m.refreshLogs(ctx)
}

func (m *Model) stopLogs() {
	if m.cancelLogs != nil {
		m.cancelLogs()
		m.cancelLogs = nil
	}
	m.logsFollow = false
}

// View renders the active tab inside the status bar frame.
func (m *Model) View() string {
	if m.width == 0 {
		m.width = 80
	}
	if m.height == 0 {
		m.height = 24
	}
	var body string
	switch m.tab {
	case TabDashboard:
		body = m.dashboardView()
	case TabKeys:
		body = m.keysView()
	case TabLogs:
		body = m.logsView()
	}
	title := []string{"DASHBOARD", "KEYS", "LOGS"}[m.tab]
	tabBar := ""
	for i, t := range []string{"1 Dashboard", "2 Keys", "3 Logs"} {
		if i == m.tab {
			tabBar += styles.tabActive.Render(" " + t + " ")
		} else {
			tabBar += styles.tabInactive.Render(" " + t + " ")
		}
		tabBar += " "
	}
	status := fmt.Sprintf(" %s | q quit | llmberth %s", title, m.projectDir)
	return lipgloss.JoinVertical(lipgloss.Left,
		tabBar,
		body,
		styles.statusBar.Render(status),
	)
}

// refreshSilent reloads dashboard/keys data (best effort; errors surface in
// the view instead of killing the app).
func (m *Model) refreshSilent() {
	if m.tab == TabLogs {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	m.refreshDashboard(ctx)
	if m.tab == TabKeys {
		m.refreshKeys()
	}
}

var _ = tea.Batch
var _ = strings.TrimSpace
