package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongly/serialhub/pkg/mcpsetup"
)

var (
	setupClient    string
	setupURL       string
	setupMode      string
	setupScope     string
	setupAssumeYes bool
)

// newSetupCmd 构造 `serialhub setup` 子命令：为 MCP 客户端生成接入配置。
func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "为 MCP 客户端自动配置 SerialHub 接入",
		Long: "交互式向导：选择 MCP 客户端（OpenCode / Claude Code / Cursor / Windsurf / " +
			"VS Code / Codex）→ 接入模式（HTTP 或 stdio）→ 写入层级（项目级/用户级），\n" +
			"然后合并写入该客户端的配置文件（不覆盖其他条目）。\n" +
			"非交互用法：serialhub setup --client cursor --url http://127.0.0.1:5000/mcp -y",
		RunE: runSetup,
		Args: cobra.NoArgs,
	}
	cmd.Flags().StringVar(&setupClient, "client", "", "客户端 ID: opencode/claude/cursor/windsurf/vscode/codex")
	cmd.Flags().StringVar(&setupURL, "url", "http://127.0.0.1:5000/mcp", "HTTP 端点（联邦模式下 127.0.0.1:5000 两侧皆可用）")
	cmd.Flags().StringVar(&setupMode, "mode", "http", "接入模式: http | stdio")
	cmd.Flags().StringVar(&setupScope, "scope", "project", "写入层级: project | user（codex 仅 user）")
	cmd.Flags().BoolVarP(&setupAssumeYes, "yes", "y", false, "非交互：确认全部默认选择")
	return cmd
}

func runSetup(cmd *cobra.Command, args []string) error {
	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string, def string) string {
		if setupAssumeYes {
			return def
		}
		fmt.Print(prompt)
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}

	// 1. 客户端
	client := setupClient
	if client == "" {
		fmt.Println("选择 MCP 客户端:")
		for i, c := range mcpsetup.Clients {
			fmt.Printf("  %d) %s\n", i+1, c.Name)
		}
		pick := ask("请输入编号 [1]: ", "1")
		n, err := strconv.Atoi(pick)
		if err != nil || n < 1 || n > len(mcpsetup.Clients) {
			return fmt.Errorf("无效编号: %s", pick)
		}
		client = mcpsetup.Clients[n-1].ID
	}
	def, err := mcpsetup.Find(client)
	if err != nil {
		return err
	}

	// 2. 模式
	mode := mcpsetup.Mode(setupMode)
	if mode != mcpsetup.ModeHTTP && mode != mcpsetup.ModeStdio {
		return fmt.Errorf("无效模式 %q（可选 http/stdio）", setupMode)
	}
	if mode == "" {
		pick := ask("接入模式: 1) HTTP（推荐） 2) stdio（客户端自动拉起） [1]: ", "1")
		if pick == "2" {
			mode = mcpsetup.ModeStdio
		} else {
			mode = mcpsetup.ModeHTTP
		}
	}

	// 3. 层级（Codex 仅用户级）
	scope := mcpsetup.Scope(setupScope)
	if def.OnlyUser {
		scope = mcpsetup.ScopeUser
	} else if scope != mcpsetup.ScopeProject && scope != mcpsetup.ScopeUser {
		return fmt.Errorf("无效层级 %q（可选 project/user）", setupScope)
	}

	opts := mcpsetup.Options{
		Client: def.ID,
		Mode:   mode,
		URL:    setupURL,
		Scope:  scope,
		ConfirmOverwrite: func(path string) bool {
			if setupAssumeYes {
				return true
			}
			ans := ask(fmt.Sprintf("%s 中已有 serialhub 条目，覆盖更新? (y/N): ", path), "n")
			return strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")
		},
	}

	target, err := mcpsetup.Install(opts)
	if err == mcpsetup.ErrEntryExists {
		fmt.Println("已存在 serialhub 条目，按选择跳过，未做修改。")
		return nil
	}
	if err != nil {
		return err
	}

	modeDesc := string(mode)
	if mode == mcpsetup.ModeHTTP {
		modeDesc = "HTTP " + setupURL
	} else {
		modeDesc = "stdio (serialhub --stdio)"
	}
	fmt.Printf("[SerialHub] 已为 %s 写入接入配置（%s，%s 级）\n  目标: %s\n",
		def.Name, modeDesc, scope, target)
	if scope == mcpsetup.ScopeProject && !def.OnlyCLI {
		fmt.Println("  提示: 项目级配置文件可提交到版本库与团队共享。")
	}
	return nil
}
