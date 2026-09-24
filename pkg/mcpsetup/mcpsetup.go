// Package mcpsetup 为各类 MCP 客户端生成 SerialHub 接入配置。
//
// 写入策略统一为「解析-合并-新增不覆盖」：目标 JSON 文件中的其他条目
// 原样保留；已存在 serialhub 条目时返回 ErrEntryExists，由调用方决定跳过或更新。
// Codex 与 Claude Code 的用户级配置由各自官方 CLI 保管（见每个客户端的 Install）。
package mcpsetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Mode 接入模式：HTTP（Streamable HTTP 端点）或 stdio（客户端本地拉起）。
type Mode string

const (
	ModeHTTP  Mode = "http"
	ModeStdio Mode = "stdio"
)

// Scope 写入层级：项目级（当前目录）或用户级（全局）。
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

// ErrEntryExists 表示目标配置中已有 serialhub 条目，调用方需决定是否覆盖。
var ErrEntryExists = errors.New("目标配置中已存在 serialhub 条目")

// Options 一次接入配置的全部参数。
type Options struct {
	Client  string // 客户端 ID
	Mode    Mode
	URL     string // HTTP 模式的端点，如 http://127.0.0.1:5000/mcp
	Scope   Scope  // 项目级 / 用户级
	Command string // stdio 模式的可执行文件名，默认 serialhub
	// ConfirmOverwrite 在条目已存在时被调用，返回 true 表示覆盖更新。
	ConfirmOverwrite func(path string) bool
}

// Client 描述一个 MCP 客户端的配置面。
type Client struct {
	ID       string
	Name     string
	Project  bool // 支持项目级
	User     bool // 支持用户级
	OnlyUser bool // 仅用户级（Codex）
	OnlyCLI  bool // 仅能通过官方 CLI 写入（Codex）
}

// Clients 支持的客户端清单（菜单顺序）。
var Clients = []Client{
	{ID: "opencode", Name: "OpenCode", Project: true, User: true},
	{ID: "claude", Name: "Claude Code", Project: true, User: true},
	{ID: "cursor", Name: "Cursor", Project: true, User: true},
	{ID: "windsurf", Name: "Windsurf", User: true},
	{ID: "vscode", Name: "VS Code (Copilot)", Project: true},
	{ID: "codex", Name: "Codex", User: true, OnlyUser: true, OnlyCLI: true},
}

// Find 按 ID 查找客户端定义。
func Find(id string) (Client, error) {
	for _, c := range Clients {
		if c.ID == id {
			return c, nil
		}
	}
	return Client{}, fmt.Errorf("未知客户端 %q（可选：%s）", id, clientIDs())
}

func clientIDs() string {
	s := ""
	for i, c := range Clients {
		if i > 0 {
			s += "/"
		}
		s += c.ID
	}
	return s
}

// stdioCommand 返回 stdio 模式的启动命令参数。
func (o *Options) stdioCommand() string {
	if o.Command != "" {
		return o.Command
	}
	return "serialhub"
}

// Install 把 serialhub 写入客户端配置，返回写入的文件路径或 CLI 命令描述。
func Install(opts Options) (string, error) {
	c, err := Find(opts.Client)
	if err != nil {
		return "", err
	}
	if opts.Mode == "" {
		opts.Mode = ModeHTTP
	}
	if opts.Mode == ModeHTTP && opts.URL == "" {
		opts.URL = "http://127.0.0.1:5000/mcp"
	}

	// Codex：始终走官方 CLI，仅用户级。
	if c.ID == "codex" {
		return installCodexCLI(opts)
	}
	// Claude Code 用户级：状态存于 ~/.claude.json 大 JSON，交给官方 CLI 保管。
	if c.ID == "claude" && opts.Scope == ScopeUser {
		return installClaudeUserCLI(opts)
	}
	if opts.Scope == ScopeProject && !c.Project {
		return "", fmt.Errorf("%s 不支持项目级配置", c.Name)
	}
	if opts.Scope == ScopeUser && !c.User {
		return "", fmt.Errorf("%s 不支持用户级配置", c.Name)
	}

	path, key, entry, err := target(opts)
	if err != nil {
		return "", err
	}
	if err := mergeJSON(path, key, "serialhub", entry, opts.ConfirmOverwrite); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(path)
	return abs, nil
}

// target 根据客户端与层级返回（配置文件路径、顶层键、serialhub 条目值）。
func target(opts Options) (path, topKey string, entry map[string]any, err error) {
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", "", nil, herr
	}
	m := opts.Mode
	switch opts.Client {
	case "opencode":
		if m == ModeStdio {
			entry = map[string]any{"type": "local", "command": opts.stdioCommand(), "args": []string{"--stdio"}, "enabled": true}
		} else {
			entry = map[string]any{"type": "remote", "url": opts.URL, "enabled": true}
		}
		topKey = "mcp"
		if opts.Scope == ScopeProject {
			path = "opencode.json"
		} else if runtime.GOOS == "windows" {
			path = filepath.Join(home, ".config", "opencode", "opencode.json")
		} else {
			path = filepath.Join(home, ".config", "opencode", "opencode.json")
		}
	case "claude": // 项目级 .mcp.json
		if m == ModeStdio {
			entry = map[string]any{"type": "stdio", "command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"type": "http", "url": opts.URL}
		}
		topKey, path = "mcpServers", ".mcp.json"
	case "cursor":
		if m == ModeStdio {
			entry = map[string]any{"command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"url": opts.URL}
		}
		topKey = "mcpServers"
		if opts.Scope == ScopeProject {
			path = filepath.Join(".cursor", "mcp.json")
		} else {
			path = filepath.Join(home, ".cursor", "mcp.json")
		}
	case "windsurf":
		if m == ModeStdio {
			entry = map[string]any{"command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"serverUrl": opts.URL}
		}
		topKey = "mcpServers"
		path = filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	case "vscode":
		if m == ModeStdio {
			entry = map[string]any{"type": "stdio", "command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"type": "http", "url": opts.URL}
		}
		topKey, path = "servers", filepath.Join(".vscode", "mcp.json")
	default:
		return "", "", nil, fmt.Errorf("客户端 %s 无文件配置面", opts.Client)
	}
	return path, topKey, entry, nil
}

// mergeJSON 读取 path（不存在则视为空对象），把 entry 写入 topKey.name，
// 保留其他所有键。条目已存在时经 confirm 决定是否覆盖。
func mergeJSON(path, topKey, name string, entry map[string]any, confirm func(string) bool) error {
	root := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("解析 %s 失败（不是有效 JSON）：%w", path, err)
		}
	}
	section, _ := root[topKey].(map[string]any)
	if section == nil {
		section = map[string]any{}
	}
	if _, exists := section[name]; exists {
		if confirm == nil || !confirm(path) {
			return ErrEntryExists
		}
	}
	section[name] = entry
	root[topKey] = section

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, out, 0o644)
}

// installCodexCLI 通过 codex 官方 CLI 写入（仅用户级，~/.codex/config.toml）。
func installCodexCLI(opts Options) (string, error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return "", fmt.Errorf("未找到 codex 命令，请先安装 Codex CLI；手动配置：~/.codex/config.toml 中添加 [mcp_servers.serialhub]")
	}
	args := []string{"mcp", "add", "serialhub"}
	if opts.Mode == ModeStdio {
		args = append(args, "--", opts.stdioCommand(), "--stdio")
	} else {
		args = append(args, "--url", opts.URL)
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("codex mcp add 失败：%v\n%s", err, out)
	}
	return "~/.codex/config.toml（经 codex mcp add 写入）", nil
}

// installClaudeUserCLI 通过 claude 官方 CLI 写入用户级（~/.claude.json）。
func installClaudeUserCLI(opts Options) (string, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("未找到 claude 命令，请先安装 Claude Code；或改用项目级 .mcp.json")
	}
	var cmd *exec.Cmd
	if opts.Mode == ModeStdio {
		cmd = exec.Command(bin, "mcp", "add", "serialhub", "--scope", "user", "--", opts.stdioCommand(), "--stdio")
	} else {
		cmd = exec.Command(bin, "mcp", "add", "--transport", "http", "serialhub", opts.URL, "--scope", "user")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude mcp add 失败：%v\n%s", err, out)
	}
	return "~/.claude.json（经 claude mcp add 写入）", nil
}
