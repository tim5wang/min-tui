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

func TestWrapToWidthExpandsTabs(t *testing.T) {
	lines := wrapToWidth("\t\treturn strings.Repeat(value, 3)", 16)

	for _, line := range lines {
		if strings.ContainsRune(line, '\t') {
			t.Fatalf("wrapped line rendered raw tab: %q", line)
		}
	}
}
