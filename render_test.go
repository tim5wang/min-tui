package minitui

import (
	"strings"
	"testing"
)

func TestBlockquoteExpandsTabsBeforeBackgroundPadding(t *testing.T) {
	tui := &TUI{width: 24}

	got := tui.renderLine(">   ✓ 1595\t\t}", nil, false)

	if strings.ContainsRune(got, '\t') {
		t.Fatalf("blockquote rendered raw tab: %q", got)
	}
	if displayWidth(stripAnsi(got)) != tui.width {
		t.Fatalf("rendered width = %d, want %d; line=%q", displayWidth(stripAnsi(got)), tui.width, got)
	}
	if !strings.HasPrefix(got, "\x1b[100m") {
		t.Fatalf("blockquote missing background prefix: %q", got)
	}
}

func TestCodeBackgroundLineExpandsTabs(t *testing.T) {
	got := bgPadLine("\t\treturn nil", ansiCodeBg, 24)

	if strings.ContainsRune(got, '\t') {
		t.Fatalf("code line rendered raw tab: %q", got)
	}
	if displayWidth(stripAnsi(got)) != 24 {
		t.Fatalf("rendered width = %d, want 24; line=%q", displayWidth(stripAnsi(got)), got)
	}
	if !strings.HasPrefix(got, ansiCodeBg) {
		t.Fatalf("code line missing background prefix: %q", got)
	}
}

func TestCodeBackgroundLineKeepsBackgroundAfterReset(t *testing.T) {
	got := bgPadLine(ansiDim+"return nil"+ansiReset, ansiCodeBg, 24)

	if displayWidth(stripAnsi(got)) != 24 {
		t.Fatalf("rendered width = %d, want 24; line=%q", displayWidth(stripAnsi(got)), got)
	}
	if !strings.Contains(got, ansiReset+ansiCodeBg+strings.Repeat(" ", 14)+ansiReset) {
		t.Fatalf("background was not restored before padding: %q", got)
	}
}

func TestWrapToWidthExpandsTabs(t *testing.T) {
	lines := wrapToWidth("\t\treturn strings.Repeat(value, 3)", 16)

	for _, line := range lines {
		if strings.ContainsRune(line, '\t') {
			t.Fatalf("wrapped line rendered raw tab: %q", line)
		}
	}
}

func TestRenderedDiffLinesArePaddedWhenWrittenAsAnsiText(t *testing.T) {
	tui := &TUI{width: 48}
	diff := "--- a/main.go\n" +
		"+++ b/main.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" func main() {\n" +
		"-\treturn nil\n" +
		"+\treturn fmt.Errorf(\"nope\")\n" +
		" }\n"

	for _, line := range strings.Split(RenderDiff(diff, true), "\n") {
		tui.emitLine(line)
	}

	for _, line := range tui.outAnsi {
		plain := stripAnsi(line)
		if !strings.Contains(plain, "return") && !strings.Contains(plain, "func main") && !strings.HasSuffix(plain, " }") {
			continue
		}
		if displayWidth(plain) != tui.width {
			t.Fatalf("diff code line width = %d, want %d; line=%q", displayWidth(plain), tui.width, line)
		}
		if !strings.HasSuffix(line, ansiReset) {
			t.Fatalf("diff code line should end with reset after padding: %q", line)
		}
	}
}

func TestCodeFenceMarkersAreNotRendered(t *testing.T) {
	cases := []struct {
		name  string
		open  string
		close string
		code  string
	}{
		{name: "plain backticks", open: "```", close: "```", code: "return nil"},
		{name: "known language", open: "```go", close: "```", code: "return nil"},
		{name: "unknown language", open: "```foobar", close: "```", code: "return nil"},
		{name: "tildes", open: "~~~", close: "~~~", code: "return nil"},
		{name: "tilde language", open: "~~~python", close: "~~~", code: "return None"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tui := &TUI{width: 32}

			tui.emitLine(tc.open)
			tui.emitLine(tc.code)
			tui.emitLine(tc.close)

			if len(tui.outAnsi) != 1 {
				t.Fatalf("rendered lines = %d, want 1; lines=%q", len(tui.outAnsi), tui.outAnsi)
			}
			if strings.Contains(tui.outAnsi[0], "```") || strings.Contains(tui.outAnsi[0], "~~~") {
				t.Fatalf("code fence marker rendered: %q", tui.outAnsi[0])
			}
			if !strings.Contains(stripAnsi(tui.outAnsi[0]), tc.code) {
				t.Fatalf("code content missing: %q", tui.outAnsi[0])
			}
			if !strings.HasPrefix(tui.outAnsi[0], ansiCodeBg) {
				t.Fatalf("code content missing background: %q", tui.outAnsi[0])
			}
		})
	}
}
