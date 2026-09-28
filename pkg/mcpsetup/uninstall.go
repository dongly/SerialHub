package mcpsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
		return "", false, false, fmt.Errorf("%s 不支持项目级配置", c.Name)
	}
	if opts.Scope == ScopeUser && !c.User {
		return "", false, false, fmt.Errorf("%s 不支持用户级配置", c.Name)
	}
	if opts.Scope != ScopeProject && opts.Scope != ScopeUser {
		return "", false, false, fmt.Errorf("无效层级 %q（可选 project/user）", opts.Scope)
	}

	path, topKey, err := configTarget(c.ID, opts.Scope)
	if err != nil {
		return "", false, false, err
	}
	removed, err := uninstallJSON(path, topKey, "serialhub")
	if err != nil {
		return "", false, false, err
	}
	abs, _ := filepath.Abs(path)
	if removed {
		return fmt.Sprintf("已从 %s 移除 serialhub 条目", abs), true, false, nil
	}
	return fmt.Sprintf("%s 中无 serialhub 条目，跳过", abs), false, false, nil
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
	_, err = os.Stat(path)
	return err == nil
}

// uninstallJSON 是 mergeJSON 的逆操作：从 topKey.name 删除条目，
// 自深向浅清理空容器（如 mcp.servers 清空后连 mcp 一起删，root 本身保留）。
// 文件不存在或条目不存在时返回 (false, nil)，幂等；其他读写错误如实返回。
// 写回采用同目录临时文件 + 原子替换，避免截断原文件导致其他配置丢失；
// 替换前重读比较做乐观冲突检测（与客户端并发写撞车时中止而非覆盖其改动）。
// 符号链接配置（dotfiles 管理）会解析到真实目标上操作，不拆链接。
func uninstallJSON(path, topKey, name string) (bool, error) {
	path = resolveSymlinkPath(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	if len(raw) == 0 {
		return false, nil
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("解析 %s 失败（不是有效 JSON）：%w", path, err)
	}
	parts := strings.Split(topKey, ".")
	cur := root
	chain := []map[string]any{root}
	for _, part := range parts {
		next, ok := cur[part].(map[string]any)
		if !ok {
			return false, nil
		}
		cur = next
		chain = append(chain, cur)
	}
	if _, exists := cur[name]; !exists {
		return false, nil
	}
	// 乐观冲突检测：从读取到写回之间文件若被外部修改（客户端或另一个
	// setup 进程新增了其他服务），中止卸载写回，避免用旧快照覆盖丢掉新改动。
	// 检查在 writeFileAtomic 内紧邻 rename 处执行，窗口缩到最小；
	// 仍是尽力而非完全并发安全（跨进程读-改-写锁不在本工具范围内）。
	delete(cur, name)
	// 自深向浅删除空容器；chain[i] 对应 parts[i-1] 键，root（chain[0]）不删。
	for i := len(chain) - 1; i >= 1; i-- {
		if len(chain[i]) != 0 {
			break
		}
		delete(chain[i-1], parts[i-1])
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	return true, writeFileAtomic(path, raw, append(out, '\n'))
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
		return fmt.Errorf("创建临时文件失败：%w", err)
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
		return fmt.Errorf("写入临时文件失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败：%w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("设置临时文件权限失败：%w", err)
	}
	// 冲突检测尽量靠近提交点：临时文件已就绪，rename 前重读比较。
	// 重读失败（权限变化等）同样中止写回，交由调用方处理。
	cur, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("提交前重读 %s 失败（疑似被外部修改或权限变化）：%w", path, err)
	}
	if !bytes.Equal(cur, orig) {
		return fmt.Errorf("%s 在卸载过程中被其他程序修改，为避免覆盖新改动已中止；请重跑卸载", path)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("替换 %s 失败：%w", path, err)
	}
	ok = true
	return nil
}

// uninstallCodexCLI 经 codex 官方 CLI 移除（仅用户级，~/.codex/config.toml）。
// CLI 缺失时的手动指引属于「待手动清理」，返回的 ok=false 且 manual=true。
func uninstallCodexCLI() (desc string, removed, manual bool, err error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return "未找到 codex 命令；请手动编辑 ~/.codex/config.toml 删除 [mcp_servers.serialhub] 段", false, true, nil
	}
	has, listErr := cliHasServer(bin, "mcp")
	if listErr == nil && !has {
		return "~/.codex/config.toml 中无 serialhub 条目，跳过", false, false, nil
	}
	out, err := cliRunWithTimeout(10*time.Second, bin, "mcp", "remove", "serialhub")
	if err != nil {
		return "", false, false, fmt.Errorf("codex mcp remove 失败：%v\n%s", err, out)
	}
	return "~/.codex/config.toml（经 codex mcp remove 移除）", true, false, nil
}

// uninstallClaudeUserCLI 经 claude 官方 CLI 移除用户级条目（~/.claude.json）。
func uninstallClaudeUserCLI() (desc string, removed, manual bool, err error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "未找到 claude 命令；请运行 claude mcp remove serialhub --scope user 或改用项目级卸载", false, true, nil
	}
	has, listErr := cliHasServer(bin, "mcp")
	if listErr == nil && !has {
		return "~/.claude.json 中无 serialhub 条目，跳过", false, false, nil
	}
	out, err := cliRunWithTimeout(10*time.Second, bin, "mcp", "remove", "serialhub", "--scope", "user")
	if err != nil {
		return "", false, false, fmt.Errorf("claude mcp remove 失败：%v\n%s", err, out)
	}
	return "~/.claude.json（经 claude mcp remove --scope user 移除）", true, false, nil
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
