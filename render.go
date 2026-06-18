package minitui

import (
	"fmt"
	"os"
	"strings"
)

// ── input box ────────────────────────────────────────────────────

func (t *TUI) renderInputBox() {
	bc := t.borderColor
	brd := bc + strings.Repeat("─", t.width) + ansiReset

	t.writeRow(t.inTopBorder(), brd)

	t.buildVisualLines()
	vs := t.inContentStart()
	for i := 0; i < t.inHeight; i++ {
		idx := t.inScrollRow + i
		if idx < len(t.visLines) {
			s := pad(t.visLines[idx].text, t.width)
			t.writeRow(vs+i, s)
		} else {
			t.writeRow(vs+i, "")
		}
	}
	t.writeRow(t.inBotBorder(), brd)
}

// ── status bar ───────────────────────────────────────────────────

func (t *TUI) renderStatus() {
	s := "\x1b[2m" // dim by default
	switch t.statusStyle {
	case StatusInfo:
		s = "\x1b[2;37m" // dim + light fg
	case StatusWarning:
		s = "\x1b[2;33m" // dim + yellow fg
	case StatusError:
		s = "\x1b[2;31m" // dim + red fg
	case StatusSuccess:
		s = "\x1b[2;32m" // dim + green fg
	}
	text := t.statusText
	// Truncate by display width so CJK/emoji are not split mid-rune.
	dw := displayWidth(text)
	if dw > t.width {
		var b strings.Builder
		cur := 0
		for _, r := range text {
			rw := runeWidth(r)
			if cur+rw > t.width {
				break
			}
			b.WriteRune(r)
			cur += rw
		}
		text = b.String()
		dw = cur
	}
	t.writeRow(t.statusRow(),
		s+text+ansiReset+strings.Repeat(" ", t.width-dw))
}

// ── word wrap ───────────────────────────────────────────────────

// stripAnsi removes ANSI escape sequences (CSI + SGR), returning plain text.
func stripAnsi(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			// Skip parameter (0x30-0x3F) and intermediate (0x20-0x2F) bytes.
			for j < len(s) && ((s[j] >= 0x30 && s[j] <= 0x3F) || (s[j] >= 0x20 && s[j] <= 0x2F)) {
				j++
			}
			// Final byte is 0x40-0x7E.  Fallback: any remaining byte ends the sequence.
			if j < len(s) {
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// takeRunesWidth returns the rune index `end` such that runes[start:end]
// fits within maxWidth display cells. Also returns the actual width.
func takeRunesWidth(rs []rune, start, maxWidth int) (end, w int) {
	w = 0
	for end = start; end < len(rs); end++ {
		rl := rs[end]
		rw := runeWidth(rl)
		if rl == '\t' { rw = 4 }
		if w+rw > maxWidth { break }
		w += rw
	}
	return end, w
}

// wrapToWidth splits an ANSI-styled line into visual lines of width.
// The first line keeps the full ANSI prefix; continuation lines get an
// indent (2 spaces) and re-apply the initial ANSI style.
func wrapToWidth(ansi string, width int) []string {
	if width < 4 { return []string{ansi} }
	plain := stripAnsi(ansi)
	if displayWidth(plain) <= width {
		return []string{ansi}
	}
	// Extract initial ANSI prefix (codes before first visible char).
	var prefix strings.Builder
	rest := ansi
	for strings.HasPrefix(rest, "\x1b[") {
		end := len("\x1b[")
		for end < len(rest) && ((rest[end] >= 0x30 && rest[end] <= 0x3F) || (rest[end] >= 0x20 && rest[end] <= 0x2F)) {
			end++
		}
		if end >= len(rest) {
			break
		}
		end++ // consume final byte
		prefix.WriteString(rest[:end])
		rest = rest[end:]
	}

	runes := []rune(plain)
	indent := "  "
	iw := displayWidth(indent)
	firstW := width
	contW := width - iw
	if contW < 1 { contW = 1 }

	var out []string
	pos := 0
	end, _ := takeRunesWidth(runes, pos, firstW)
	if end == pos { end = pos + 1 }
	out = append(out, prefix.String()+string(runes[pos:end])+ansiReset)
	pos = end

	for pos < len(runes) {
		end, _ = takeRunesWidth(runes, pos, contW)
		if end == pos { end = pos + 1 }
		out = append(out, indent+prefix.String()+string(runes[pos:end])+ansiReset)
		pos = end
	}
	return out
}

func (t *TUI) writeRow(row int, s string) {
	if row < 0 || row >= t.height {
		return
	}
	// \x1b[0m before \x1b[K ensures no stale background colors linger.
	fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;1H\x1b[0m\x1b[K%s\x1b[K\x1b[u", row+1, s)
}

// ── markdown → ANSI ─────────────────────────────────────────────

func (t *TUI) renderLine(raw string, tableBuf *[]string, forceDim bool) string {
	if t.customRender != nil {
		if tableBuf != nil && isTableLine(raw) {
			*tableBuf = append(*tableBuf, raw)
			return ""
		}
		r := t.customRender(raw)
		if forceDim {
			r = ansiDim + r + ansiReset
		}
		return r
	}
	if tableBuf != nil && isTableLine(raw) {
		*tableBuf = append(*tableBuf, raw)
		return ""
	}
	// Blockquote: > text (gray bg, content only, no > prefix).
	if isBlockquote(raw) {
		content := raw
		if strings.HasPrefix(content, "> ") {
			content = content[2:]
		} else if content == ">" {
			content = ""
		}
		return "\x1b[100m" + padTo(content, t.width) + ansiReset
	}
	if forceDim {
		return ansiDim + raw + ansiReset
	}
	return t.renderInline(raw)
}

// ── table ────────────────────────────────────────────────────────

func renderTable(buf []string, width int) string {
	if len(buf) < 2 || !isTableSep(buf[1]) {
		return strings.Join(buf, "\n")
	}
	hdr := splitCols(buf[0])
	var rows [][]string
	for i := 2; i < len(buf); i++ {
		rows = append(rows, splitCols(buf[i]))
	}
	ncols := len(hdr)
	for _, r := range rows {
		if len(r) < ncols {
			ncols = len(r)
		}
	}
	widths := make([]int, ncols)
	for i := 0; i < ncols; i++ {
		w := displayWidth(strings.TrimSpace(hdr[i]))
		for _, r := range rows {
			if i < len(r) {
				if dw := displayWidth(strings.TrimSpace(r[i])); dw > w {
					w = dw
				}
			}
		}
		widths[i] = w + 2
	}
	var out []string
	out = append(out, tableRow(hdr, widths, true))
	sep := make([]string, ncols)
	for i := 0; i < ncols; i++ {
		sep[i] = strings.Repeat("─", widths[i])
	}
	out = append(out, tableRow(sep, widths, false))
	for _, r := range rows {
		out = append(out, tableRow(r, widths, false))
	}
	return strings.Join(out, "\n")
}

func tableRow(cols []string, widths []int, bold bool) string {
	var b strings.Builder
	b.WriteString("│ ")
	for i := 0; i < len(widths); i++ {
		txt := ""
		if i < len(cols) {
			txt = cols[i]
		}
		if bold {
			b.WriteString(ansiBold + pad(txt, widths[i]) + ansiReset)
		} else {
			b.WriteString(pad(txt, widths[i]))
		}
		b.WriteString(" │ ")
	}
	return b.String()
}

func splitCols(s string) []string {
	s = strings.Trim(s, "|")
	p := strings.Split(s, "|")
	for i := range p {
		p[i] = strings.TrimSpace(p[i])
	}
	return p
}

func isTableLine(s string) bool { return strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") }
func isTableSep(s string) bool {
	if !isTableLine(s) { return false }
	for _, p := range strings.Split(strings.Trim(s, "|"), "|") {
		if strings.Trim(p, "-: ") != "" { return false }
	}
	return true
}
func isCodeFence(s string) bool { return strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~") }
func isBlockquote(s string) bool { return strings.HasPrefix(s, "> ") || s == ">" }

func isHeadingLine(s string) bool {
	if len(s) == 0 || s[0] != '#' { return false }
	lvl := 0
	for lvl < len(s) && s[lvl] == '#' { lvl++ }
	return lvl <= 6 && (lvl == len(s) || s[lvl] == ' ')
}

// ── inline markdown ─────────────────────────────────────────────

func (t *TUI) renderInline(s string) string {
	if s == "" { return s }
	if s[0] == '#' {
		lvl := 0
		for lvl < len(s) && s[lvl] == '#' { lvl++ }
		if lvl <= 6 && (lvl == len(s) || s[lvl] == ' ') {
			if t.showMarks {
				return ansiBold + s + ansiReset
			}
			return ansiBold + strings.TrimLeft(s[lvl:], " ") + ansiReset
		}
	}
	var b strings.Builder
	b.Grow(len(s) + 32)
	for i := 0; i < len(s); {
		if i+3 < len(s) && s[i] == '*' && s[i+1] == '*' {
			if e := strings.Index(s[i+2:], "**"); e >= 0 {
				b.WriteString(ansiBold); b.WriteString(s[i+2 : i+2+e])
				b.WriteString(ansiReset); i += 2 + e + 2; continue
			}
		}
		if i+2 < len(s) && s[i] == '*' && s[i+1] != '*' {
			if e := strings.Index(s[i+1:], "*"); e > 0 {
				b.WriteString(ansiItalic); b.WriteString(s[i+1 : i+1+e])
				b.WriteString(ansiReset); i += 1 + e + 1; continue
			}
		}
		if s[i] == '`' {
			if e := strings.Index(s[i+1:], "`"); e >= 0 {
				b.WriteString(ansiDim); b.WriteString(s[i+1 : i+1+e])
				b.WriteString(ansiReset); i += 1 + e + 1; continue
			}
		}
		b.WriteByte(s[i]); i++
	}
	return b.String()
}

// ── display width ────────────────────────────────────────────────

func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r == '\t' {
			w += 4
		} else {
			w += runeWidth(r)
		}
	}
	return w
}

func runeWidth(r rune) int {
	if r == 0 { return 0 }
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, r >= 0x3400 && r <= 0x4DBF,
		r >= 0xF900 && r <= 0xFAFF, r >= 0x3000 && r <= 0x303F,
		r >= 0xFF01 && r <= 0xFF60, r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0xAC00 && r <= 0xD7AF, r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0x2EFF, r >= 0x2F00 && r <= 0x2FDF,
		r >= 0xFE30 && r <= 0xFE4F, r >= 0x1F300 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x2FA1F:
		return 2
	}
	if r < 32 || (r >= 0x7F && r <= 0x9F) { return 0 }
	return 1
}

func pad(s string, w int) string {
	dw := displayWidth(s)
	if dw >= w {
		var b strings.Builder
		cur := 0
		for _, r := range s {
			rw := runeWidth(r)
			if r == '\t' {
				rw = 4
			}
			if cur+rw > w {
				break
			}
			b.WriteRune(r)
			cur += rw
		}
		for cur < w {
			b.WriteByte(' ')
			cur++
		}
		return b.String()
	}
	s = strings.ReplaceAll(s, "\t", "    ")
	return s + strings.Repeat(" ", w-dw)
}
