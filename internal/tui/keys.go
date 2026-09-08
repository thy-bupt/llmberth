package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/THY17308111153/llmberth/internal/admin"
	tea "github.com/charmbracelet/bubbletea"
)

// refreshKeys reloads the key list through internal/admin (never directly).
func (m *Model) refreshKeys() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	keys, err := m.client.ListKeys(ctx)
	if err != nil {
		m.keysErr = err.Error()
		m.keys = nil
		return
	}
	m.keysErr = ""
	m.keys = keys
}

// handleCreateInput drives the name prompt of key creation inside the TUI.
func (m *Model) handleCreateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.keyInput)
		m.keyInput = ""
		if name == "" {
			m.keyAction = ""
			m.keysErr = "empty name; creation cancelled"
			return m, nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		res, err := m.client.CreateKey(ctx, admin.CreateKeyRequest{Name: name})
		if err != nil {
			m.keyAction = ""
			m.keysErr = err.Error()
			return m, nil
		}
		m.created = &res
		m.keyAction = "show-created"
		return m, nil
	case "esc":
		m.keyAction = ""
		m.keyInput = ""
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.keyInput) > 0 {
			m.keyInput = m.keyInput[:len(m.keyInput)-1]
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.keyInput += msg.String()
		}
		return m, nil
	}
}

// handleRevokeConfirm drives the y/n confirm for the selected key.
func (m *Model) handleRevokeConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := m.client.RevokeKey(ctx, m.revokeSid); err != nil {
			m.keysErr = err.Error()
		}
		m.keyAction = ""
		m.refreshKeys()
		return m, nil
	default: // anything else cancels
		m.keyAction = ""
		return m, nil
	}
}

// keysView renders the list plus creation/revoke/show-created overlays.
// The full key is displayed exactly once (plan §3.2) and dismissed on any
// key press.
func (m *Model) keysView() string {
	if m.keyAction == "show-created" && m.created != nil {
		return styles.keyFull.Render(
			fmt.Sprintf("API key created — store it now, it is never displayed again:\n\n  %s\n\npress any key to continue", m.created.Full))
	}
	if m.keyAction == "creating" {
		return styles.box.Render(fmt.Sprintf("New key name (esc to cancel):\n\n  %s▌", m.keyInput))
	}
	if m.keyAction == "confirm-revoke" {
		return styles.box.Render(fmt.Sprintf("Revoke key %s? [y]es / any other key cancels", m.revokeSid))
	}

	var b strings.Builder
	b.WriteString(styles.title.Render("API keys (loopback admin API)\n"))
	if m.keysErr != "" {
		b.WriteString("  " + styles.bad.Render("!"+m.keysErr+"\n"))
	}
	if len(m.keys) == 0 {
		b.WriteString("  (no keys — create one with `n`)\n")
	} else {
		fmt.Fprintf(&b, "  %-36s %-20s %-16s %-10s %s\n", "ID", "PREFIX", "NAME", "BUDGET", "STATE")
		for _, k := range m.keys {
			state := styles.ok.Render("active")
			if k.RevokedAt != nil {
				state = styles.bad.Render("revoked")
			}
			budget := "-"
			if k.BudgetUSD != nil {
				budget = fmt.Sprintf("$%.2f", *k.BudgetUSD)
			}
			name := k.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Fprintf(&b, "  %-36s %-20s %-16s %-10s %s\n", k.ID, k.Prefix, name, budget, state)
		}
	}
	b.WriteString("\n" + styles.muted.Render("n = new key · r = revoke selected · enter = select\n"))
	b.WriteString(styles.muted.Render("Full keys are shown exactly once; list shows prefixes only.\n"))
	return b.String()
}

// selectedKeyID resolves the selected key id (used by the revoke flow).
func (m *Model) selectedKeyID() string {
	if m.revokeIndex >= 0 && m.revokeIndex < len(m.keys) {
		return m.keys[m.revokeIndex].ID
	}
	return ""
}
