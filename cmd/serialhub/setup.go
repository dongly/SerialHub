package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dongly/serialhub/internal/i18n"
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
		Short: i18n.CLI.SetupShort,
		Long:  i18n.CLI.SetupLong,
		RunE:  runSetup,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().StringVar(&setupClient, "client", "", i18n.CLI.SetupClient)
	cmd.Flags().StringVar(&setupURL, "url", mcpsetup.DefaultURL(), i18n.CLI.SetupURL)
	cmd.Flags().StringVar(&setupMode, "mode", "stdio", i18n.CLI.SetupMode)
	cmd.Flags().StringVar(&setupScope, "scope", "project", i18n.CLI.SetupScope)
	cmd.Flags().BoolVarP(&setupAssumeYes, "yes", "y", false, i18n.CLI.SetupYes)
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
		fmt.Println(i18n.CLI.ChooseClient)
		for i, c := range mcpsetup.Clients {
			fmt.Printf("  %d) %s\n", i+1, c.Name)
		}
		pick := ask(i18n.CLI.ChooseNumber, "1")
		n, err := strconv.Atoi(pick)
		if err != nil || n < 1 || n > len(mcpsetup.Clients) {
			return fmt.Errorf(i18n.CLI.InvalidNumber, pick)
		}
		client = mcpsetup.Clients[n-1].ID
	}
	def, err := mcpsetup.Find(client)
	if err != nil {
		return err
	}

	// 2. 模式：交互向导且未显式指定 --mode 时询问；-y 或显式指定时直接采用 flag 值
	mode := mcpsetup.Mode(setupMode)
	if !cmd.Flags().Changed("mode") && !setupAssumeYes {
		pick := ask(i18n.CLI.ChooseMode, "1")
		switch pick {
		case "", "1":
			mode = mcpsetup.ModeStdio
		case "2":
			mode = mcpsetup.ModeHTTP
		default:
			return fmt.Errorf(i18n.CLI.InvalidChoice, pick)
		}
	} else if mode != mcpsetup.ModeHTTP && mode != mcpsetup.ModeStdio {
		return fmt.Errorf(i18n.CLI.InvalidMode, setupMode)
	}

	// 3. 层级（Codex 仅用户级；仅支持单一层级的客户端强制该层级）；
	// 交互向导且未显式指定 --scope 时询问，-y 或显式指定时直接采用 flag 值。
	scope := mcpsetup.Scope(setupScope)
	if def.OnlyUser || (!def.Project && def.User) {
		scope = mcpsetup.ScopeUser
	} else if def.Project && !def.User {
		scope = mcpsetup.ScopeProject
	} else if !cmd.Flags().Changed("scope") && !setupAssumeYes {
		pick := ask(i18n.CLI.ChooseScope, "1")
		switch pick {
		case "", "1":
			scope = mcpsetup.ScopeProject
		case "2":
			scope = mcpsetup.ScopeUser
		default:
			return fmt.Errorf(i18n.CLI.InvalidChoice, pick)
		}
	}
	if scope != mcpsetup.ScopeProject && scope != mcpsetup.ScopeUser {
		return fmt.Errorf(i18n.CLI.InvalidScope, setupScope)
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
			ans := ask(fmt.Sprintf(i18n.CLI.ConfirmOverwrite, path), "n")
			return strings.EqualFold(ans, "y") || strings.EqualFold(ans, "yes")
		},
	}

	target, err := mcpsetup.Install(opts)
	if errors.Is(err, mcpsetup.ErrNoConfig) {
		// 「存在才写」：未找到配置文件属正常跳过，提示探测路径后按成功退出。
		fmt.Println(err)
		return nil
	}
	if err == mcpsetup.ErrEntryExists {
		fmt.Println(i18n.CLI.EntrySkipped)
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
	fmt.Printf(i18n.CLI.SetupDone,
		def.Name, modeDesc, scope, target)
	if scope == mcpsetup.ScopeProject && !def.OnlyCLI {
		fmt.Println(i18n.CLI.SetupHint)
	}
	return nil
}
