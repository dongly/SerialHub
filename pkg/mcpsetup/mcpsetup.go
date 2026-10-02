// Package mcpsetup 为各类 MCP 客户端生成 SerialHub 接入配置。
//
// 写入策略统一为「解析-合并-新增不覆盖」：目标 JSON 文件中的其他条目
// 原样保留；已存在 serialhub 条目时返回 ErrEntryExists，由调用方决定跳过或更新。
// Codex 与 Claude Code 的用户级配置由各自官方 CLI 保管（见每个客户端的 Install）。
package mcpsetup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/pkg/config"
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
var ErrEntryExists = errors.New(i18n.MCPSetupInstall.EntryExists)

// Options 一次接入配置的全部参数。
type Options struct {
	Client  string // 客户端 ID
	Mode    Mode
	URL     string // HTTP 模式的端点，如 http://127.0.0.1:5050/mcp
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
	return Client{}, fmt.Errorf(i18n.MCPSetupInstall.UnknownClient, id, clientIDs())
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

// DefaultURL 返回 HTTP 模式的默认端点（端口取 config.DefaultHTTPPort）。
func DefaultURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/mcp", config.DefaultHTTPPort)
}

// Install 把 serialhub 写入客户端配置，返回写入的文件路径或 CLI 命令描述。
func Install(opts Options) (string, error) {
	c, err := Find(opts.Client)
	if err != nil {
		return "", err
	}
	if opts.Mode == "" {
		opts.Mode = ModeStdio // 默认本地模式：客户端拉起 serialhub --stdio，无需先启动服务
	}
	if opts.Mode == ModeHTTP && opts.URL == "" {
		opts.URL = DefaultURL()
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
		return "", fmt.Errorf(i18n.MCPSetupInstall.UnsupportedProject, c.Name)
	}
	if opts.Scope == ScopeUser && !c.User {
		return "", fmt.Errorf(i18n.MCPSetupInstall.UnsupportedUser, c.Name)
	}

	path, key, entry, err := target(opts)
	if err != nil {
		return "", err
	}
	written, err := mergeJSON(path, key, "serialhub", entry, opts.ConfirmOverwrite)
	if err != nil {
		return "", err
	}
	if opts.Client == "opencode" {
		removeLegacyOpenCodeFlat(path) // 传原始目标：json/jsonc 并存时两个都清理
	}
	abs, _ := filepath.Abs(written)
	return abs, nil
}

// configTarget 根据客户端与层级返回（配置文件路径、顶层键）。
// 路径与接入模式无关，Install/Uninstall 共用。
func configTarget(client string, scope Scope) (path, topKey string, err error) {
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", "", herr
	}
	switch client {
	case "opencode":
		topKey = "mcp.servers"
		if scope == ScopeProject {
			path = "opencode.json"
		} else {
			path = filepath.Join(home, ".config", "opencode", "opencode.json")
		}
	case "claude": // 项目级 .mcp.json
		topKey, path = "mcpServers", ".mcp.json"
	case "cursor":
		topKey = "mcpServers"
		if scope == ScopeProject {
			path = filepath.Join(".cursor", "mcp.json")
		} else {
			path = filepath.Join(home, ".cursor", "mcp.json")
		}
	case "windsurf":
		topKey = "mcpServers"
		path = filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	case "vscode":
		topKey, path = "servers", filepath.Join(".vscode", "mcp.json")
	default:
		return "", "", fmt.Errorf(i18n.MCPSetupInstall.NoFileTarget, client)
	}
	return path, topKey, nil
}

// target 根据客户端与层级返回（配置文件路径、顶层键、serialhub 条目值）。
func target(opts Options) (path, topKey string, entry map[string]any, err error) {
	path, topKey, err = configTarget(opts.Client, opts.Scope)
	if err != nil {
		return "", "", nil, err
	}
	m := opts.Mode
	switch opts.Client {
	case "opencode":
		// OpenCode V2 要求 MCP 服务器嵌套在 mcp.servers 下（V1 的扁平 mcp.<name> 已废弃）；
		// local 的 command 是「可执行文件+参数」数组（无独立 args 字段），
		// 停用状态用 disabled（无 enabled 字段），SerialHub 端点无鉴权故关闭 OAuth。
		if m == ModeStdio {
			entry = map[string]any{"type": "local", "command": append([]string{opts.stdioCommand()}, "--stdio")}
		} else {
			entry = map[string]any{"type": "remote", "url": opts.URL, "oauth": false}
		}
	case "claude":
		if m == ModeStdio {
			entry = map[string]any{"type": "stdio", "command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"type": "http", "url": opts.URL}
		}
	case "cursor", "windsurf":
		if m == ModeStdio {
			entry = map[string]any{"command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else if opts.Client == "cursor" {
			entry = map[string]any{"url": opts.URL}
		} else {
			entry = map[string]any{"serverUrl": opts.URL}
		}
	case "vscode":
		if m == ModeStdio {
			entry = map[string]any{"type": "stdio", "command": opts.stdioCommand(), "args": []string{"--stdio"}}
		} else {
			entry = map[string]any{"type": "http", "url": opts.URL}
		}
	}
	return path, topKey, entry, nil
}

// mergeJSON 读取配置文件并把 entry 写入 topKey.name，保留其他所有键与注释。
// 「存在才写」：文件缺失或内容空白时报 ErrNoConfig（NoConfigError 携带探测路径），不创建新文件；
// OpenCode 且 json/jsonc 并存时写入 jsonc（findConfig 决定真实路径）。
// topKey 支持点分嵌套路径（如 "mcp.servers"，缺失的中间层补齐）。
// 条目已存在时经 confirm(realPath) 决定是否覆盖。返回实际写入的文件路径。
func mergeJSON(path, topKey, name string, entry map[string]any, confirm func(string) bool) (string, error) {
	real, raw, ok, err := findConfig(path)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", &NoConfigError{Paths: configCandidates(path)}
	}
	v, perr := hujson.Parse(raw)
	if perr != nil {
		return "", fmt.Errorf(i18n.MCPSetupInstall.InvalidJSON, real, perr)
	}
	unit := jsonIndentUnit(raw)
	depth := len(strings.Split(topKey, ".")) + 1 // 条目成员所在层级（root 成员为 1）
	obj, err := ensureContainer(&v, topKey, unit)
	if err != nil {
		return "", err
	}
	if idx := findMember(obj, name); idx >= 0 {
		if confirm == nil || !confirm(real) {
			return "", ErrEntryExists
		}
	}
	if err := setEntry(obj, name, entry, unit, depth); err != nil {
		return "", err
	}
	return real, os.WriteFile(real, packWithNewline(&v), 0o644)
}

// removeLegacyOpenCodeFlat 迁移清理：v0.5.0 及以前写入过 V1 扁平结构
// mcp.serialhub（OpenCode V2 不加载该位置），检测到即删除，尽力而为不报错。
// json/jsonc 并存时两个文件都清理，注释经 hujson AST 保留。
func removeLegacyOpenCodeFlat(path string) {
	files, err := findConfigAll(path)
	if err != nil {
		return
	}
	for _, f := range files {
		v, perr := hujson.Parse(f.raw)
		if perr != nil {
			continue
		}
		if !removeEntry(&v, "mcp", "serialhub") {
			continue
		}
		_ = os.WriteFile(f.path, packWithNewline(&v), 0o644)
	}
}

// installCodexCLI 通过 codex 官方 CLI 写入（仅用户级，~/.codex/config.toml）。
// 「存在才写」：配置缺失或空白时跳过（不先创建文件再写）。
func installCodexCLI(opts Options) (string, error) {
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", fmt.Errorf(i18n.MCPSetupInstall.HomeDir, herr)
	}
	if err := requireConfigFile(filepath.Join(home, ".codex", "config.toml")); err != nil {
		return "", err
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		return "", errors.New(i18n.MCPSetupInstall.MissingCodex)
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
		return "", fmt.Errorf(i18n.MCPSetupInstall.CodexFailed, err, out)
	}
	return i18n.MCPSetupInstall.CodexAdded, nil
}

// installClaudeUserCLI 通过 claude 官方 CLI 写入用户级（~/.claude.json）。
// 「存在才写」：配置缺失或空白时跳过（不先创建文件再写）。
func installClaudeUserCLI(opts Options) (string, error) {
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", fmt.Errorf(i18n.MCPSetupInstall.HomeDir, herr)
	}
	if err := requireConfigFile(filepath.Join(home, ".claude.json")); err != nil {
		return "", err
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", errors.New(i18n.MCPSetupInstall.MissingClaude)
	}
	var cmd *exec.Cmd
	if opts.Mode == ModeStdio {
		cmd = exec.Command(bin, "mcp", "add", "serialhub", "--scope", "user", "--", opts.stdioCommand(), "--stdio")
	} else {
		cmd = exec.Command(bin, "mcp", "add", "--transport", "http", "serialhub", opts.URL, "--scope", "user")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(i18n.MCPSetupInstall.ClaudeFailed, err, out)
	}
	return i18n.MCPSetupInstall.ClaudeAdded, nil
}
