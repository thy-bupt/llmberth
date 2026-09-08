package usage

import (
	"strings"
	"testing"
	"time"

	"github.com/thy-bupt/llmberth/internal/admin"
)

func sampleResponse() admin.UsageResponse {
	return admin.UsageResponse{
		By:    "model",
		Since: time.Now().AddDate(0, -1, 0).Format(time.RFC3339),
		Points: []admin.UsagePoint{
			{Group: "fake-chat", Requests: 3, PromptTokens: 6, CompletionTokens: 21, CostUSD: 0.00069},
			{Group: "gpt-4o-mini", Requests: 1, PromptTokens: 10, CompletionTokens: 5, CostUSD: 0.000004},
		},
		Note:       "cost figures are estimates (models.yaml price table)",
		MonthSpend: 0.0007,
	}
}

func TestSummarizeTotals(t *testing.T) {
	s := Summarize(sampleResponse(), 20)
	if s.TotalRequests != 4 || s.TotalTokens != 42 {
		t.Fatalf("totals wrong: req=%d tokens=%d", s.TotalRequests, s.TotalTokens)
	}
	if len(s.Rows) != 2 || s.Rows[0].Group != "fake-chat" {
		t.Fatalf("rows wrong: %+v", s.Rows)
	}
	if s.BudgetState() != BudgetOK || s.BudgetRatio < 0.00003 || s.BudgetRatio > 0.00004 {
		t.Fatalf("budget state wrong: %+v", s)
	}
}

func TestBudgetStates(t *testing.T) {
	cases := []struct {
		monthSpend float64
		budget     float64
		want       BudgetStatus
	}{
		{1, 10, BudgetOK},
		{8.1, 10, BudgetWarn}, // 81% >= 80%
		{10, 10, BudgetExhausted},
		{12, 10, BudgetExhausted},
		{5, 0, BudgetOK}, // unlimited
	}
	for _, tc := range cases {
		s := Summarize(sampleResponse(), tc.budget)
		s.MonthSpendUSD = tc.monthSpend
		if tc.budget > 0 {
			s.BudgetRatio = tc.monthSpend / tc.budget
		}
		if got := s.BudgetState(); got != tc.want {
			t.Errorf("spend %.1f/budget %.1f: got %v want %v", tc.monthSpend, tc.budget, got, tc.want)
		}
	}
}

func TestRenderMarksEstimates(t *testing.T) {
	s := Summarize(sampleResponse(), 20)
	var b strings.Builder
	s.Render(&b)
	out := b.String()
	if !strings.Contains(out, "(estimate)") {
		t.Fatalf("render must label costs as estimates:\\n%s", out)
	}
	if !strings.Contains(out, "fake-chat") || !strings.Contains(out, "3") {
		t.Fatalf("rows missing from render:\\n%s", out)
	}
	if !strings.Contains(out, "budget:") && !strings.Contains(out, "Monthly budget") {
		t.Fatalf("budget line missing:\\n%s", out)
	}
}

func TestExhaustedHint(t *testing.T) {
	s := Summarize(sampleResponse(), 10)
	s.MonthSpendUSD = 11
	s.BudgetRatio = 1.1
	if hint := s.BudgetHint(); !strings.Contains(hint, "budget_exhausted") {
		t.Fatalf("hint should mention rejection reason: %q", hint)
	}
}
