# min-tui

[![Go Reference](https://pkg.go.dev/badge/github.com/tim5wang/min-tui.svg)](https://pkg.go.dev/github.com/tim5wang/min-tui)

A lightweight terminal UI library in pure Go, purpose-built for **coding agents**.  
Streaming output, multi-line input, markdown, slash commands, popups — ~1800 lines.

<p align="center">
  <img src="examples/demo/image.png" alt="min-tui demo screenshot" width="720">
</p>

```bash
go get github.com/tim5wang/min-tui
```

## Features

- **Streaming output** — `Write()` renders incrementally; overflow enters terminal scrollback
- **Multi-line input** — `Shift+Enter` / `Ctrl+J` newlines; input box expands to `MaxInputRows` (default 8)
- **Slash commands** — `/name` filtered dropdown; arrow keys navigate, Enter selects, Esc cancels
- **Multi-turn interaction** — `Prompt()` for text, `Select()` for menus, written like sync code
- **Popup windows** — overlay dialogs with focus switching (Tab), interactive `OnKey`, custom colors
- **Markdown** — headings, **bold**, *italic*, `inline code`, fenced blocks, aligned tables
- **Status bar** — 5 styles (default / info / warning / error / success)
- **Configurable** — border color, custom markdown renderer, event channel
- **Concurrency-safe** — `Write()` from any goroutine while `ReadLine()` blocks

## Quick Start

```go
package main

import "github.com/tim5wang/min-tui"

func main() {
    tui, _ := minitui.New()
    defer tui.Close()

    tui.SetStatus("Enter to submit | / for commands", minitui.StatusInfo)

    for {
        input, err := tui.ReadLine()
        if err != nil { return }
        tui.WriteString("You said: " + input + "\n")
    }
}
```

## API

```go
tui, _ := minitui.New()
tui, _ := minitui.NewWithConfig(minitui.Config{
    EventCh:      myEventCh,         // optional
    BorderColor:  "\x1b[36m",       // input box borders
    RenderLine:   myRenderer,        // custom markdown→ANSI
    MaxInputRows: 12,
})

tui.WriteString("output\n")          // streaming output
input, err := tui.ReadLine()         // blocking (Ctrl+C → err)
tui.SetStatus("...", minitui.StatusWarning)
defer tui.Close()                    // restore terminal
```

## Slash Commands

```go
tui.RegisterCommand(minitui.SlashCommand{
    Name: "login",
    Handler: func(ctx *minitui.CommandContext) {
        // secondary menu
        method := ctx.Select("选择方式", []minitui.SelectOption{
            {Label: "password", Description: "用户名+密码"},
            {Label: "token",    Description: "Token 认证"},
        })
        if method < 0 { return }           // Esc cancelled

        // text input
        username := ctx.Prompt("用户名")
        password := ctx.Prompt("密码")

        ctx.Write("登录成功: " + username + "\n")
        ctx.SetStatus("就绪", minitui.StatusSuccess)
    },
})
```

### CommandContext Methods

| Method | Description |
|--------|-------------|
| `Prompt(prompt) string` | Block until user presses Enter, return text |
| `Select(prompt, opts) int` | Show dropdown, return index (-1 = Esc) |
| `Write(s)` | Output to history area |
| `SetStatus(text, style)` | Update status bar |

## Popup Windows

```go
// Open a popup by registering a global key handler.
tui.SetGlobalKeyHandler(func(k minitui.KeyEvent) bool {
    if k.Ctrl && k.Rune == 'p' {
        tui.PushPopup(minitui.Popup{
            Title:  "Help",
            Width:  40, Height: 12,
            Render: func(w, h int) []string {
                return []string{"", "  Esc to close", "", "  Tab to toggle focus"}
            },
            OnKey: func(k minitui.KeyEvent) minitui.PopupAction {
                if k.Special == minitui.KeyDown { return minitui.PopupUpdate }
                return minitui.PopupPassthrough
            },
        })
        return true
    }
    return false
})
```

### Popup Fields

| Field | Description |
|-------|-------------|
| `Title` | Window title |
| `Width`, `Height` | Size (0 = auto) |
| `BorderColor` | ANSI color when focused, default cyan |
| `BgColor` | Background when focused, default white |
| `BorderColorUnfocus` | Border when unfocused, default dim cyan |
| `BgColorUnfocus` | Background when unfocused, default gray |
| `Render(w,h)` | Return content lines |
| `OnKey` | Handle keys → `PopupPassthrough` / `PopupUpdate` / `PopupClose` |
| `OnClose` | Called after popup removed |

### Focus & Interaction

| Key | Action |
|-----|--------|
| `Tab` | Toggle focus between input ↔ popup |
| `Esc` | Close popup |
| Focused popup | Bright border, keys go to `OnKey` |
| Unfocused popup | Dim border, keys pass through to input editor |

## Key Bindings

| Key | Action |
|-----|--------|
| `Enter` | Submit / confirm |
| `Shift+Enter` / `Ctrl+J` | Insert newline |
| `Ctrl+C` | Interrupt |
| `Esc` | Cancel slash / close popup |
| `Tab` | Insert spaces (normal) / toggle popup focus |
| `↑` `↓` | Navigate dropdowns |
| `←` `→` | Move cursor |
| `Home` `End` | Jump start/end of line |
| `Ctrl+A` `E` `K` `U` `W` | Emacs shortcuts |
| `Backspace` / `Delete` | Delete char |

## Markdown

| Syntax | Rendering |
|--------|-----------|
| `# Heading` | **Bold** |
| `**text**` | **Bold** |
| `*text*` | *Italic* |
| `` `code` `` | Dim |
| ` ``` ``` ` | Dim block |
| `\| col \| col \|` | Aligned table |

## Dependencies

Only [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term).

## Terminal Support

**iTerm2**, **Terminal.app**, **WezTerm**, **Kitty**, **VS Code** terminal.

- `Shift+Enter` needs kitty/XTerm modifyOtherKeys. Use `Ctrl+J` as universal fallback.

## License

MIT
