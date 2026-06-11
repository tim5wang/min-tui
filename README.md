# min-tui

[![Go Reference](https://pkg.go.dev/badge/github.com/tim5wang/min-tui.svg)](https://pkg.go.dev/github.com/tim5wang/min-tui)

A lightweight terminal UI library in pure Go, purpose-built for **coding agents**.  
Features streaming output, multi-line input with markdown rendering, and a fixed status bar — all under 1200 lines.

```bash
go get github.com/tim5wang/min-tui
```

## Features

- **Streaming output** — `Write()` streams content incrementally to the terminal, using the native scroll region so history stays in the scrollback buffer
- **Multi-line input** — `Shift+Enter` / `Ctrl+J` inserts newlines; input box expands up to `MaxInputRows` (default 8), then scrolls internally
- **Markdown rendering** — headings (`#`), **bold**, *italic*, `inline code`, fenced code blocks, and aligned tables
- **Status bar** — fixed at the bottom, 5 built-in styles (default / info / warning / error / success)
- **Scroll region** — output uses the terminal's own scrolling; scroll up to review history
- **Configurable** — custom border color, custom markdown renderer, event channel
- **Concurrency-safe** — `Write()` can be called from any goroutine while `ReadLine()` is blocking

## Quick Start

```go
package main

import (
    "fmt"
    "os"
    "time"

    "github.com/tim5wang/min-tui"
)

func main() {
    tui, err := minitui.New()
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    defer tui.Close()

    tui.SetStatus("Enter to submit | Shift+Enter/Ctrl+J for newline", minitui.StatusInfo)

    for {
        input, err := tui.ReadLine()
        if err != nil {
            return // Ctrl+C
        }

        // Stream response token by token.
        tui.WriteString("You said: ")
        tui.WriteString(input)
        tui.WriteString("\n")

        // Markdown demo.
        tui.WriteString("\n# Heading\n**bold** and *italic*\n")
        tui.WriteString("| Col A | Col B |\n|-------|-------|\n| 1     | 2     |\n\n")
    }
}
```

## API

### Creating a TUI

```go
// Default configuration.
tui, err := minitui.New()

// With configuration.
tui, err := minitui.NewWithConfig(minitui.Config{
    EventCh:      myEventCh,           // optional event channel
    BorderColor:  "\x1b[36m",         // cyan input borders
    RenderLine:   myRenderFunc,        // custom markdown → ANSI
    MaxInputRows: 12,                  // override default 8
})
```

### Writing Output

```go
tui.Write([]byte("streaming text...\n"))
tui.WriteString("convenience wrapper\n")
```

Each `Write` call appends to a line buffer. Complete lines (ending with `\n`) are rendered and pushed to the output area immediately. These methods are safe to call concurrently.

### Reading Input

```go
// Blocking — returns when user presses Enter.
input, err := tui.ReadLine()
// err != nil when user presses Ctrl+C.
```

### Status Bar

```go
tui.SetStatus("processing...", minitui.StatusWarning)
tui.SetStatus("done!", minitui.StatusSuccess)
```

### Cleanup

```go
defer tui.Close() // restores terminal state
```

### Events (optional)

```go
eventCh := make(chan minitui.Event, 8)
tui, _ := minitui.NewWithConfig(minitui.Config{EventCh: eventCh})

go func() {
    for ev := range eventCh {
        switch ev.Type {
        case minitui.EventSubmit:
            fmt.Println("submitted:", ev.Input)
        case minitui.EventResize:
            fmt.Println("resized:", ev.Width, "x", ev.Height)
        case minitui.EventInterrupt:
            fmt.Println("Ctrl+C pressed")
        }
    }
}()

// ReadLine still works normally; events are sent in addition.
for { input, _ := tui.ReadLine() { ... } }
```

## Configuration

```go
type Config struct {
    // EventCh: if non-nil, EventSubmit/EventResize/EventInterrupt are sent here.
    EventCh chan<- Event

    // RenderLine: custom markdown-to-ANSI renderer.
    // Receives one raw line, returns the ANSI-styled version.
    // When nil, built-in renderer handles: headings, **bold**, *italic*, `code`, tables.
    RenderLine func(raw string) string

    // BorderColor: ANSI escape for input-box border lines (e.g. "\x1b[34m" for blue).
    // Default: dim.
    BorderColor string

    // MaxInputRows: maximum visible rows in the input box.  Default: 8.
    MaxInputRows int
}
```

### Custom Render Example

```go
func myRenderer(raw string) string {
    // Highlight every line that starts with "> " as green.
    if strings.HasPrefix(raw, "> ") {
        return "\x1b[32m" + raw + "\x1b[0m"
    }
    return raw
}
```

## Key Bindings

| Key | Action |
|-----|--------|
| `Enter` | Submit input |
| `Shift+Enter` | Insert newline (kitty / modifyOtherKeys protocol) |
| `Ctrl+J` | Insert newline (always works) |
| `Ctrl+C` | Interrupt (returns error from ReadLine) |
| `Left/Right` | Move cursor |
| `Up/Down` | Move cursor between lines |
| `Home/End` | Jump to start/end of line |
| `Ctrl+A/E` | Jump to start/end of line |
| `Ctrl+K` | Delete to end of line |
| `Ctrl+U` | Delete to start of line |
| `Ctrl+W` | Delete previous word |
| `Backspace` | Delete character before cursor |
| `Delete / Ctrl+D` | Delete character at cursor |
| `Tab` | Insert 4 spaces |
| `Paste` | Multi-line paste (bracketed paste mode) |

## Markdown Support

| Syntax | Rendering |
|--------|-----------|
| `# Heading` | **Bold** |
| `**text**` | **Bold** |
| `*text*` | *Italic* |
| `` `code` `` | Dim/inline-code |
| ` ``` ... ``` ` | Dim block |
| `\| col \| col \|` | Aligned table |

## Dependencies

- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) — terminal raw mode and size detection

## Terminal Support

Tested on: **iTerm2**, **Terminal.app**, **WezTerm**, **Kitty**, **VS Code integrated terminal**.

- **Shift+Enter** requires kitty keyboard protocol or XTerm modifyOtherKeys (supported by modern terminals).  
  Use **Ctrl+J** as a reliable fallback everywhere.
- Bracketed paste mode is enabled for reliable multi-line paste detection.

## License

MIT
