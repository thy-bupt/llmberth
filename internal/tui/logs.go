package tui

import (
	"context"
	"io"
	"strings"

	"github.com/THY17308111153/llmberth/internal/runtime"
)

// refreshLogs starts the streaming log reader (internal/runtime) and pushes
// lines into the ring buffer; the view filters by level client-side (the
// filter is view concern only — the data path is shared with the CLI).
func (m *Model) refreshLogs(ctx context.Context) {
	go func() {
		pr, pw := io.Pipe()
		go func() {
			defer pw.Close() //nolint:errcheck // pipe close is terminal
			err := runtime.Logs(ctx, m.projectDir, runtime.ServiceApp, true, pw, pw)
			if err != nil && ctx.Err() == nil {
				m.logTailMsg = err.Error()
			}
		}()
		buf := make([]byte, 64*1024)
		line := ""
		for {
			n, err := pr.Read(buf)
			line += string(buf[:n])
			for {
				idx := strings.IndexByte(line, '\n')
				if idx < 0 {
					break
				}
				m.appendLog(strings.TrimRight(line[:idx], "\r"))
				line = line[idx+1:]
			}
			if err != nil {
				if line != "" {
					m.appendLog(line)
				}
				return
			}
		}
	}()
}

// appendLog keeps a bounded ring buffer (view internal).
func (m *Model) appendLog(line string) {
	const maxLines = 500
	m.logsBuf = append(m.logsBuf, line)
	if len(m.logsBuf) > maxLines {
		m.logsBuf = m.logsBuf[len(m.logsBuf)-maxLines:]
	}
}

// levelMatches implements the level filter for structured JSON log lines and
// plain text fallback.
func levelMatches(line, filter string) bool {
	switch filter {
	case "", "ALL":
		return true
	case "ERROR":
		return strings.Contains(line, `"level":"ERROR"`) || strings.Contains(line, `"level":"error"`) || strings.Contains(line, " ERROR ")
	case "WARN":
		return strings.Contains(line, `"level":"WARN"`) || strings.Contains(line, `"level":"warn"`) || strings.Contains(line, " WARN ")
	case "INFO":
		return strings.Contains(line, `"level":"INFO"`) || strings.Contains(line, `"level":"info"`)
	case "DEBUG":
		return strings.Contains(line, `"level":"DEBUG"`) || strings.Contains(line, `"level":"debug"`)
	}
	return true
}

// logsView renders the ring buffer with the active filter applied.
func (m *Model) logsView() string {
	var b strings.Builder
	b.WriteString(styles.title.Render("Logs (app, streaming) — f: filter "))
	b.WriteString(styles.ok.Render(m.logFilter))
	b.WriteString(styles.muted.Render(" | 1/2/3 or tab to leave\n"))
	if m.logTailMsg != "" {
		b.WriteString(styles.warn.Render("! " + m.logTailMsg + "\n"))
	}
	shown := 0
	for i := len(m.logsBuf) - 1; i >= 0; i-- {
		line := m.logsBuf[i]
		if !levelMatches(line, m.logFilter) {
			continue
		}
		var styled string
		if strings.Contains(line, `"level":"ERROR"`) || strings.Contains(line, `"level":"error"`) {
			styled = styles.bad.Render(line)
		} else if strings.Contains(line, `"level":"WARN"`) || strings.Contains(line, `"level":"warn"`) {
			styled = styles.warn.Render(line)
		} else {
			styled = styles.muted.Render(line)
		}
		b.WriteString(styled + "\n")
		shown++
		if shown >= 80 {
			break
		}
	}
	if shown == 0 {
		b.WriteString("  (no matching log lines yet)\n")
	}
	return b.String()
}
