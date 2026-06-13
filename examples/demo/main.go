// Demo: showcase all min-tui features — streaming output, markdown,
// code highlighting, slash commands, multi-turn interaction, and popups.
// Press Enter to submit · Shift+Enter / Ctrl+J for newline · Ctrl+C to quit.
package main

import (
	"fmt"
	"os"
	"time"

	minitui "github.com/tim5wang/min-tui"
)

func main() {
	tui, err := minitui.NewWithConfig(minitui.Config{
		BorderColor:      "\x1b[36m", // cyan input borders
		ShowHeadingMarks: true,       // show ## marks
		Spacious:         true,       // blank lines between blocks
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer tui.Close()

	// ── popup: Ctrl+P opens interactive key-bindings window ──────
	tui.SetGlobalKeyHandler(func(k minitui.KeyEvent) bool {
		if k.Ctrl && k.Rune == 'p' {
			page := 0
			pages := [][]string{
				{"", " Enter        Submit input", " Shift+Enter  Newline", " Ctrl+J       Newline (fallback)", " /            Slash commands", " Ctrl+P       This popup", " Ctrl+C       Quit", " Tab          Switch focus", "", " ←→ page 1/2"},
				{"", " Built-in features:", "", " • Markdown: headings **bold** *italic* `code`", " • Tables with aligned columns", " • Code blocks with syntax highlighting", " • Slash commands with /dropdown", " • Multi-turn interaction (Prompt/Select)", " • Popup windows (Tab focus)", "", " ←→ page 1/2"},
			}
			tui.PushPopup(minitui.Popup{
				Title: "Key Bindings",
				Width: 44, Height: 13,
				BorderColor: "\x1b[35m",
				BgColor:     "\x1b[47;30m",
				Render:      func(w, h int) []string { return pages[page] },
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

	// ── slash commands ───────────────────────────────────────────
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "help", Description: "显示帮助信息",
		Handler: func(ctx *minitui.CommandContext) {
			ctx.Write("\n**min-tui 帮助**\n\n")
			ctx.Write("- 输入文字后按 **Enter** 提交\n")
			ctx.Write("- **Shift+Enter** / **Ctrl+J** 换行\n")
			ctx.Write("- 输入 **/** 唤起命令面板\n")
			ctx.Write("- **Ctrl+P** 弹出快捷键帮助\n\n")
		},
	})
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "echo", Description: "回显参数",
		Handler: func(ctx *minitui.CommandContext) {
			ctx.Write("\n**Echo:** " + ctx.Args + "\n\n")
		},
	})
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "code", Description: "展示代码高亮",
		Handler: func(ctx *minitui.CommandContext) {
			lang := ctx.Args
			if lang == "" {
				lang = "go"
			}
			ctx.Write("```" + lang + "\n")
			ctx.Write(sampleCode(lang))
			ctx.Write("```\n\n")
		},
	})
	tui.RegisterCommand(minitui.SlashCommand{
		Name: "login", Description: "多轮交互 — 二级菜单选择",
		Handler: func(ctx *minitui.CommandContext) {
			method := ctx.Select("选择登录方式", []minitui.SelectOption{
				{Label: "password", Description: "用户名 + 密码"},
				{Label: "token", Description: "Token 认证"},
				{Label: "guest", Description: "访客模式"},
				{Label: "oauth", Description: "OAuth 2.0"},
				{Label: "sso", Description: "单点登录"},
				{Label: "ldap", Description: "LDAP 目录"},
				{Label: "api_key", Description: "API Key"},
				{Label: "mfa", Description: "多因素认证"},
				{Label: "qr", Description: "扫码登录"},
			})
			if method < 0 {
				ctx.Write("\n已取消\n\n")
				return
			}
			switch method {
			case 0:
				username := ctx.Prompt("用户名")
				if username == "" {
					return
				}
				ctx.Prompt("密码")
				ctx.Write("\n**密码登录成功** — 用户: " + username + "\n\n")
			case 1:
				ctx.Prompt("Token")
				ctx.Write("\n**Token 认证成功**\n\n")
			case 2:
				ctx.Write("\n**访客模式** — 仅浏览权限\n\n")
			default:
				ctx.Write("\n**" + []string{"OAuth", "SSO", "LDAP", "API Key", "MFA", "扫码"}[method-3] + "** 认证完成\n\n")
			}
			ctx.SetStatus("就绪 | / 唤起命令", minitui.StatusSuccess)
		},
	})
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("cmd%d", i)
		tui.RegisterCommand(minitui.SlashCommand{
			Name: name, Description: fmt.Sprintf("测试命令 %d", i),
			Handler: func(ctx *minitui.CommandContext) {
				ctx.Write("\n执行: " + ctx.Args + "\n\n")
			},
		})
	}

	tui.SetStatus("输入文字后 Enter 提交 | / 唤起命令 | Ctrl+P 弹窗", minitui.StatusInfo)

	// ── main loop ────────────────────────────────────────────────
	for {
		input, err := tui.ReadLine()
		if err != nil {
			tui.WriteString("\n再见！\n")
			time.Sleep(10 * time.Millisecond)
			return
		}

		tui.SetStatus("处理中...", minitui.StatusWarning)
		time.Sleep(10 * time.Millisecond)

		// Stream input back character by character.
		for _, r := range input {
			tui.WriteString(string(r))
			time.Sleep(25 * time.Millisecond)
		}

		// Markdown demo.
		tui.WriteString("\n\n---\n**以上是你的输入**\n\n")
		tui.WriteString("# 一级标题\n**粗体** *斜体* `行内代码`\n\n")
		tui.WriteString("> 这是引用文本 — 背景灰底\n")
		tui.WriteString("> 用于展示用户说的话\n\n")
		tui.WriteString("| 名称 | 数量 | 备注 |\n|------|------|------|\n| 苹果 | 10 | 新鲜 |\n| 香蕉 | 5 | - |\n| 橙子 | 20 | 进口 |\n\n")

		// Code highlighting demo.
		tui.WriteString("**代码高亮示例：**\n\n")
		tui.WriteString("```go\n")
		tui.WriteString("func greet(name string) string {\n")
		tui.WriteString("    // return a greeting\n")
		tui.WriteString("    return fmt.Sprintf(\"Hello, %s!\", name)\n")
		tui.WriteString("}\n")
		tui.WriteString("```\n\n")

		tui.WriteString("```python\n")
		tui.WriteString("def fibonacci(n: int) -> int:\n")
		tui.WriteString("    # compute the nth fibonacci number\n")
		tui.WriteString("    a, b = 0, 1\n")
		tui.WriteString("    for _ in range(n):\n")
		tui.WriteString("        a, b = b, a + b\n")
		tui.WriteString("    return a\n")
		tui.WriteString("```\n\n")

		tui.WriteString("```sql\n")
		tui.WriteString("-- active users from last week\n")
		tui.WriteString("SELECT name, email\n")
		tui.WriteString("FROM users\n")
		tui.WriteString("WHERE active = true\n")
		tui.WriteString("  AND last_login > '2026-01-01'\n")
		tui.WriteString("ORDER BY name ASC;\n")
		tui.WriteString("```\n\n")

		tui.SetStatus("Enter 提交 | / 唤起命令 | Ctrl+P 快捷键 | Shift+Enter 换行", minitui.StatusInfo)
	}
}

func sampleCode(lang string) string {
	switch lang {
	case "python", "py":
		return "def greet(name):\n    return f\"Hello, {name}\"\n"
	case "js", "javascript", "ts", "typescript":
		return "function greet(name: string): string {\n    return `Hello, ${name}`;\n}\n"
	case "rust", "rs":
		return "fn greet(name: &str) -> String {\n    format!(\"Hello, {name}!\")\n}\n"
	case "sql":
		return "SELECT * FROM users WHERE name = 'admin';\n"
	default:
		return "func greet(name string) string {\n    return fmt.Sprintf(\"Hello, %s!\", name)\n}\n"
	}
}
