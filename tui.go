// Package minitui provides a lightweight TUI for coding agents with streaming
// output, multi-line input, markdown rendering, and a status bar.
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

// Event is sent when the user submits input, resizes the terminal, or interrupts.
type Event struct {
	Type   EventType
	Input  string // valid for EventSubmit
	Width  int    // valid for EventResize
	Height int
}

// Config holds optional settings for the TUI.
type Config struct {
	// EventCh: if non-nil, events are sent here instead of ReadLine blocking.
	// The channel must be buffered or the caller must drain it promptly.
	EventCh chan<- Event

	// RenderLine: custom markdown-to-ANSI renderer.  Receives one raw line
	// and returns the ANSI-styled version.  When nil, the built-in renderer
	// is used (supports headings, **bold**, *italic*, `code`, tables).
	RenderLine func(raw string) string

	// BorderColor is an ANSI escape for the input-box border lines, e.g.
	// "\x1b[34m" for blue.  Must include a reset if needed.
	BorderColor string

	// MaxInputRows overrides the default maximum input box height.
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

	// scroll region: output scrolls within rows [0 … scrollBottom].
	// Rows below are the fixed input box + status bar.
	scrollBottom int

	// output state
	pendingRaw  []byte
	inCodeBlock bool
	tableBuf    []string

	// input editor
	inLines     [][]rune
	inCursorRow int
	inCursorCol int
	inHeight    int
	inScrollRow int

	// status bar
	statusText  string
	statusStyle StatusStyle

	// configuration
	eventCh      chan<- Event
	customRender func(string) string
	borderColor  string
	maxInputRows int

	// keyboard
	keyBuf []byte
	sigCh  chan os.Signal
}

// New creates a TUI with default settings.
func New() (*TUI, error) {
	return NewWithConfig(Config{})
}

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
		bc = "\x1b[2m" // dim by default
	}

	t := &TUI{
		stdinFd:   stdinFd, stdoutFd: stdoutFd,
		oldState:  oldState, width: w, height: h,
		inLines:   [][]rune{{}}, inHeight: 1,
		keyBuf:    make([]byte, 4096),
		sigCh:     make(chan os.Signal, 1),
		eventCh:   cfg.EventCh,
		customRender: cfg.RenderLine,
		borderColor:  bc,
		maxInputRows: maxRows,
	}

	t.recalcScrollBottom()

	fmt.Fprint(os.Stdout, "\x1b[?2004h\x1b[>1u\x1b[>4;2m") // paste + kitty + xterm
	fmt.Fprint(os.Stdout, "\x1b[?25l")                        // hide cursor
	t.setScrollRegion()
	signal.Notify(t.sigCh, syscall.SIGWINCH)

	// Draw initial UI.
	t.renderInputBox()
	t.renderStatus()
	t.showCursor()

	return t, nil
}

// Close restores the terminal. Call once when done.
func (t *TUI) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetScrollRegion()
	fmt.Fprint(os.Stdout, "\x1b[?2004l\x1b[<u\x1b[>4;0m\x1b[?25h")
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
	term.Restore(t.stdinFd, t.oldState)
	signal.Stop(t.sigCh)
}

// ── scroll region ────────────────────────────────────────────────

func (t *TUI) recalcScrollBottom() {
	t.scrollBottom = t.height - t.inHeight - 4
}

func (t *TUI) setScrollRegion() {
	fmt.Fprintf(os.Stdout, "\x1b[1;%dr", t.scrollBottom+1)
}

func (t *TUI) resetScrollRegion() {
	fmt.Fprint(os.Stdout, "\x1b[r")
}

// ── layout ───────────────────────────────────────────────────────

func (t *TUI) inTopBorder() int   { return t.scrollBottom + 1 }
func (t *TUI) inContentStart() int { return t.scrollBottom + 2 }
func (t *TUI) inBotBorder() int    { return t.scrollBottom + 2 + t.inHeight }
func (t *TUI) statusRow() int      { return t.height - 1 }

// ── cursor ───────────────────────────────────────────────────────

// showCursor moves the cursor to the input box and makes it visible.
func (t *TUI) showCursor() {
	if t.inCursorRow >= 0 && t.inCursorRow < len(t.inLines) {
		cr := t.inContentStart() + (t.inCursorRow - t.inScrollRow)
		cc := 1 + runeDisplayWidth(string(t.inLines[t.inCursorRow][:t.inCursorCol]))
		fmt.Fprintf(os.Stdout, "\x1b[%d;%dH\x1b[?25h", cr+1, cc)
	}
}

// ── output (streaming, uses scroll region) ───────────────────────

// Write appends data to the output area. It uses the terminal's scroll region
// so history naturally scrolls into the terminal scrollback buffer.
// Safe for concurrent use.
func (t *TUI) Write(data []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.appendOutput(data)
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
		t.writeToScroll(t.renderLine(raw, nil, true))
		return
	}
	var ansi string
	if t.inCodeBlock {
		ansi = t.renderLine(raw, nil, true)
	} else {
		ansi = t.renderLine(raw, &t.tableBuf, false)
	}
	t.writeToScroll(ansi)
}

func (t *TUI) flushTable() {
	if len(t.tableBuf) < 2 || !isTableSep(t.tableBuf[1]) {
		for _, l := range t.tableBuf {
			t.writeToScroll(t.renderLine(l, nil, false))
		}
	} else {
		rendered := renderTable(t.tableBuf, t.width)
		t.writeToScroll(rendered)
	}
	t.tableBuf = nil
}

// writeToScroll writes content to the bottom of the scroll region,
// causing the terminal to scroll the output area naturally.
// writeToScroll writes content to the bottom of the scroll region.
// Each line must use \r\n so the cursor returns to column 1 after scrolling.
func (t *TUI) writeToScroll(s string) {
	if s == "" {
		return
	}
	lines := strings.Split(s, "\n")
	fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;1H", t.scrollBottom+1)
	for _, line := range lines {
		fmt.Fprintf(os.Stdout, "%s\r\n", line)
	}
	fmt.Fprint(os.Stdout, "\x1b[u")
}

// ── ReadLine (blocking) ──────────────────────────────────────────

// ReadLine blocks until the user submits input (Enter without Shift).
// Ctrl+C returns an error.  Other goroutines may call Write/SetStatus.
func (t *TUI) ReadLine() (string, error) {
	t.showCursor()
	for {
		select {
		case <-t.sigCh:
			t.mu.Lock()
			t.handleResize()
			t.fullRedraw()
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
	t.recalcScrollBottom()
	t.setScrollRegion()
}

func (t *TUI) recalcInputHeight() {
	n := len(t.inLines)
	if n < 1 {
		n = 1
	}
	if n > t.maxInputRows {
		n = t.maxInputRows
	}
	prev := t.inHeight
	if t.inHeight != n {
		if n > prev {
			// Push old output up so expanded input box doesn't cover it.
			t.scrollOutputUp(n - prev)
		}
		t.inHeight = n
		t.recalcScrollBottom()
		t.setScrollRegion()
		t.renderInputBox()
	}
	// Adjust scroll so cursor is visible.
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

// scrollOutputUp pushes existing output up by writing empty lines at the
// bottom of the current scroll region, so the expanded input box has room.
func (t *TUI) scrollOutputUp(rows int) {
	fmt.Fprintf(os.Stdout, "\x1b[s\x1b[%d;1H", t.scrollBottom+1)
	for i := 0; i < rows; i++ {
		fmt.Fprint(os.Stdout, "\r\n")
	}
	fmt.Fprint(os.Stdout, "\x1b[u")
}

func (t *TUI) handleResize() {
	w, h, err := term.GetSize(t.stdoutFd)
	if err != nil || h < 5 {
		return
	}
	t.width = w
	t.height = h
	t.recalcScrollBottom()
	t.setScrollRegion()
	if t.eventCh != nil {
		select { case t.eventCh <- Event{Type: EventResize, Width: w, Height: h}: default: }
	}
}

func (t *TUI) fullRedraw() {
	t.resetScrollRegion()
	t.setScrollRegion()
	t.renderInputBox()
	t.renderStatus()
	t.showCursor()
}
