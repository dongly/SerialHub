package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/mcpsetup"
)

var uninstallAssumeYes bool

// newUninstallCmd 构造 `serialhub uninstall` 子命令：清理 MCP 客户端条目、
// 配置与日志目录、二进制本身。默认 dry-run 列清单，确认后执行；--yes 跳过确认。
func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "卸载 SerialHub（清理 MCP 接入条目、配置日志与二进制）",
		Long: "卸载 SerialHub：\n" +
			"  1. 移除各 MCP 客户端中的 serialhub 条目（OpenCode/Claude/Cursor/Windsurf/VS Code/Codex，\n" +
			"     含当前目录的项目级配置；Codex 与 Claude 用户级经官方 CLI 移除，遵循该 CLI 行为）\n" +
			"  2. 删除配置与日志目录（Linux/macOS: ~/.config/serialhub/；Windows: exe 同目录 config.toml 与 logs/）\n" +
			"  3. 删除二进制本身（Windows 下经延迟删除命令）\n" +
			"默认先列出将清理的项（dry-run），确认后执行；全程幂等，不存在的项自动跳过。\n" +
			"运行实例检测覆盖默认端口与用户配置文件的地址端口（含 Windows exe 同目录配置）；\n" +
			"其他自定义端口（-m/-c 临时指定）的实例请自行确认已退出。\n" +
			"前置清理项失败时会跳过二进制删除并返回非零退出码，修复后重跑即可。\n" +
			"注意：-c/--config 指定的自定义路径配置不在清理范围，需手动删除。",
		RunE: runUninstall,
		Args: cobra.NoArgs,
	}
	cmd.Flags().BoolVarP(&uninstallAssumeYes, "yes", "y", false, "跳过确认直接卸载")
	return cmd
}

// uninstallAction 一项卸载动作：desc 描述；probe 报告目标是否存在与展示标签
// （dry-run 展示「配置文件存在」/「待检查」）；run 执行并返回结果描述、是否待
// 手动处理与错误；selfBinary 标记二进制自删项（前置项有硬错误时跳过）。全部动作幂等。
type uninstallAction struct {
	desc       string
	probe      func() (bool, string)
	run        func() (string, bool, error)
	selfBinary bool
}

func runUninstall(cmd *cobra.Command, args []string) error {
	// 0. 检测运行中的实例（默认端口 + 用户配置端口，验证 SerialHub 身份）
	for _, t := range detectCandidateTargets() {
		base := "http://" + net.JoinHostPort(t.host, strconv.Itoa(t.port))
		if serialhubRunningAt(base) {
			return fmt.Errorf("检测到 SerialHub 正在运行（%s/health），请先退出再卸载", base)
		}
	}

	actions := buildUninstallActions()

	// 1. dry-run 清单
	fmt.Println("[SerialHub] 卸载将清理以下内容：")
	some := false
	for i, a := range actions {
		if ok, tag := a.probe(); ok {
			fmt.Printf("  %d. %s [%s]\n", i+1, a.desc, tag)
			some = true
		}
	}
	if !some {
		fmt.Println("  （无可清理项，已是干净状态）")
		return nil
	}

	// 2. 确认
	if !uninstallAssumeYes {
		fmt.Print("确认执行卸载? (y/N): ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		ans := strings.TrimSpace(line)
		if !strings.EqualFold(ans, "y") && !strings.EqualFold(ans, "yes") {
			fmt.Println("已取消卸载。")
			return nil
		}
	}

	// 3. 逐项执行（不存在的项由 run 幂等跳过）；硬错误汇总。
	failures, manualPending := executeUninstallActions(actions)
	if len(failures) > 0 {
		return fmt.Errorf("卸载未完全完成，%d 项失败：\n%s", len(failures), strings.Join(failures, "\n"))
	}
	if manualPending {
		fmt.Println("[SerialHub] 卸载完成（存在待手动处理项，见上方说明）。")
		return nil
	}
	fmt.Println("[SerialHub] 卸载完成。感谢使用！")
	return nil
}

// executeUninstallActions 逐项执行卸载动作：硬错误汇总返回；
// 前置项失败时跳过二进制自删，保留可重跑修复的能力。抽出为独立函数便于测试。
func executeUninstallActions(actions []uninstallAction) (failures []string, manualPending bool) {
	for _, a := range actions {
		if a.selfBinary && len(failures) > 0 {
			fmt.Println("  跳过二进制删除：存在失败项，请修复后重跑卸载")
			continue
		}
		desc, manual, err := a.run()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：%v", a.desc, err))
			fmt.Printf("  失败 %s：%v\n", a.desc, err)
			continue
		}
		fmt.Println("  " + desc)
		if manual {
			manualPending = true
		}
	}
	return failures, manualPending
}

// buildUninstallActions 按顺序组装动作：MCP 条目 → 配置/日志目录 → 二进制。
func buildUninstallActions() []uninstallAction {
	var actions []uninstallAction

	addEntry := func(client, scope string) {
		var desc string
		c, err := mcpsetup.Find(client)
		if err == nil {
			desc = fmt.Sprintf("%s（%s 级）serialhub 条目", c.Name, scope)
		} else {
			desc = client + " serialhub 条目"
		}
		isCLI := false
		if client == "codex" {
			desc = "Codex（用户级，经 codex CLI）serialhub 条目"
			isCLI = true
		} else if client == "claude" && scope == "user" {
			desc = "Claude Code（用户级，经 claude CLI）serialhub 条目"
			isCLI = true
		}
		opts := mcpsetup.UninstallOptions{Client: client, Scope: mcpsetup.Scope(scope)}
		probe := func() (bool, string) {
			if isCLI {
				// CLI 型无法廉价探测，恒列出待执行时再判断
				return true, "待检查"
			}
			if mcpsetup.EntryExists(opts) {
				return true, "配置文件存在"
			}
			return false, ""
		}
		actions = append(actions, uninstallAction{
			desc:  desc,
			probe: probe,
			run: func() (string, bool, error) {
				d, _, manual, err := mcpsetup.UninstallFrom(opts)
				return d, manual, err
			},
		})
	}

	// 用户级：全部 6 客户端；项目级：仅当前目录有配置文件的 4 客户端
	addEntry("opencode", "user")
	addEntry("claude", "user")
	addEntry("cursor", "user")
	addEntry("windsurf", "user")
	addEntry("opencode", "project")
	addEntry("claude", "project")
	addEntry("cursor", "project")
	addEntry("vscode", "project")
	addEntry("codex", "user")

	// 配置与日志目录
	actions = append(actions, uninstallAction{
		desc:  "配置与日志目录（" + userStateDesc() + "）",
		probe: func() (bool, string) { return userStateExists(), "存在" },
		run: func() (string, bool, error) {
			d, err := removeUserState()
			return d, false, err
		},
	})

	// 二进制本身（最后）
	actions = append(actions, uninstallAction{
		desc:  "二进制 " + currentExeDesc(),
		probe: func() (bool, string) { return currentExeDesc() != "", "存在" },
		run: func() (string, bool, error) {
			d, err := removeSelfBinary()
			return d, false, err
		},
		selfBinary: true,
	})
	return actions
}

// candidateTarget 一个待探测的实例地址。
type candidateTarget struct {
	host string
	port int
}

// detectCandidateTargets 返回需要检测运行实例的地址：默认端口 + 用户配置文件的
// 监听地址端口（去重）。Linux/macOS 读 XDG 配置；Windows 读 exe 同目录配置。
func detectCandidateTargets() []candidateTarget {
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	return detectCandidateTargetsImpl(runtime.GOOS == "windows", exeDir, xdgConfigDir(), config.DefaultHTTPPort)
}

// detectCandidateTargetsImpl 是可测内核：isWindows 决定读哪份配置，
// exeDir/xdgBase 注入。按 host+port 组合去重（配置用默认端口但绑定具体地址时
// 仍需探测该地址）；配置的通配监听地址（空/0.0.0.0/::）以 127.0.0.1 与 ::1 探测。
func detectCandidateTargetsImpl(isWindows bool, exeDir, xdgBase string, defaultPort int) []candidateTarget {
	targets := []candidateTarget{}
	addTarget := func(host string, port int) {
		for _, t := range targets {
			if t.host == host && t.port == port {
				return
			}
		}
		targets = append(targets, candidateTarget{host, port})
	}
	for _, h := range effectiveHosts("") {
		addTarget(h, defaultPort)
	}
	var cfgPath string
	if isWindows {
		if exeDir != "" {
			cfgPath = filepath.Join(exeDir, "config.toml")
		}
	} else if xdgBase != "" {
		cfgPath = filepath.Join(xdgBase, "serialhub", "config.toml")
	}
	if cfgPath == "" {
		return targets
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return targets
	}
	port := cfg.MCP.HTTPPort
	if port <= 0 {
		port = defaultPort
	}
	for _, h := range effectiveHosts(cfg.Host) {
		addTarget(h, port)
	}
	return targets
}

// effectiveHosts 把配置的监听地址归一为待探测地址：通配（空/0.0.0.0/::）
// 用两个回环地址探测（IPv6-only 环境经 127.0.0.1 可能不可达，反之亦然），
// 具体地址按配置原样探测。
func effectiveHosts(host string) []string {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::":
		return []string{"127.0.0.1", "::1"}
	default:
		return []string{strings.TrimSpace(host)}
	}
}

// serialhubRunningAt 探测 base/health 是否是正在运行的 SerialHub。
// 识别规则：返回 200 且 JSON 中 service=="serialhub"（新版实例产品标识）；
// 老版本实例无 service 字段，回退 role∈{master,worker} 启发式。
// role 并非 SerialHub 特有字段，回退分支是尽力识别而非保证，
// 其他服务恰好返回相同 role 时仍可能被误判。
func serialhubRunningAt(base string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(base + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return false
	}
	var h struct {
		Service string `json:"service"`
		Role    string `json:"role"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return false
	}
	isRole := h.Role == "master" || h.Role == "worker"
	return (h.Service == "serialhub" && isRole) || (h.Service == "" && isRole)
}

// userStateDesc 描述用户状态目录位置（配置+日志），显示实际路径。
func userStateDesc() string {
	if runtime.GOOS == "windows" {
		return exeDirDesc()
	}
	if base := xdgConfigDir(); base != "" {
		return filepath.Join(base, "serialhub") + "/"
	}
	return "~/.config/serialhub/"
}

func exeDirDesc() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe) + " 下 config.toml 与 logs/"
}

func currentExeDesc() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

func userStateExists() bool {
	if runtime.GOOS == "windows" {
		exe, err := os.Executable()
		if err != nil {
			return false
		}
		dir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(dir, "config.toml")); err == nil {
			return true
		}
		_, err = os.Stat(filepath.Join(dir, "logs"))
		return err == nil
	}
	base := xdgConfigDir()
	if base == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(base, "serialhub"))
	return err == nil
}

// removeUserState 删除配置与日志（Linux/macOS 删整个用户配置目录下的 serialhub/；
// Windows 删 exe 同目录 config.toml 与 logs/，不动其他文件）。
func removeUserState() (string, error) {
	if runtime.GOOS == "windows" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("无法定位二进制目录：%w", err)
		}
		dir := filepath.Dir(exe)
		cfgPath := filepath.Join(dir, "config.toml")
		logsDir := filepath.Join(dir, "logs")
		// 先记录删除前状态，用于准确的结果描述。
		// pathExists 区分「不存在」与「检查失败」（如 ACL 权限错误），
		// 检查失败如实报错，避免把删不掉的目标当作不存在静默跳过。
		cfgEx, err := pathExists(cfgPath)
		if err != nil {
			return "", fmt.Errorf("检查 %s 失败：%w", cfgPath, err)
		}
		logsEx, err := pathExists(logsDir)
		if err != nil {
			return "", fmt.Errorf("检查 %s 失败：%w", logsDir, err)
		}
		if !cfgEx && !logsEx {
			return "无配置与日志文件，跳过", nil
		}
		if cfgEx {
			if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("删除 %s 失败：%w", cfgPath, err)
			}
		}
		if logsEx {
			if err := os.RemoveAll(logsDir); err != nil {
				return "", fmt.Errorf("删除 %s 失败：%w", logsDir, err)
			}
		}
		return "已删除 " + dir + " 下 config.toml 与 logs/", nil
	}
	base := xdgConfigDir()
	if base == "" {
		return "无法定位用户配置目录，跳过", nil
	}
	dir := filepath.Join(base, "serialhub")
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return dir + " 不存在，跳过", nil
		}
		return "", fmt.Errorf("检查 %s 失败：%w", dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("删除 %s 失败：%w", dir, err)
	}
	return "已删除 " + dir + "/（配置与日志）", nil
}

// pathExists 报告路径是否存在，并区分「不存在」与「检查失败」：
// 仅 os.IsNotExist 视为不存在返回 (false, nil)，其他 Stat 错误（如权限）
// 如实返回错误——布尔存在性检查会把这些错误吞掉当作不存在。
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// removeSelfBinary 删除自身二进制。Linux/macOS 直接删除；
// Windows 下运行中的 exe 无法立即删除，先试直接删，失败则丢延迟删除命令
// （用 ping 计时代替 timeout：timeout /t 在无交互 stdin 的子进程中会报错），
// 延迟删除也失败时打印路径请用户手动删。
func removeSelfBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位自身：%w", err)
	}
	if rmErr := os.Remove(exe); rmErr == nil {
		return "已删除二进制 " + exe, nil
	} else if !os.IsNotExist(rmErr) && runtime.GOOS != "windows" {
		return "", fmt.Errorf("删除 %s 失败：%w（请手动删除）", exe, rmErr)
	}
	if runtime.GOOS == "windows" {
		del := exec.Command("cmd", "/c", fmt.Sprintf("ping -n 3 127.0.0.1 >nul & del /f %q", exe))
		if err := del.Start(); err == nil {
			return "已安排延迟删除二进制 " + exe + "（进程退出后生效）", nil
		}
	}
	return "", fmt.Errorf("二进制删除失败（请手动删除 %s）", exe)
}
