// Demo: echo server with configurable border color, event callbacks, and custom rendering.
// Press Enter to submit, Shift+Enter or Ctrl+J for newline, Ctrl+C to quit.
package main

import (
	"fmt"
	"os"
	"strings"
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

	// Global key handler: Ctrl+P opens an interactive popup.
	tui.SetGlobalKeyHandler(func(k minitui.KeyEvent) bool {
		if k.Ctrl && k.Rune == 'p' {
			page := 0
			pages := [][]string{
				{"", "  Enter       Submit input", "  Shift+Enter Newline", "  Ctrl+J      Newline (fallback)", "  /           Slash commands", "  Ctrl+P      This popup", "  Ctrl+C      Quit", "  Tab         Switch focus", "", "  ←→ page 1/2"},
				{"", "  ↑↓          Navigate popups", "  Esc         Close popup", "  Tab toggles input↔popup", "  Popup shows active", "  or dimmed border", "", "  ←→ page 1/2"},
			}
			tui.PushPopup(minitui.Popup{
				Title:       "Key Bindings",
				Width:       42, Height: 13,
				BorderColor: "\x1b[35m",
				BgColor:     "\x1b[47;30m",
				Render: func(w, h int) []string {
					return pages[page]
				},
				OnKey: func(k minitui.KeyEvent) minitui.PopupAction {
					if k.Special == minitui.KeyLeft || k.Special == minitui.KeyRight {
						page = (page + 1) % len(pages)
						return minitui.PopupUpdate
					}
					return minitui.PopupPassthrough
				},
			})
			return true
		}
		return false
	})

	// Register slash commands — type / in the input box to see the dropdown.
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "help", Description: "显示帮助信息",
		Handler: func(ctx *minitui.CommandContext) {
			ctx.Write("\n**帮助**\n")
			ctx.Write("- Enter 提交 | Shift+Enter/Ctrl+J 换行\n")
			ctx.Write("- /help /echo /login 等命令\n")
			ctx.Write("- Ctrl+C 退出\n\n")
		},
	})
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "echo", Description: "回显输入的参数",
		Handler: func(ctx *minitui.CommandContext) {
			ctx.Write("\n**Echo:** " + ctx.Args + "\n\n")
		},
	})
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "login", Description: "多轮交互示例 — 二级菜单选择",
		Handler: func(ctx *minitui.CommandContext) {
			// 二级菜单：选择登录方式
			method := ctx.Select("选择登录方式", []minitui.SelectOption{
				{Label: "password", Description: "用户名 + 密码登录"},
				{Label: "token", Description: "Token 认证登录"},
				{Label: "guest", Description: "访客模式"},
			})
			if method < 0 {
				ctx.Write("\n已取消\n\n")
				return
			}

			switch method {
			case 0: // password
				ctx.SetStatus("请输入用户名", minitui.StatusWarning)
				username := ctx.Prompt("")
				if username == "" {
					ctx.Write("\n已取消\n\n")
					return
				}
				ctx.SetStatus("请输入密码", minitui.StatusWarning)
				password := ctx.Prompt("")
				ctx.Write("\n**密码登录成功**\n用户: " + username + "\n\n")
				_ = password
			case 1: // token
				ctx.SetStatus("请输入 Token", minitui.StatusWarning)
				token := ctx.Prompt("")
				ctx.Write("\n**Token 认证成功**\nToken: " + strings.Repeat("*", len(token)) + "\n\n")
			case 2: // guest
				ctx.Write("\n**访客模式** — 仅浏览权限\n\n")
			}
			ctx.SetStatus("就绪 | / 唤起命令", minitui.StatusSuccess)
		},
	})

	// Extra commands to test dropdown scrolling.
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("cmd%d", i)
		tui.RegisterCommand(minitui.SlashCommand{
			Name: name, Description: fmt.Sprintf("测试命令 %d", i),
			Handler: func(ctx *minitui.CommandContext) {
				ctx.Write("\n执行: " + ctx.Args + "\n\n")
			},
		})
	}

	tui.SetStatus("输入 / 唤起命令 | Enter 提交 | Shift+Enter/Ctrl+J 换行", minitui.StatusInfo)

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
