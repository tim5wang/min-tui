# min-tui

[![Go Reference](https://pkg.go.dev/badge/github.com/tim5wang/min-tui.svg)](https://pkg.go.dev/github.com/tim5wang/min-tui)

A lightweight terminal UI library in pure Go, purpose-built for **coding agents**.  
Features streaming output, multi-line input with markdown rendering, slash commands, and a status bar — around 1500 lines.

<p align="center">
  <img src="examples/demo/image.png" alt="min-tui demo screenshot" width="720">
</p>

```bash
go get github.com/tim5wang/min-tui
```

## Features

- **Streaming output** — `Write()` renders content incrementally; overflow is committed to the terminal scrollback so history is scrollable
- **Multi-line input** — `Shift+Enter` / `Ctrl+J` inserts newlines; input box expands up to `MaxInputRows` (default 8), then scrolls internally
- **Slash commands** — `/name` triggers a filtered dropdown; arrow keys navigate, Enter selects, Esc cancels
- **Multi-turn interaction** — handlers use `Prompt()` for text input and `Select()` for secondary menus, written like synchronous code
- **Markdown rendering** — headings (`#`), **bold**, *italic*, `inline code`, fenced code blocks, and aligned tables
- **Status bar** — fixed at the bottom, 5 built-in styles (default / info / warning / error / success)
- **Configurable** — custom border color, custom markdown renderer, event channel
- **Concurrency-safe** — `Write()` can be called from any goroutine while `ReadLine()` is blocking

## Quick Start

```go
package main

import (
    "fmt"
    "os"

    "github.com/tim5wang/min-tui"
)

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

### Creating a TUI

```go
// Defaults.
tui, _ := minitui.New()

// With configuration.
tui, _ := minitui.NewWithConfig(minitui.Config{
    EventCh:      myEventCh,
    BorderColor:  "\x1b[36m",   // cyan borders
    RenderLine:   myRenderer,    // custom markdown→ANSI
    MaxInputRows: 12,
})
```

### Writing Output

```go
tui.WriteString("streaming output\n")
tui.Write([]byte("bytes too\n"))
```

Lines are buffered until `\n`, then rendered immediately. Safe for concurrent use.

### Reading Input

```go
input, err := tui.ReadLine()  // blocks until Enter
// err != nil → Ctrl+C
```

### Status Bar

```go
tui.SetStatus("processing...", minitui.StatusWarning)
tui.SetStatus("done!",         minitui.StatusSuccess)
```

### Events (optional)

```go
eventCh := make(chan minitui.Event, 8)
tui, _ := minitui.NewWithConfig(minitui.Config{EventCh: eventCh})

go func() {
    for ev := range eventCh {
        switch ev.Type {
        case minitui.EventSubmit:    // input submitted
        case minitui.EventResize:    // terminal resized
        case minitui.EventInterrupt: // Ctrl+C
        }
    }
}()
```

## Slash Commands

### Registering Commands

```go
tui.RegisterCommand(minitui.SlashCommand{
    Name:        "help",
    Description: "Show help",
    Handler: func(ctx *minitui.CommandContext) {
        ctx.Write("Available commands: /help, /echo, /login\n")
    },
})
```

### Multi-turn Interaction

The `CommandContext` provides blocking methods that work inside the handler goroutine:

```go
// Prompt — wait for the user to type and press Enter.
name := ctx.Prompt("What's your name?")

// Select — show a dropdown menu, return chosen index (or -1 on Esc).
method := ctx.Select("Pick a method", []minitui.SelectOption{
    {Label: "password", Description: "Login with password"},
    {Label: "token",    Description: "Login with token"},
})

// Write — output to the history area.
ctx.Write("Hello, " + name + "\n")

// SetStatus — update the status bar.
ctx.SetStatus("Ready", minitui.StatusSuccess)
```

### Complete Multi-step Example

```go
tui.RegisterCommand(minitui.SlashCommand{
    Name: "login",
    Handler: func(ctx *minitui.CommandContext) {
        // Step 1: secondary menu
        method := ctx.Select("选择登录方式", []minitui.SelectOption{
            {Label: "password", Description: "用户名 + 密码"},
            {Label: "token",    Description: "Token 认证"},
        })
        if method < 0 { return }

        // Step 2: text input
        if method == 0 {
            username := ctx.Prompt("用户名")
            password := ctx.Prompt("密码")
            ctx.Write("登录成功: " + username + "\n")
        } else {
            token := ctx.Prompt("Token")
            ctx.Write("Token 认证成功\n")
        }
    },
})
```

## Key Bindings

| Key | Action |
|-----|--------|
| `Enter` | Submit input / confirm selection |
| `Shift+Enter` | Insert newline |
| `Ctrl+J` | Insert newline (always works) |
| `Ctrl+C` | Interrupt |
| `Esc` | Cancel slash / close dropdown |
| `↑` `↓` | Navigate dropdown / select menu |
| `←` `→` | Move cursor |
| `Home` `End` | Jump to start/end of line |
| `Ctrl+A` `Ctrl+E` | Same as Home/End |
| `Ctrl+K` | Delete to end of line |
| `Ctrl+U` | Delete to start of line |
| `Ctrl+W` | Delete previous word |
| `Backspace` | Delete before cursor |
| `Delete` | Delete at cursor |
| `Tab` | Insert 4 spaces |

## Markdown Support

| Syntax | Rendering |
|--------|-----------|
| `# Heading` | **Bold** |
| `**text**` | **Bold** |
| `*text*` | *Italic* |
| `` `code` `` | Dim |
| ` ``` ... ``` ` | Dim block |
| `\| col \| col \|` | Aligned table |

## Configuration

```go
type Config struct {
    EventCh      chan<- Event           // async event channel
    RenderLine   func(string) string    // custom markdown→ANSI renderer
    BorderColor  string                 // e.g. "\x1b[34m" for blue
    MaxInputRows int                    // default 8
}
```

## Dependencies

Only [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) — terminal raw mode and size detection.

## Terminal Support

Tested on **iTerm2**, **Terminal.app**, **WezTerm**, **Kitty**, **VS Code integrated terminal**.

- **Shift+Enter** needs kitty/XTerm modifyOtherKeys protocol (modern terminals). Use **Ctrl+J** as universal fallback.
- Bracketed paste mode enabled for reliable multi-line paste.

## License

MIT
