// Demo: echo server with configurable border color, event callbacks, and custom rendering.
// Press Enter to submit, Shift+Enter or Ctrl+J for newline, Ctrl+C to quit.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/tim5wang/min-tui"
)

func main() {
	// Optional: event channel for async input handling.
	eventCh := make(chan minitui.Event, 8)
	go func() {
		for ev := range eventCh {
			switch ev.Type {
			case minitui.EventResize:
				// Terminal was resized.
			case minitui.EventSubmit:
				// Input received (also returned by ReadLine).
				_ = ev.Input
			case minitui.EventInterrupt:
				// Ctrl+C pressed.
			}
		}
	}()

	tui, err := minitui.NewWithConfig(minitui.Config{
		EventCh:    eventCh,
		BorderColor: "\x1b[36m", // cyan borders
		// RenderLine: customRender,  // uncomment to use custom markdown renderer
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { close(eventCh); tui.Close() }()

	tui.SetStatus("Enter 提交 | Shift+Enter/Ctrl+J 换行 | Ctrl+C 退出", minitui.StatusInfo)

	for {
		input, err := tui.ReadLine()
		if err != nil {
			tui.WriteString("\n再见！\n")
			time.Sleep(500 * time.Millisecond)
			return
		}

		tui.SetStatus("正在处理...", minitui.StatusWarning)
		time.Sleep(200 * time.Millisecond)

		// 流式逐字输出
		for _, r := range input {
			tui.WriteString(string(r))
			time.Sleep(30 * time.Millisecond)
		}

		tui.WriteString("\n\n---\n")
		tui.WriteString("**以上是你的输入**\n")
		tui.WriteString("\nMarkdown 演示：\n\n")
		tui.WriteString("# 一级标题\n")
		tui.WriteString("**粗体** 和 *斜体* `代码`\n")
		tui.WriteString("\n表格示例：\n")
		tui.WriteString("| 名称 | 数量 | 备注 |\n")
		tui.WriteString("|------|------|------|\n")
		tui.WriteString("| 苹果 | 10 | 新鲜 |\n")
		tui.WriteString("| 香蕉 | 5 | - |\n")
		tui.WriteString("| 橙子 | 20 | 进口 |\n")
		tui.WriteString("\n你好世界 🌍 中文测试\n\n")

		tui.SetStatus("Enter 提交 | Shift+Enter/Ctrl+J 换行 | Ctrl+C 退出", minitui.StatusInfo)
	}
}

// Example custom renderer (uncomment RenderLine above to use).
// func customRender(raw string) string {
// 	return "\x1b[35m" + raw + "\x1b[0m" // all text in magenta
// }
