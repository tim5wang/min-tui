package minitui

import (
	"strings"
)

// ── popup types ──────────────────────────────────────────────────

// KeyEvent is the public key type exposed to popup handlers.
type KeyEvent struct {
	Rune   rune
	Enter  bool
	Shift  bool
	Ctrl   bool
	Alt    bool
	Special int // keyUp, keyDown, etc. (same constants as internal key.go)
}

func keyEventFromInternal(k keyEvent) KeyEvent {
	return KeyEvent{
		Rune:    k.r,
		Enter:   k.enter,
		Shift:   k.shift,
		Ctrl:    k.ctrl,
		Alt:     k.alt,
		Special: k.special,
	}
}

// Popup configures an overlay window.
type Popup struct {
	Title       string
	Width       int  // 0 = 80% of terminal width
	Height      int  // 0 = auto-fit content
	BorderColor string // ANSI escape for border/title, default "\x1b[36m" (cyan)
	BgColor     string // ANSI escape for content background, default "\x1b[47;30m" (white bg)
	Render      func(w, h int) []string          // returns content lines
	OnKey       func(key KeyEvent) (close bool)  // true closes the popup
	OnClose     func()                            // called after popup is removed
}

// ── popup stack ──────────────────────────────────────────────────

// PushPopup opens a popup window on top of the current view.  The popup
// is rendered over the output area; when closed, the original output is
// restored.  Safe to call from any goroutine.
func (t *TUI) PushPopup(p Popup) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if p.Width <= 0 {
		p.Width = t.width * 8 / 10
	}
	if p.Width > t.width-2 {
		p.Width = t.width - 2
	}
	if p.Height <= 0 {
		p.Height = t.outputRows() * 6 / 10
	}
	if p.Height < 3 {
		p.Height = 3
	}
	if p.Height > t.outputRows()-1 {
		p.Height = t.outputRows() - 1
	}
	if p.BorderColor == "" {
		p.BorderColor = "\x1b[36m" // cyan
	}
	if p.BgColor == "" {
		p.BgColor = "\x1b[47;30m" // white bg, black fg
	}

	// Center popup in the output area.
	x := (t.width - p.Width) / 2
	y := (t.outputRows() - p.Height) / 2

	pp := &popupState{
		Popup: p,
		x:     x, y: y,
	}
	t.popups = append(t.popups, pp)
	t.renderPopup(pp)
}

// PopPopup closes the topmost popup.
func (t *TUI) PopPopup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.popPopup()
	t.renderAfterPopupClose()
}

// SetGlobalKeyHandler registers a function called for every keypress in
// normal mode.  Return true to consume the key (it will not be processed
// by the input editor).  Use this to detect trigger keys for popups.
func (t *TUI) SetGlobalKeyHandler(fn func(KeyEvent) bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.globalKeyFn = fn
}

// ── popup state ──────────────────────────────────────────────────

type popupState struct {
	Popup
	x, y int
}

// ── internal: popup rendering ────────────────────────────────────

func (t *TUI) renderPopup(p *popupState) {
	w, h := p.Width, p.Height
	bc, bg, rst := p.BorderColor, p.BgColor, ansiReset

	// Top border with title.
	title := p.Title
	tw := displayWidth(title)
	if tw > w-5 {
		// Truncate title to fit.
		var b strings.Builder
		cur := 0
		for _, r := range title {
			rw := runeWidth(r)
			if cur+rw > w-5 { break }
			b.WriteRune(r); cur += rw
		}
		title = b.String()
		tw = cur
	}
	padW := w - tw - 5 // dashes between title and right border
	if padW < 0 {
		padW = 0
	}

	topBar := bc + "┌─ " + rst + title + bc + " " + strings.Repeat("─", padW) + "┐" + rst
	t.writeRow(p.y, topBar)

	// Content area.
	content := p.Render(w-2, h-2)
	for i := 0; i < h-2; i++ {
		var line string
		if i < len(content) {
			line = bc + "│" + rst + bg + padTo(content[i], w-2) + rst + bc + "│" + rst
		} else {
			line = bc + "│" + rst + bg + strings.Repeat(" ", w-2) + rst + bc + "│" + rst
		}
		t.writeRow(p.y+1+i, line)
	}

	// Bottom border.
	botBar := bc + "└" + strings.Repeat("─", w) + "┘" + rst
	t.writeRow(p.y+h-1, botBar)
}

// ── internal: popup key dispatch ─────────────────────────────────

// handlePopupKey returns true if the key was consumed (Esc or OnKey).
func (t *TUI) handlePopupKey(k keyEvent) bool {
	if k.r == 27 { // Escape
		t.popPopup()
		t.renderAfterPopupClose()
		return true
	}
	top := t.popups[len(t.popups)-1]
	if top.OnKey != nil && top.OnKey(keyEventFromInternal(k)) {
		t.popPopup()
		t.renderAfterPopupClose()
		return true
	}
	return false
}

// processPopupKey is called from processKey when a popup is active
// and the key wasn't consumed by the ReadLine dispatch.
func (t *TUI) processPopupKey(k keyEvent) {
	if k.r == 27 {
		t.popPopup()
		t.renderAfterPopupClose()
		return
	}
	top := t.popups[len(t.popups)-1]
	if top.OnKey != nil && top.OnKey(keyEventFromInternal(k)) {
		t.popPopup()
		t.renderAfterPopupClose()
		return
	}
}

func (t *TUI) popPopup() {
	if len(t.popups) == 0 {
		return
	}
	top := t.popups[len(t.popups)-1]
	t.popups = t.popups[:len(t.popups)-1]
	if top.OnClose != nil {
		top.OnClose()
	}
}

func (t *TUI) renderAfterPopupClose() {
	t.renderOutputScreen()
	t.renderInputBox()
	t.renderStatus()
	t.showCursor()
}

// reRenderPopups redraws all active popups (called after Write).
func (t *TUI) reRenderPopups() {
	for _, p := range t.popups {
		t.renderPopup(p)
	}
}

// padTo pads or truncates a string to exactly w display cells.
func padTo(s string, w int) string {
	dw := displayWidth(s)
	if dw >= w {
		var b strings.Builder
		cur := 0
		for _, r := range s {
			rw := runeWidth(r)
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
	return s + strings.Repeat(" ", w-dw)
}
