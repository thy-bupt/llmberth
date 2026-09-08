package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thy-bupt/llmberth/internal/runtime"
	"github.com/thy-bupt/llmberth/internal/usage"
)

// refreshDashboard pulls stack status and usage windows through internal
// packages only. Failures surface in the view, never crash the TUI.
func (m *Model) refreshDashboard(ctx context.Context) {
	m.dashErr = ""

	// Stack health (internal/runtime).
	raw, err := runtime.StatusRaw(ctx, m.projectDir)
	if err != nil {
		m.dashErr = "stack: " + err.Error()
		m.svcRows = nil
		m.healthOK = false
	} else {
		rows, err := runtime.ParseStatus(raw)
		if err != nil {
			m.dashErr = "stack: " + err.Error()
			m.svcRows = nil
			m.healthOK = false
		} else {
			m.svcRows = nil
			m.healthOK = true
			for _, r := range rows {
				m.svcRows = append(m.svcRows, runtimeRow{service: r.Service, state: r.State, published: r.Published})
				if r.State != "running" {
					m.healthOK = false
				}
			}
		}
	}

	// Usage windows (internal/admin + internal/usage aggregation).
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	weekStart := dayStart.AddDate(0, 0, -6)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	m.dashDay = m.summaryWindow(ctx, "today", dayStart)
	m.dashWeek = m.summaryWindow(ctx, "this week", weekStart)
	m.dashMonth = m.summaryWindow(ctx, "this month", monthStart)

	s, err := usageSummarizeMonth(ctx, m, monthStart)
	if err != nil {
		m.dashBudget = "budget: unavailable (" + err.Error() + ")"
	} else {
		m.dashBudget = s.BudgetLine()
	}
}

// summaryWindow pulls one admin usage window and converts it to the view
// subset (the aggregation itself runs in internal/usage).
func (m *Model) summaryWindow(ctx context.Context, title string, since time.Time) usageSummary {
	res, err := m.client.Usage(ctx, "", since)
	if err != nil {
		return usageSummary{title: title}
	}
	s := usage.Summarize(res, m.manifest.Budget.MonthlyUSD)
	return usageSummary{
		title:    title,
		totalReq: s.TotalRequests,
		totalTok: s.TotalTokens,
		cost:     s.TotalCostUSD,
	}
}

// usageSummarizeMonth is a small helper so the dashboard can reuse the
// budget line rendering without duplicating aggregation logic.
func usageSummarizeMonth(ctx context.Context, m *Model, since time.Time) (usage.Summary, error) {
	res, err := m.client.Usage(ctx, "", since)
	if err != nil {
		return usage.Summary{}, err
	}
	return usage.Summarize(res, m.manifest.Budget.MonthlyUSD), nil
}

// dashboardView renders the health lights, usage windows and budget line.
func (m *Model) dashboardView() string {
	var b strings.Builder

	b.WriteString(styles.title.Render("Stack health\n"))
	if len(m.svcRows) == 0 {
		b.WriteString("  " + styles.warn.Render("! stack not running — `llmberth up` (or docker compose up -d)\n"))
	} else {
		for _, r := range m.svcRows {
			light := styles.ok.Render("●")
			if r.state != "running" {
				light = styles.bad.Render("●")
			}
			fmt.Fprintf(&b, "  %s %-10s %-28s %s\n", light, r.service, r.state, styles.muted.Render(r.published))
		}
	}

	b.WriteString("\n" + styles.title.Render("Usage (estimates)\n"))
	fmt.Fprintf(&b, "  %-12s %10s %12s %14s\n", "window", "requests", "tokens", "cost_usd")
	for _, w := range []usageSummary{m.dashDay, m.dashWeek, m.dashMonth} {
		fmt.Fprintf(&b, "  %-12s %10d %12d %14.6f\n", w.title, w.totalReq, w.totalTok, w.cost)
	}

	b.WriteString("\n" + styles.title.Render("Budget\n"))
	if m.dashBudget != "" {
		b.WriteString("  " + m.dashBudget + "\n")
	} else {
		b.WriteString("  (loading…)\n")
	}

	b.WriteString("\n" + styles.muted.Render("All cost figures are estimates (models.yaml price table).\n"))
	if m.dashErr != "" {
		b.WriteString("\n" + styles.bad.Render("! " + m.dashErr + "\n"))
	}
	return b.String()
}
