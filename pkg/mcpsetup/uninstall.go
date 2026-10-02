package mcpsetup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tailscale/hujson"

	"github.com/dongly/serialhub/internal/i18n"
)

// UninstallOptions 一次移除接入配置的参数。
type UninstallOptions struct {
	Client string // 客户端 ID
	Scope  Scope  // 项目级 / 用户级
}

// UninstallFrom 从客户端配置中移除 serialhub 条目，返回（结果描述、是否实际移除、是否待手动处理）。
// 幂等：条目不存在时返回 (描述, false, false, nil)；Codex 与 Claude 用户级走官方 CLI。
// CLI 缺失时的手动指引 manual=true，调用方不应将其计为完全成功。
func UninstallFrom(opts UninstallOptions) (string, bool, bool, error) {
	c, err := Find(opts.Client)
	if err != nil {
		return "", false, false, err
	}
	if c.ID == "codex" {
		return uninstallCodexCLI()
	}
	if c.ID == "claude" && opts.Scope == ScopeUser {
		return uninstallClaudeUserCLI()
	}
	if opts.Scope == ScopeProject && !c.Project {
		return "", false, false, fmt.Errorf(i18n.MCPSetupRemoval.NoProject, c.Name)
	}
	if opts.Scope == ScopeUser && !c.User {
		return "", false, false, fmt.Errorf(i18n.MCPSetupRemoval.NoUser, c.Name)
	}
	if opts.Scope != ScopeProject && opts.Scope != ScopeUser {
		return "", false, false, fmt.Errorf(i18n.MCPSetupRemoval.BadScope, opts.Scope)
	}

	path, topKey, err := configTarget(c.ID, opts.Scope)
	if err != nil {
		return "", false, false, err
	}
	removed, err := uninstallJSON(path, topKey, "serialhub")
	if err != nil {
		return "", false, false, err
	}
	// 展示真实写入/移除的文件（OpenCode json/jsonc 并存时可能是 opencode.jsonc）。
	real := path
	if r, _, found, ferr := findConfig(path); ferr == nil && found {
		real = r
	}
	abs, _ := filepath.Abs(real)
	if removed {
		return fmt.Sprintf(i18n.MCPSetupRemoval.Removed, abs), true, false, nil
	}
	return fmt.Sprintf(i18n.MCPSetupRemoval.Absent, abs), false, false, nil
}

// EntryExists 探测某客户端的接入配置面是否存在（配置文件存在即视为待检查）。
// CLI 型（codex / claude 用户级）无法廉价探测文件，恒返回 true 交由移除时判断。
func EntryExists(opts UninstallOptions) bool {
	c, err := Find(opts.Client)
	if err != nil {
		return false
	}
	if c.ID == "codex" || (c.ID == "claude" && opts.Scope == ScopeUser) {
		return true
	}
	path, _, err := configTarget(c.ID, opts.Scope)
	if err != nil {
		return false
	}
	_, _, ok, ferr := findConfig(path)
	return ferr == nil && ok
}

// uninstallJSON 是 mergeJSON 的逆操作：从 topKey.name 删除条目，
// 自深向浅清理空容器（如 mcp.servers 清空后连 mcp 一起删，root 本身保留）。
// 文件不存在或内容空白、条目不存在时返回 (false, nil)，幂等；其他读写错误如实返回。
// json/jsonc 并存时两个文件都探测（条目在哪个文件就从哪个移除）：先全部解析并删除、
// 全部成功后才写回，避免半更新；写回用 hujson AST 保留注释，采用同目录临时文件 +
// 原子替换，替换前重读比较做乐观冲突检测（与客户端并发写撞车时中止而非覆盖其改动）。
// 符号链接配置（dotfiles 管理）在 findConfigAll 中解析到真实目标上操作，不拆链接。
func uninstallJSON(path, topKey, name string) (bool, error) {
	files, err := findConfigAll(path)
	if err != nil {
		return false, err
	}
	type pending struct {
		path string
		orig []byte
		next []byte
	}
	var edits []pending
	for _, f := range files {
		v, perr := hujson.Parse(f.raw)
		if perr != nil {
			return false, fmt.Errorf(i18n.MCPSetupRemoval.Parse, f.path, perr)
		}
		if !removeEntry(&v, topKey, name) {
			continue
		}
		edits = append(edits, pending{path: f.path, orig: f.raw, next: packWithNewline(&v)})
	}
	removed := len(edits) > 0
	for _, e := range edits {
		// 乐观冲突检测在 writeFileAtomic 内紧邻 rename 处执行，窗口缩到最小；
		// 仍是尽力而非完全并发安全（跨进程读-改-写锁不在本工具范围内）。
		if err := writeFileAtomic(e.path, e.orig, e.next); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// resolveSymlinkPath 在 path 是符号链接时返回其指向的真实文件路径，
// 使后续读写在真实文件上进行（原子替换真实文件而非拆掉链接本身）。
// 解析失败（悬空链接）时原样返回：后续 ReadFile 将得到 IsNotExist，
// 按条目不存在幂等跳过（不创建文件、不拆链接）。
func resolveSymlinkPath(path string) string {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return path
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// writeFileAtomic 在同目录写临时文件后原子替换 path，并保留原文件权限。
// orig 是调用方读取的原始快照：rename 前重读比较做乐观冲突检测，
// 文件已被外部修改（或不一致）时中止并保留原文件，不用旧快照覆盖新改动。
func writeFileAtomic(path string, orig, data []byte) error {
	info, err := os.Stat(path)
	mode := os.FileMode(0o644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".serialhub-uninstall-*")
	if err != nil {
		return fmt.Errorf(i18n.MCPSetupRemoval.CreateTemp, err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf(i18n.MCPSetupRemoval.WriteTemp, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf(i18n.MCPSetupRemoval.CloseTemp, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf(i18n.MCPSetupRemoval.ChmodTemp, err)
	}
	// 冲突检测尽量靠近提交点：临时文件已就绪，rename 前重读比较。
	// 重读失败（权限变化等）同样中止写回，交由调用方处理。
	cur, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf(i18n.MCPSetupRemoval.Reread, path, err)
	}
	if !bytes.Equal(cur, orig) {
		return fmt.Errorf(i18n.MCPSetupRemoval.Modified, path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf(i18n.MCPSetupRemoval.Replace, path, err)
	}
	ok = true
	return nil
}

// uninstallCodexCLI 经 codex 官方 CLI 移除（仅用户级，~/.codex/config.toml）。
// CLI 缺失时的手动指引属于「待手动清理」，返回的 ok=false 且 manual=true。
func uninstallCodexCLI() (desc string, removed, manual bool, err error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return i18n.MCPSetupRemoval.NoCodex, false, true, nil
	}
	has, listErr := cliHasServer(bin, "mcp")
	if listErr == nil && !has {
		return i18n.MCPSetupRemoval.CodexAbsent, false, false, nil
	}
	out, err := cliRunWithTimeout(10*time.Second, bin, "mcp", "remove", "serialhub")
	if err != nil {
		return "", false, false, fmt.Errorf(i18n.MCPSetupRemoval.CodexFailed, err, out)
	}
	return i18n.MCPSetupRemoval.CodexRemoved, true, false, nil
}

// uninstallClaudeUserCLI 经 claude 官方 CLI 移除用户级条目（~/.claude.json）。
func uninstallClaudeUserCLI() (desc string, removed, manual bool, err error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return i18n.MCPSetupRemoval.NoClaude, false, true, nil
	}
	has, listErr := cliHasServer(bin, "mcp")
	if listErr == nil && !has {
		return i18n.MCPSetupRemoval.ClaudeAbsent, false, false, nil
	}
	out, err := cliRunWithTimeout(10*time.Second, bin, "mcp", "remove", "serialhub", "--scope", "user")
	if err != nil {
		return "", false, false, fmt.Errorf(i18n.MCPSetupRemoval.ClaudeFailed, err, out)
	}
	return i18n.MCPSetupRemoval.ClaudeRemoved, true, false, nil
}

// serialhubWord 匹配作为独立单词出现的 serialhub（前后是非单词字符或边界），
// 避免误匹配 serialhub-backup 之类的其他名称。
var serialhubWord = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_-])serialhub(?:$|[^a-zA-Z0-9_-])`)

// cliHasServer 调用 `<bin> args... list` 检查是否注册了 serialhub（按词匹配）。
// list 失败（如旧版 CLI 无该子命令）时保守视为存在，交由 remove 决定成败。
// 10s 超时防 list 探测服务连接时长时间阻塞。
// 已知限制：按整段 list 输出匹配，无法区分 scope（如 Claude 仅项目级有条目时
// 也会触发用户级 remove，由 remove 自身幂等兜底）；其他服务 command 路径
// 恰含 serialhub 目录名时可能误判存在。官方 CLI 提供结构化查询后再改进。
func cliHasServer(bin string, args ...string) (bool, error) {
	out, err := cliRunWithTimeout(10*time.Second, bin, append(args, "list")...)
	if err != nil {
		return true, err
	}
	return serialhubWord.Match(bytes.ToLower(out)), nil
}

// cliRunWithTimeout 带超时执行外部 CLI 并返回合并输出（list/remove 统一走这里，
// 防止 CLI 卡在服务探测或交互输入上阻塞卸载流程）。
func cliRunWithTimeout(timeout time.Duration, bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, bin, args...).CombinedOutput()
}
