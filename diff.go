package minitui

import (
	"fmt"
	"strconv"
	"strings"
)

// DiffStyle holds ANSI escape codes for diff rendering.
type DiffStyle struct{ Add, Del, Hunk, Header, Meta string }

// DefaultDiffStyle uses subdued green/red backgrounds for +/- lines.
// 256-color palette: 22=dark green, 52=dark red, 231=bright white.
var DefaultDiffStyle = DiffStyle{
	Add: "\x1b[48;5;22;38;5;231m", Del: "\x1b[48;5;52;38;5;231m",
	Hunk: "\x1b[36m", Header: "\x1b[1m", Meta: "\x1b[2m",
}

// RenderDiff converts a unified diff string to ANSI-colored output.
// Pass true for showLineNum to prepend dim line numbers on each hunk line.
//
//	tui.WriteString(minitui.RenderDiff(diff))       // no line numbers
//	tui.WriteString(minitui.RenderDiff(diff, true)) // with line numbers
func RenderDiff(diff string, showLineNum ...bool) string {
	ln := len(showLineNum) > 0 && showLineNum[0]
	return renderDiff(diff, DefaultDiffStyle, ln)
}

// WriteDiff renders and writes a colored unified diff.
// Code lines (context / + / −) get full-width backgrounds:
//   • context → code-area bg 236 (from lineNumFmt)
//   • added   → green bg 22  fills entire line
//   • deleted → red bg 52    fills entire line
// Headers, hunks, and meta lines do not get background fill.
// Pass true to show line numbers.
func (t *TUI) WriteDiff(diff string, showLineNum ...bool) {
	ln := len(showLineNum) > 0 && showLineNum[0]
	rendered := RenderDiff(diff, ln)
	renderedLines := strings.Split(rendered, "\n")
	origLines := strings.Split(diff, "\n")

	var b strings.Builder
	ri := 0
	for _, orig := range origLines {
		if ri >= len(renderedLines) {
			break
		}
		rline := renderedLines[ri]
		ri++

		if rline == "" {
			b.WriteByte('\n')
			continue
		}

		b.WriteString(rline)

		// Only code lines (context / + / −) get padded to full width.
		if isDiffCodeLine(orig) {
			dw := displayWidth(stripAnsi(rline))
			if dw < t.width {
				// The background (green/red for +/−, 236 for context)
				// is still active at this point — just emit spaces.
				b.WriteString(strings.Repeat(" ", t.width-dw))
				b.WriteString(ansiReset)
			}
		}
		b.WriteByte('\n')
	}
	t.WriteString(b.String())
}

// isDiffCodeLine returns true for context (' '), added ('+'), or deleted ('-') lines.
func isDiffCodeLine(s string) bool {
	return len(s) > 0 && (s[0] == ' ' || s[0] == '+' || s[0] == '-')
}

const lineNumFmt = "\x1b[48;5;238;90m%4d \x1b[39;48;5;236m" // gutter bg 238, then code bg 236

func renderDiff(diff string, s DiffStyle, showNum bool) string {
	if diff == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(diff) + 1024)
	curLang := ""
	oldL, newL := 0, 0 // current line numbers in old / new file
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- "):
			curLang = langFromPath(line)
			b.WriteString(s.Header + line + ansiReset)
		case strings.HasPrefix(line, "@@"):
			oldL, newL = parseHunk(line)
			b.WriteString(s.Hunk + line + ansiReset)
		case strings.HasPrefix(line, "+"):
			bufNum(&b, showNum, newL)
			// \x1b[39m resets fg only — green bg persists for full-width fill.
			b.WriteString(s.Add + "+" + highlightDiffCode(line[1:], curLang) + "\x1b[39m")
			newL++
		case strings.HasPrefix(line, "-"):
			bufNum(&b, showNum, oldL)
			b.WriteString(s.Del + "-" + highlightDiffCode(line[1:], curLang) + "\x1b[39m")
			oldL++
		case isMeta(line):
			b.WriteString(s.Meta + line + ansiReset)
		default:
			if strings.HasPrefix(line, " ") {
				bufNum(&b, showNum, newL)
				b.WriteString(" " + highlightDiffCode(line[1:], curLang))
				oldL++
				newL++
			} else {
				b.WriteString(line)
			}
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// bufNum writes a dim line number if showNum is true.
func bufNum(b *strings.Builder, show bool, n int) {
	if show {
		fmt.Fprintf(b, lineNumFmt, n)
	}
}

// parseHunk extracts oldStart and newStart from "@@ -old,count +new,count @@".
func parseHunk(line string) (old, new int) {
	if i := strings.IndexByte(line, '-'); i >= 0 {
		old, _ = strconv.Atoi(parseNum(line[i+1:]))
	}
	if i := strings.IndexByte(line, '+'); i >= 0 {
		new, _ = strconv.Atoi(parseNum(line[i+1:]))
	}
	return
}

// parseNum returns the leading number from s, stopping at space or comma.
func parseNum(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == ',' {
			return s[:i]
		}
	}
	return s
}

// langFromPath extracts "go" from "a/main.go" or "b/pkg/render.py".
func langFromPath(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return ""
}

// highlightDiffCode applies syntax highlighting to code text inside
// a diff line. The SGR parameters stack: diff bg color + hl fg color
// coexist, and hlReset (\x1b[39m) only resets fg, keeping the diff bg.
func highlightDiffCode(code, lang string) string {
	if lang == "" || code == "" {
		return code
	}
	def, ok := langTable[strings.ToLower(lang)]
	if !ok {
		return code
	}
	return highlightLine(code, def)
}

var metaPrefixes = []string{
	"diff ", "index ", "new ", "old ", "rename ",
	"copy ", "similarity ", "Binary files ", "deleted file ", "new file ",
}

func isMeta(line string) bool {
	for _, p := range metaPrefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}
