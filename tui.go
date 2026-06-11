// Package minitui provides a lightweight TUI for coding agents with streaming
// output, multi-line input, markdown rendering, and a status bar.
//
// Rendering model:
//   - The entire terminal screen is a virtual canvas (live-rendered).
//   - Output that fits on screen is rendered via absolute positioning.
//   - When output exceeds the visible area, the oldest lines are "committed"
//     to the terminal scrollback via a temporary scroll region, keeping them
//     accessible with native terminal scrolling.
//   - The input box and status bar are always redrawn at the bottom.
package minitui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

// MaxInputHeight is the default maximum visible lines in the input box.
const MaxInputHeight = 8

// StatusStyle defines the visual style of the status bar.
type StatusStyle int

const (
	StatusDefault StatusStyle = iota
	StatusInfo
	StatusWarning
	StatusError
	StatusSuccess
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiItalic = "\x1b[3m"
)

// EventType describes what kind of event occurred.
type EventType int

const (
	EventSubmit    EventType = iota // user submitted input
	EventResize                      // terminal resized
	EventInterrupt                  // user pressed Ctrl+C
)

// Event is sent on the event channel when subscribed.
type Event struct {
	Type   EventType
	Input  string
	Width  int
	Height int
}

// Config holds optional settings.
type Config struct {
	EventCh      chan<- Event
	RenderLine   func(string) string
	BorderColor  string
	MaxInputRows int
}

// TUI is the main text user interface.
type TUI struct {
	mu sync.Mutex

	stdinFd  int
	stdoutFd int
	oldState *term.State
	width    int
	height   int

	// Output buffer: all ANSI-rendered screen lines ever written.
	outAnsi  []string
	// pushed counts how many lines have been committed to terminal scrollback.
	pushed   int

	pendingRaw  []byte
	inCodeBlock bool
	tableBuf    []string

	// Input editor.
	inLines     [][]rune
	inCursorRow int
	inCursorCol int
	inHeight    int
	inScrollRow int

	// Status bar.
	statusText  string
	statusStyle StatusStyle

	// Configuration.
	eventCh      chan<- Event
	customRender func(string) string
	borderColor  string
	maxInputRows int

	// Keyboard.
	keyBuf []byte
	sigCh  chan os.Signal
}

// New creates a TUI with default settings.
func New() (*TUI, error) { return NewWithConfig(Config{}) }

// NewWithConfig creates a TUI with custom configuration.
func NewWithConfig(cfg Config) (*TUI, error) {
	stdinFd := int(os.Stdin.Fd())
	stdoutFd := int(os.Stdout.Fd())

	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return nil, fmt.Errorf("minitui: make raw: %w", err)
	}
	w, h, err := term.GetSize(stdoutFd)
	if err != nil {
		term.Restore(stdinFd, oldState)
		return nil, fmt.Errorf("minitui: get size: %w", err)
	}
	if h < 5 {
		term.Restore(stdinFd, oldState)
		return nil, fmt.Errorf("minitui: terminal too small (need ≥5 rows)")
	}

	maxRows := cfg.MaxInputRows
	if maxRows <= 0 {
		maxRows = MaxInputHeight
	}
	bc := cfg.BorderColor
	if bc == "" {
		bc = "\x1b[2m"
	}

	t := &TUI{
		stdinFd:      stdinFd, stdoutFd: stdoutFd,
		oldState:     oldState, width: w, height: h,
		inLines:      [][]rune{{}}, inHeight: 1,
		keyBuf:       make([]byte, 4096),
		sigCh:        make(chan os.Signal, 1),
		eventCh:      cfg.EventCh,
		customRender: cfg.RenderLine,
		borderColor:  bc,
		maxInputRows: maxRows,
	}

	fmt.Fprint(os.Stdout, "\x1b[?2004h\x1b[>1u\x1b[>4;2m")
	fmt.Fprint(os.Stdout, "\x1b[?25l")
	fmt.Fprint(os.Stdout, "\x1b[r") // full-screen scroll region (virtual rendering)
	signal.Notify(t.sigCh, syscall.SIGWINCH)

	t.fullDraw()
	return t, nil
}

// Close restores the terminal.
func (t *TUI) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprint(os.Stdout, "\x1b[r") // reset scroll region
	fmt.Fprint(os.Stdout, "\x1b[?2004l\x1b[<u\x1b[>4;0m\x1b[?25h")
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
	term.Restore(t.stdinFd, t.oldState)
	signal.Stop(t.sigCh)
}

// ── layout ───────────────────────────────────────────────────────

func (t *TUI) outputRows() int { return t.height - t.inHeight - 3 }
func (t *TUI) inTopBorder() int  { return t.outputRows() + 1 }
func (t *TUI) inContentStart() int { return t.inTopBorder() + 1 }
func (t *TUI) inBotBorder() int { return t.inContentStart() + t.inHeight }
func (t *TUI) statusRow() int  { return t.height - 1 }

// ── full draw ────────────────────────────────────────────────────

func (t *TUI) fullDraw() {
	t.pushed = 0 // reset — all lines are re-rendered on screen
	fmt.Fprint(os.Stdout, "\x1b[H\x1b[J")

	t.renderOutputScreen()
	t.renderInputBox()
	t.renderStatus()
	t.showCursor()
}

// renderOutputScreen renders the visible portion of the output buffer.
func (t *TUI) renderOutputScreen() {
	vis := t.outputRows()
	start := len(t.outAnsi) - vis
	if start < 0 {
		start = 0
	}
	for i := 0; i < vis; i++ {
		if i+start < len(t.outAnsi) {
			t.writeRow(i, t.outAnsi[i+start])
		} else {
			t.writeRow(i, "")
		}
	}
}

// ── incremental output (on Write) ────────────────────────────────

// Write appends data to the output area.  Safe for concurrent use.
func (t *TUI) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.appendOutput(data)
	t.renderAfterWrite()
	return len(data), nil
}

func (t *TUI) WriteString(s string) (int, error) { return t.Write([]byte(s)) }

func (t *TUI) appendOutput(data []byte) {
	t.pendingRaw = append(t.pendingRaw, data...)
	for {
		idx := indexByte(t.pendingRaw, '\n')
		if idx < 0 {
			break
		}
		t.emitLine(string(t.pendingRaw[:idx]))
		t.pendingRaw = t.pendingRaw[idx+1:]
	}
}

func (t *TUI) emitLine(raw string) {
	if len(t.tableBuf) > 0 && !isTableLine(raw) {
		t.flushTable()
	}
	if isCodeFence(raw) {
		t.inCodeBlock = !t.inCodeBlock
		t.appendRendered(t.renderLine(raw, nil, true))
		return
	}
	var ansi string
	if t.inCodeBlock {
		ansi = t.renderLine(raw, nil, true)
	} else {
		ansi = t.renderLine(raw, &t.tableBuf, false)
	}
	t.appendRendered(ansi)
}

func (t *TUI) flushTable() {
	if len(t.tableBuf) < 2 || !isTableSep(t.tableBuf[1]) {
		for _, l := range t.tableBuf {
			t.appendRendered(t.renderLine(l, nil, false))
		}
	} else {
		rendered := renderTable(t.tableBuf, t.width)
		for _, line := range strings.Split(rendered, "\n") {
			t.outAnsi = append(t.outAnsi, line)
		}
	}
	t.tableBuf = nil
}

// appendRendered adds a rendered line (may be empty for buffered table lines).
func (t *TUI) appendRendered(s string) {
	if s != "" {
		t.outAnsi = append(t.outAnsi, s)
	}
}

// renderAfterWrite is called after new output lines have been appended.
// It commits overflow to scrollback and renders the visible portion.
func (t *TUI) renderAfterWrite() {
	vis := t.outputRows()
	if vis <= 0 {
		return
	}

	// 1. Commit lines that have scrolled off the visible area to terminal scrollback.
	if len(t.outAnsi) > vis && t.pushed < len(t.outAnsi)-vis {
		newLines := t.outAnsi[t.pushed : len(t.outAnsi)-vis]

		// Use a temporary scroll region for the output area.
		fmt.Fprintf(os.Stdout, "\x1b[1;%dr", vis)
		fmt.Fprintf(os.Stdout, "\x1b[%d;1H", vis)
		for _, line := range newLines {
			fmt.Fprintf(os.Stdout, "%s\r\n", line)
		}
		// Restore full screen — no persistent scroll region.
		fmt.Fprint(os.Stdout, "\x1b[r")

		t.pushed = len(t.outAnsi) - vis
	}

	// 2. Render visible output rows.
	t.renderOutputScreen()
}

// ── ReadLine ─────────────────────────────────────────────────────

func (t *TUI) ReadLine() (string, error) {
	t.showCursor()
	for {
		select {
		case <-t.sigCh:
			t.mu.Lock()
			t.handleResize()
			t.fullDraw()
			t.mu.Unlock()
		default:
		}
		key, err := t.readKey()
		if err != nil {
			return "", fmt.Errorf("minitui: read key: %w", err)
		}
		t.mu.Lock()
		if key.ctrl && (key.r == 'c' || key.r == 'C') {
			t.mu.Unlock()
			if t.eventCh != nil {
				select { case t.eventCh <- Event{Type: EventInterrupt}: default: }
			}
			return "", fmt.Errorf("minitui: interrupted")
		}
		if key.enter && !key.shift {
			result := t.inputText()
			t.clearInput()
			t.renderInputBox()
			t.renderStatus()
			t.showCursor()
			t.mu.Unlock()
			if t.eventCh != nil {
				select { case t.eventCh <- Event{Type: EventSubmit, Input: result}: default: }
			}
			return result, nil
		}
		t.processKey(key)
		t.renderInputBox()
		t.renderStatus()
		t.showCursor()
		t.mu.Unlock()
	}
}

// SetStatus updates the status bar.
func (t *TUI) SetStatus(text string, style StatusStyle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.statusText = text
	t.statusStyle = style
	t.renderStatus()
}

// ── input helpers ────────────────────────────────────────────────

func (t *TUI) inputText() string {
	lines := make([]string, len(t.inLines))
	for i, r := range t.inLines {
		lines[i] = string(r)
	}
	return strings.Join(lines, "\n")
}

func (t *TUI) clearInput() {
	t.inLines = [][]rune{{}}
	t.inCursorRow = 0
	t.inCursorCol = 0
	t.inScrollRow = 0
	t.inHeight = 1
	// Re-render output area since it now has more visible rows.
	t.renderOutputScreen()
}

func (t *TUI) recalcInputHeight() {
	n := len(t.inLines)
	if n < 1 { n = 1 }
	if n > t.maxInputRows { n = t.maxInputRows }
	prev := t.inHeight
	if t.inHeight != n {
		if n > prev {
			// Push old output up to make room for the expanded input box.
			t.scrollOutputUp(n - prev)
		}
		t.inHeight = n
		// Re-render: output area shrank/grew, input box moved.
		t.renderOutputScreen()
		t.renderInputBox()
	}
	if t.inCursorRow < t.inScrollRow {
		t.inScrollRow = t.inCursorRow
	}
	if t.inCursorRow >= t.inScrollRow+t.inHeight {
		t.inScrollRow = t.inCursorRow - t.inHeight + 1
	}
	if t.inScrollRow < 0 {
		t.inScrollRow = 0
	}
}

func (t *TUI) scrollOutputUp(rows int) {
	vis := t.outputRows()
	fmt.Fprintf(os.Stdout, "\x1b[1;%dr", vis)
	fmt.Fprintf(os.Stdout, "\x1b[%d;1H", vis)
	for i := 0; i < rows; i++ {
		fmt.Fprint(os.Stdout, "\r\n")
	}
	fmt.Fprint(os.Stdout, "\x1b[r")
}

func (t *TUI) handleResize() {
	w, h, err := term.GetSize(t.stdoutFd)
	if err != nil || h < 5 {
		return
	}
	t.width = w
	t.height = h
	if t.eventCh != nil {
		select { case t.eventCh <- Event{Type: EventResize, Width: w, Height: h}: default: }
	}
}

// ── cursor ───────────────────────────────────────────────────────

func (t *TUI) showCursor() {
	if t.inCursorRow >= 0 && t.inCursorRow < len(t.inLines) {
		cr := t.inContentStart() + (t.inCursorRow - t.inScrollRow)
		cc := 1 + runeDisplayWidth(string(t.inLines[t.inCursorRow][:t.inCursorCol]))
		fmt.Fprintf(os.Stdout, "\x1b[%d;%dH\x1b[?25h", cr+1, cc)
	}
}
