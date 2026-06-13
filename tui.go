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

// StatusStyle controls the color and weight of the status bar text.
type StatusStyle int

// Status bar styles.
const (
	StatusDefault StatusStyle = iota // dim white — neutral idle state
	StatusInfo                        // cyan    — informational hints
	StatusWarning                    // yellow  — in-progress / pending action
	StatusError                      // red     — failed operations
	StatusSuccess                    // green   — successful completion
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
	EventSubmit    EventType = iota // user submitted input (also returned by ReadLine)
	EventResize                     // terminal was resized
	EventInterrupt                 // user pressed Ctrl+C
)

// SelectOption is a single item in a secondary selection menu.
type SelectOption struct {
	Label       string
	Description string
}

// CommandContext provides multi-turn interaction for slash commands.
type CommandContext struct {
	Args string
	tui  *TUI
}

// Prompt displays a prompt in the status bar and waits for user input.
func (ctx *CommandContext) Prompt(prompt string) string {
	if prompt != "" {
		ctx.tui.SetStatus(prompt, StatusInfo)
	}
	return ctx.tui.ReadModalInput()
}

// Select shows a dropdown menu. Returns the index of the chosen item,
// or -1 if the user pressed Escape to cancel.
func (ctx *CommandContext) Select(prompt string, options []SelectOption) int {
	if prompt != "" {
		ctx.tui.SetStatus(prompt, StatusInfo)
	}
	return ctx.tui.ReadSelect(options)
}

// Write outputs text to the output area.
func (ctx *CommandContext) Write(s string) { ctx.tui.WriteString(s) }

// SetStatus updates the status bar.
func (ctx *CommandContext) SetStatus(text string, style StatusStyle) {
	ctx.tui.SetStatus(text, style)
}

// SlashCommand is a slash-command registered by the application.
type SlashCommand struct {
	Name        string                      // e.g. "help", "search"
	Description string                      // shown in the dropdown
	Handler     func(ctx *CommandContext)   // called with interaction context
}

// Event is sent on the event channel when subscribed.
type Event struct {
	Type   EventType
	Input  string
	Width  int
	Height int
}

// Config holds optional settings for NewWithConfig.
type Config struct {
	// EventCh: if non-nil, EventSubmit/EventResize/EventInterrupt are sent here.
	EventCh chan<- Event

	// RenderLine: custom markdown-to-ANSI renderer. Receives one raw line,
	// returns the ANSI-styled version. When nil, the built-in renderer
	// handles headings, **bold**, *italic*, `code`, and tables.
	RenderLine func(string) string

	// BorderColor: ANSI escape for input-box border lines, e.g. "\x1b[34m".
	// Default: dim (\x1b[2m).
	BorderColor string

	// MaxInputRows: maximum visible rows in the input box. Default: 8.
	MaxInputRows int

	// ShowHeadingMarks: when true, # marks are visible (e.g. "## Title").
	// Default: false (only bold text).
	ShowHeadingMarks bool

	// Spacious: when true, blank lines are inserted before/after headings,
	// code blocks, and tables for readability. Default: false (compact).
	Spacious bool
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
	pendingGap  bool // insert blank line before next block (Spacious mode)
	tableBuf    []string
	codeLang    string // fence info string for current code block

	// Input editor.
	inLines     [][]rune // physical lines (one per Enter/Shift+Enter)
	inCursorRow int
	inCursorCol int // rune index
	inHeight    int
	inScrollRow int // visual scroll offset

	// Visual wrapping cache.
	visLines    []vLine // flat visual lines (wrapped from inLines)

	// Status bar.
	statusText  string
	statusStyle StatusStyle

	// Configuration.
	eventCh      chan<- Event
	customRender func(string) string
	borderColor  string
	maxInputRows int
	showMarks    bool // heading marks visible
	spacious     bool // blank lines between blocks

	// Slash commands.
	slashCmds      []SlashCommand
	slashMode      bool
	slashQuery     string
	slashMatches   []int
	slashSelected  int
	slashDropdownH int
	slashScrollOff int // first visible match index

	// Modal input (multi-turn slash commands).
	modal chan string

	// Select mode (secondary dropdown menu from CommandContext.Select).
	selectMode     bool
	selectItems    []SelectOption
	selectIdx      int
	selectScrollOff int
	selectCh       chan int

	// Popup windows.
	popups      []*popupState
	globalKeyFn func(KeyEvent) bool

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
		showMarks:    cfg.ShowHeadingMarks,
		spacious:     cfg.Spacious,
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

func (t *TUI) outputRows() int { return t.outputRowsForSlash() }

// outputRowsForSlash returns visible output rows accounting for dropdown.
func (t *TUI) outputRowsForSlash() int {
	dh := 0
	if t.slashMode || t.selectMode {
		dh = t.slashDropdownH
	}
	return t.height - t.inHeight - dh - 3
}

// Layout (0-based):
//   [0 … outputRows-1]              output area
//   [outputRows … outputRows+dh-1]   slash dropdown (if active)
//   [outputRows+dh]                  input top border
//   [outputRows+dh+1 … +dh+inH]      input content
//   [outputRows+dh+inH+1]            input bottom border
//   [height-1]                        status bar
func (t *TUI) inTopBorder() int    { return t.outputRows() + t.slashDropdownH }
func (t *TUI) inContentStart() int { return t.inTopBorder() + 1 }
func (t *TUI) inBotBorder() int    { return t.inContentStart() + t.inHeight }
func (t *TUI) statusRow() int      { return t.height - 1 }

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

// WriteString is a convenience wrapper around Write.
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
		if t.inCodeBlock {
			t.codeLang = extractLang(raw)
			t.maybeInsertGap()
		} else {
			t.codeLang = ""
			t.pendingGap = t.spacious
		}
		t.appendRendered(t.renderLine(raw, nil, true))
		return
	}

	if t.inCodeBlock {
		if t.codeLang != "" {
			t.appendRendered(highlightCodeBlock(raw, t.codeLang))
		} else {
			t.appendRendered(t.renderLine(raw, nil, true))
		}
		return
	}

	// Headings trigger spacing in spacious mode.
	if t.spacious && isHeadingLine(raw) {
		t.maybeInsertGap()
		t.appendRendered(t.renderLine(raw, &t.tableBuf, false))
		t.pendingGap = true
		return
	}

	// Table start — gap before first row.
	if t.spacious && isTableLine(raw) && len(t.tableBuf) == 0 {
		t.maybeInsertGap()
	}

	t.appendRendered(t.renderLine(raw, &t.tableBuf, false))
}

func (t *TUI) maybeInsertGap() {
	if !t.pendingGap || len(t.outAnsi) == 0 {
		t.pendingGap = false
		return
	}
	if t.outAnsi[len(t.outAnsi)-1] == "" {
		t.pendingGap = false
		return
	}
	t.outAnsi = append(t.outAnsi, "")
	t.pendingGap = false
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

// commitOverflow pushes non-blank content lines beyond vis into scrollback.
// Blank spacing lines are skipped so gaps don't push real content off-screen.
func (t *TUI) commitOverflow() {
	vis := t.outputRows()
	// Safety clamp: scroll region must not touch input/status area.
	if vis > t.height-3 {
		vis = t.height - 3
	}
	if vis <= 0 || len(t.outAnsi) <= vis {
		return
	}
	// Count content (non-blank) lines to commit.
	need := len(t.outAnsi) - vis
	var lines []string
	i := t.pushed
	for i < len(t.outAnsi) && len(lines) < need {
		if t.outAnsi[i] != "" {
			lines = append(lines, t.outAnsi[i])
		}
		i++
	}
	if len(lines) == 0 {
		t.pushed = i
		return
	}
	t.pushed = i

	fmt.Fprintf(os.Stdout, "\x1b[1;%dr", vis)
	fmt.Fprintf(os.Stdout, "\x1b[%d;1H", vis)
	for _, line := range lines {
		fmt.Fprintf(os.Stdout, "%s\r\n", line)
	}
	fmt.Fprint(os.Stdout, "\x1b[r")
	os.Stdout.Sync()
}

// renderAfterWrite is called after new output lines have been appended.
// It commits overflow to scrollback and renders the visible portion.
func (t *TUI) renderAfterWrite() {
	vis := t.outputRows()
	if vis <= 0 {
		return
	}

	// 1. Commit overflowing content lines to scrollback (skip blank spacing).
	t.commitOverflow()

	// 2. Render visible output rows.
	t.renderOutputScreen()

	// 3. Re-render overlay (input box + status bar) to fix any corruption.
	t.renderInputBox()
	t.renderStatus()

	// Re-render popups if active (scroll may have shifted positions).
	if len(t.popups) > 0 {
		t.reRenderPopups()
	}
}

// ── modal input (multi-turn slash commands) ─────────────────────

// ReadModalInput blocks until the user submits input. It is called from
// slash command handlers (which run in a goroutine) to implement multi-step
// interactions like /login → username → password.
func (t *TUI) ReadModalInput() string {
	t.mu.Lock()
	ch := make(chan string, 1)
	t.modal = ch
	t.mu.Unlock()
	return <-ch
}

// ReadSelect shows a dropdown menu with the given options and returns the
// selected index. Returns -1 if cancelled with Escape.
func (t *TUI) ReadSelect(options []SelectOption) int {
	t.mu.Lock()
	t.selectMode = true
	t.selectItems = options
	t.selectIdx = 0
	t.selectScrollOff = 0
	dh := len(options)
	if dh > slashDropdownMax {
		dh = slashDropdownMax
	}
	t.slashDropdownH = dh // reuse slash dropdown height field for layout
	t.selectCh = make(chan int, 1)

	t.renderOutputScreen()
	t.renderSelectDropdown()
	t.renderInputBox()
	t.renderStatus()
	t.mu.Unlock()

	result := <-t.selectCh

	t.mu.Lock()
	t.selectMode = false
	t.slashDropdownH = 0
	t.selectItems = nil
	t.renderOutputScreen()
	t.mu.Unlock()
	return result
}

// ── ReadLine ─────────────────────────────────────────────────────

// ReadLine blocks until the user presses Enter (without Shift).
// It returns the submitted text, or an error if Ctrl+C is pressed.
//
// While ReadLine is blocking, other goroutines may call Write or SetStatus
// to update the output area and status bar concurrently.
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

		// Global key handler — before lock so PushPopup does not deadlock.
		if t.globalKeyFn != nil && t.globalKeyFn(keyEventFromInternal(key)) {
			continue
		}

		t.mu.Lock()

		// Popup active: handle Esc / OnKey, otherwise normal input editing.
		if len(t.popups) > 0 {
			if t.handlePopupKey(key) {
				t.mu.Unlock()
				continue
			}
			// Key not consumed by popup — fall through to normal processing.
		}

		if key.ctrl && (key.r == 'c' || key.r == 'C') {
			t.mu.Unlock()
			if t.eventCh != nil {
				select { case t.eventCh <- Event{Type: EventInterrupt}: default: }
			}
			return "", fmt.Errorf("minitui: interrupted")
		}
		if key.enter && !key.shift {
			// Select mode: send selected index.
			if t.selectMode && t.selectCh != nil {
				ch := t.selectCh
				t.selectCh = nil
				t.mu.Unlock()
				ch <- t.selectIdx
				continue
			}
			// Slash mode: execute selected command instead of submitting.
			if t.slashMode {
				t.executeSelectedCommand()
				t.clearInput()
				t.exitSlashMode()
				t.renderInputBox()
				t.renderStatus()
				t.showCursor()
				t.mu.Unlock()
				continue
			}
			// Modal input: route to waiting handler.
			if t.modal != nil {
				result := t.inputText()
				t.clearInput()
				t.renderInputBox()
				t.renderStatus()
				t.showCursor()
				ch := t.modal
				t.modal = nil
				t.mu.Unlock()
				ch <- result
				continue
			}
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
	t.renderOutputScreen()
}

func (t *TUI) recalcInputHeight() {
	t.buildVisualLines()
	n := len(t.visLines)
	if n < 1 { n = 1 }
	if n > t.maxInputRows { n = t.maxInputRows }
	prev := t.inHeight
	if t.inHeight != n {
		if n > prev {
			t.scrollOutputUp(n - prev)
		}
		t.inHeight = n
		t.renderOutputScreen()
		t.renderInputBox()
	}
	vr, _ := t.visCursor()
	if vr < t.inScrollRow {
		t.inScrollRow = vr
	}
	if vr >= t.inScrollRow+t.inHeight {
		t.inScrollRow = vr - t.inHeight + 1
	}
	if t.inScrollRow < 0 {
		t.inScrollRow = 0
	}
}

func (t *TUI) scrollOutputUp(rows int) {
	oldVis := t.outputRows() // output rows before height change
	if oldVis > t.height-3 {
		oldVis = t.height - 3
	}

	// Temporarily set scroll region to old output area.
	fmt.Fprintf(os.Stdout, "\x1b[1;%dr", oldVis)
	fmt.Fprintf(os.Stdout, "\x1b[%d;1H", oldVis)

	// Push the last `rows` lines of the current visible output into scrollback.
	start := len(t.outAnsi) - oldVis
	if start < 0 {
		start = 0
	}
	for i := 0; i < rows; i++ {
		line := ""
		if start+i < len(t.outAnsi) {
			line = t.outAnsi[start+i]
		}
		fmt.Fprintf(os.Stdout, "%s\r\n", line)
	}

	fmt.Fprint(os.Stdout, "\x1b[r") // restore full-screen
	t.pushed += rows
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
	vr, vc := t.visCursor()
	cr := t.inContentStart() + (vr - t.inScrollRow)
	cc := vc + 1
	fmt.Fprintf(os.Stdout, "\x1b[%d;%dH\x1b[?25h", cr+1, cc)
}

// ── slash commands ──────────────────────────────────────────────

// RegisterCommand adds a slash command. When the user types /name in the
// input box, a dropdown appears. Selecting and pressing Enter calls Handler.
func (t *TUI) RegisterCommand(cmd SlashCommand) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.slashCmds = append(t.slashCmds, cmd)
}

// UnregisterCommand removes a slash command by name.
func (t *TUI) UnregisterCommand(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, c := range t.slashCmds {
		if c.Name == name {
			t.slashCmds = append(t.slashCmds[:i], t.slashCmds[i+1:]...)
			return
		}
	}
}

// ── visual word wrapping ────────────────────────────────────────

type vLine struct {
	physRow int    // index into inLines
	start   int    // rune offset in inLines[physRow]
	text    string // display text for this visual line
}

// buildVisualLines wraps inLines into visual lines of width t.width.
func (t *TUI) buildVisualLines() {
	t.visLines = nil
	maxW := t.width - 1 // 1 reserved for cursor at EOL
	if maxW < 1 { maxW = 1 }
	for ri, line := range t.inLines {
		pos := 0
		for pos < len(line) {
			end := pos
			w := 0
			for end < len(line) {
				rl := line[end]
				rw := 4; if rl != '\t' { rw = runeWidth(rl) }
				if w+rw > maxW { break }
				w += rw
				end++
			}
			if end == pos {
				end = pos + 1 // force at least 1 char per visual line
			}
			t.visLines = append(t.visLines, vLine{
				physRow: ri,
				start:   pos,
				text:    strings.ReplaceAll(string(line[pos:end]), "\t", "    "),
			})
			pos = end
		}
	}
	if t.visLines == nil {
		t.visLines = append(t.visLines, vLine{text: ""})
	}
}

// visCursor returns the visual row/col of the current cursor.
func (t *TUI) visCursor() (visRow, visCol int) {
	if len(t.visLines) == 0 {
		return 0, 0
	}
	// Find which visual line contains inCursorRow and inCursorCol.
	for i, vl := range t.visLines {
		if vl.physRow != t.inCursorRow {
			continue
		}
		line := t.inLines[t.inCursorRow]
		// inCursorCol is a rune index; compute the visual column within this wrapped line.
		preDisplay := displayWidth(string(line[vl.start:t.inCursorCol]))
		if preDisplay <= t.width-1 || i == len(t.visLines)-1 || t.visLines[i+1].physRow != t.inCursorRow {
			return i, preDisplay
		}
	}
	// Cursor on the last character that starts a wrapped continuation line.
	// Put it at column 0 of the next line.
	last := 0
	for i := len(t.visLines) - 1; i >= 0; i-- {
		if t.visLines[i].physRow == t.inCursorRow {
			last = i + 1
			break
		}
	}
	if last < len(t.visLines) {
		return last, 0
	}
	return len(t.visLines) - 1, displayWidth(t.visLines[len(t.visLines)-1].text)
}

