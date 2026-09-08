// Package usage computes ledger summaries and budget status from admin API
// aggregates. It is shared by the CLI `usage` command, `status` output and
// the TUI Dashboard — one calculation, three view layers (plan §3.2 iron
// rule: the TUI is only a view on internal packages).
package usage

import (
	"fmt"
	"io"
	"strings"

	"github.com/THY17308111153/llmberth/internal/admin"
)

// Row is one aggregation group (model or key prefix).
type Row struct {
	Group            string
	Requests         int64
	PromptTokens     int64
	CompletionTokens int64
	CostUSD          float64 // estimate
}

// BudgetStatus is the derived budget health.
type BudgetStatus int

// Budget states in ascending severity.
const (
	BudgetOK BudgetStatus = iota
	BudgetWarn            // >= 80% of the monthly budget consumed
	BudgetExhausted       // >= 100%
)

func (b BudgetStatus) String() string {
	switch b {
	case BudgetWarn:
		return "warn"
	case BudgetExhausted:
		return "exhausted"
	default:
		return "ok"
	}
}

// Summary is the computed view over one admin usage response.
type Summary struct {
	By             string // "", "key", "model"
	Rows           []Row
	Since          string
	TotalRequests  int64
	TotalTokens    int64
	TotalCostUSD   float64
	MonthSpendUSD  float64
	BudgetMonthly  float64 // 0 = unlimited (not configured)
	BudgetRatio    float64 // MonthSpendUSD / BudgetMonthly, 0 when unlimited
	EstimateNote   string
}

// BudgetState derives the budget health from the current ratio — a method,
// not a stored field, so the state can never drift from the numbers.
func (s Summary) BudgetState() BudgetStatus {
	if s.BudgetMonthly <= 0 {
		return BudgetOK
	}
	switch {
	case s.BudgetRatio >= 1:
		return BudgetExhausted
	case s.BudgetRatio >= WarnThreshold:
		return BudgetWarn
	default:
		return BudgetOK
	}
}

// WarnThreshold is the consumption ratio at which budget warnings start.
const WarnThreshold = 0.80

// Summarize turns an admin usage response into a Summary. monthlyUSD is the
// project's monthly budget from the manifest (0 disables budgeting).
func Summarize(res admin.UsageResponse, monthlyUSD float64) Summary {
	s := Summary{
		By:            res.By,
		Since:         res.Since,
		MonthSpendUSD: res.MonthSpend,
		BudgetMonthly: monthlyUSD,
		EstimateNote:  res.Note,
	}
	for _, p := range res.Points {
		s.Rows = append(s.Rows, Row{
			Group:            p.Group,
			Requests:         p.Requests,
			PromptTokens:     p.PromptTokens,
			CompletionTokens: p.CompletionTokens,
			CostUSD:          p.CostUSD,
		})
		s.TotalRequests += p.Requests
		s.TotalTokens += p.PromptTokens + p.CompletionTokens
		s.TotalCostUSD += p.CostUSD
	}
	if monthlyUSD > 0 {
		s.BudgetRatio = s.MonthSpendUSD / monthlyUSD
	}
	return s
}

// Render prints a friendly table plus the budget line. Costs are labeled
// estimates everywhere (plan §4.2).
func (s Summary) Render(out io.Writer) {
	fmt.Fprintln(out, "")
	if s.By == "" {
		fmt.Fprintf(out, "Usage since %s (all keys)\n", s.Since)
	} else {
		fmt.Fprintf(out, "Usage since %s (by %s)\n", s.Since, s.By)
	}
	fmt.Fprintf(out, "%-24s %10s %14s %18s %12s\n", "GROUP", "REQUESTS", "PROMPT_TOKENS", "COMPLETION_TOKENS", "COST_USD")
	for _, r := range s.Rows {
		group := r.Group
		if group == "" {
			group = "(all keys)"
		}
		fmt.Fprintf(out, "%-24s %10d %14d %18d %12.6f\n",
			truncate(group, 24), r.Requests, r.PromptTokens, r.CompletionTokens, r.CostUSD)
	}
	if len(s.Rows) == 0 {
		fmt.Fprintln(out, "(no usage recorded in this window)")
	}
	fmt.Fprintf(out, "\nTotal: %d request(s), %d token(s), $%.6f (estimate)\n",
		s.TotalRequests, s.TotalTokens, s.TotalCostUSD)

	// Budget line (plan: status/usage surface the consumption ratio).
	if s.BudgetMonthly <= 0 {
		fmt.Fprintln(out, "Monthly budget: not configured (0 disables budgeting)")
		return
	}
	remaining := s.BudgetMonthly - s.MonthSpendUSD
	if remaining < 0 {
		remaining = 0
	}
	// Spend/remaining carry full precision: sub-dollar spends must not
	// render as $0.00 (plan §4.2: costs are estimates, show them honestly).
	fmt.Fprintf(out, "Monthly budget: $%.2f — spent $%.6f (%.2f%%), remaining $%.6f [%s]\n",
		s.BudgetMonthly, s.MonthSpendUSD, s.BudgetRatio*100, remaining, s.BudgetState())
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// BudgetLine renders just the budget state (used by status/TUI).
func (s Summary) BudgetLine() string {
	var b strings.Builder
	RenderBudget(&b, s)
	return b.String()
}

// RenderBudget writes the compact budget line only.
func RenderBudget(out io.Writer, s Summary) {
	if s.BudgetMonthly <= 0 {
		fmt.Fprintln(out, "budget: not configured")
		return
	}
	fmt.Fprintf(out, "budget: $%.6f spent of $%.2f (%.2f%%) [%s]",
		s.MonthSpendUSD, s.BudgetMonthly, s.BudgetRatio*100, s.BudgetState())
	if s.BudgetState() != BudgetOK {
		fmt.Fprintf(out, " — %s", s.BudgetHint())
	}
	fmt.Fprintln(out)
}

// BudgetHint is the operator-facing hint for warn/exhausted states.
func (s Summary) BudgetHint() string {
	switch s.BudgetState() {
	case BudgetExhausted:
		return "requests are being rejected with 429 budget_exhausted"
	case BudgetWarn:
		return "approaching the monthly budget"
	default:
		return ""
	}
}
