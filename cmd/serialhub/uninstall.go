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
	"unicode/utf16"

	"github.com/spf13/cobra"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/internal/instance"
	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/mcpsetup"
)

var uninstallAssumeYes bool

// newUninstallCmd 构造 `serialhub uninstall` 子命令：清理 MCP 客户端条目、
// 配置与日志目录、二进制本身。默认 dry-run 列清单，确认后执行；--yes 跳过确认。
func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: i18n.Uninstall.Short,
		Long:  i18n.Uninstall.Long,
		RunE:  runUninstall,
		Args:  cobra.NoArgs,
	}
	cmd.Flags().BoolVarP(&uninstallAssumeYes, "yes", "y", false, i18n.Uninstall.Yes)
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
	// 版本行模板集中在 i18n（产品名与版本号本身无需翻译）
	fmt.Printf(i18n.Uninstall.VersionLine+"\n", appVersion)

	// 0. 检测运行中的实例（默认端口 + 用户配置端口，验证 SerialHub 身份）。
	// 对侧实例（如 WSL2 localhost 端口转发命中的另一系统实例）不占用
	// 本机文件与端口，仅提示、不拦截卸载。
	for _, t := range detectCandidateTargets() {
		base := "http://" + net.JoinHostPort(t.host, strconv.Itoa(t.port))
		local, remote := serialhubInstanceAt(base)
		if local {
			return fmt.Errorf(i18n.Uninstall.Running, base)
		}
		if remote {
			fmt.Println(fmt.Sprintf(i18n.Uninstall.RemoteInstance, base))
		}
	}

	actions := buildUninstallActions()

	// 1. dry-run 清单
	fmt.Println(i18n.Uninstall.Plan)
	some := false
	for i, a := range actions {
		if ok, tag := a.probe(); ok {
			fmt.Printf("  %d. %s [%s]\n", i+1, a.desc, tag)
			some = true
		}
	}
	if !some {
		fmt.Println(i18n.Uninstall.Empty)
		return nil
	}

	// 2. 确认
	if !uninstallAssumeYes {
		fmt.Print(i18n.Uninstall.Confirm)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		ans := strings.TrimSpace(line)
		if !strings.EqualFold(ans, "y") && !strings.EqualFold(ans, "yes") {
			fmt.Println(i18n.Uninstall.Cancel)
			return nil
		}
	}

	// 3. 逐项执行（不存在的项由 run 幂等跳过）；硬错误汇总。
	failures, manualPending := executeUninstallActions(actions)
	if len(failures) > 0 {
		return fmt.Errorf(i18n.Uninstall.Failed, len(failures), strings.Join(failures, "\n"))
	}
	if manualPending {
		fmt.Println(i18n.Uninstall.ManualDone)
		return nil
	}
	fmt.Println(i18n.Uninstall.Done)
	return nil
}

// executeUninstallActions 逐项执行卸载动作：硬错误汇总返回；
// 前置项失败时跳过二进制自删，保留可重跑修复的能力。抽出为独立函数便于测试。
func executeUninstallActions(actions []uninstallAction) (failures []string, manualPending bool) {
	for _, a := range actions {
		if a.selfBinary && len(failures) > 0 {
			fmt.Println(i18n.Uninstall.SkipBinary)
			continue
		}
		desc, manual, err := a.run()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s：%v", a.desc, err))
			fmt.Printf(i18n.Uninstall.ActionFailed, a.desc, err)
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
			desc = fmt.Sprintf(i18n.Uninstall.Entry, c.Name, scope)
		} else {
			desc = fmt.Sprintf(i18n.Uninstall.Entry, client, scope)
		}
		isCLI := false
		if client == "codex" {
			desc = i18n.Uninstall.CLIEntryCodex
			isCLI = true
		} else if client == "claude" && scope == "user" {
			desc = i18n.Uninstall.CLIEntryClaude
			isCLI = true
		}
		opts := mcpsetup.UninstallOptions{Client: client, Scope: mcpsetup.Scope(scope)}
		probe := func() (bool, string) {
			if isCLI {
				// CLI 型无法廉价探测，恒列出待执行时再判断
				return true, i18n.Uninstall.Pending
			}
			if mcpsetup.EntryExists(opts) {
				return true, i18n.Uninstall.FileExists
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
		desc:  fmt.Sprintf(i18n.Uninstall.State, userStateDesc()),
		probe: func() (bool, string) { return userStateExists(), i18n.Uninstall.Exists },
		run: func() (string, bool, error) {
			d, err := removeUserState()
			return d, false, err
		},
	})

	// 安装目录随包文件（发布包解压部署：启动脚本/文档/升级残留）
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	if hasInstallLayout(installBundledFiles(exeDir)) {
		actions = append(actions, uninstallAction{
			desc:  fmt.Sprintf(i18n.Uninstall.InstallFiles, exeDir),
			probe: func() (bool, string) { return len(installBundledFiles(exeDir)) > 0, i18n.Uninstall.Exists },
			run: func() (string, bool, error) {
				return removeInstallDirExtrasIn(exeDir)
			},
		})
	}

	// 二进制本身（最后）；exe 删除后兜底删除可能已空的安装目录
	actions = append(actions, uninstallAction{
		desc:  fmt.Sprintf(i18n.Uninstall.Binary, currentExeDesc()),
		probe: func() (bool, string) { return currentExeDesc() != "", i18n.Uninstall.Exists },
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

// serialhubInstanceAt 探测 base/health 是否是正在运行的 SerialHub。
// 识别规则：返回 200 且 JSON 中 service=="serialhub"（新版实例产品标识）；
// 老版本实例无 service 字段，回退 role∈{master,worker} 启发式。
// role 并非 SerialHub 特有字段，回退分支是尽力识别而非保证，
// 其他服务恰好返回相同 role 时仍可能被误判。
func serialhubInstanceAt(base string) (local, remote bool) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(base + "/health")
	if err != nil {
		return false, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return false, false
	}
	var h struct {
		Service string `json:"service"`
		Role    string `json:"role"`
		Side    string `json:"side"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return false, false
	}
	isRole := h.Role == "master" || h.Role == "worker"
	if !((h.Service == "serialhub" && isRole) || (h.Service == "" && isRole)) {
		return false, false
	}
	// side 与本机不一致＝对侧实例经 WSL2 localhost 转发可达，
	// 不是本机进程：不拦截卸载，仅提示。老版本无 side 字段按本机处理。
	if h.Side != "" && h.Side != instance.LocalSide() {
		return false, true
	}
	return true, false
}

// userStateDesc 描述用户状态目录位置（配置+日志+Windows 锁目录），显示实际路径。
func userStateDesc() string {
	if runtime.GOOS == "windows" {
		desc := exeDirDesc()
		if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
			desc += fmt.Sprintf(i18n.Uninstall.LockDir, filepath.Join(cacheDir, "serialhub")+`\`)
		}
		return desc
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
	return fmt.Sprintf(i18n.Uninstall.ConfigAndLogs, filepath.Dir(exe))
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
// Windows 删 exe 同目录 config.toml 与 logs/，以及固定位置的锁目录
// %LOCALAPPDATA%\serialhub\，不动其他文件）。
func removeUserState() (string, error) {
	if runtime.GOOS == "windows" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf(i18n.Uninstall.LocateDir, err)
		}
		dir := filepath.Dir(exe)
		cfgPath := filepath.Join(dir, "config.toml")
		logsDir := filepath.Join(dir, "logs")
		// 先记录删除前状态，用于准确的结果描述。
		// pathExists 区分「不存在」与「检查失败」（如 ACL 权限错误），
		// 检查失败如实报错，避免把删不掉的目标当作不存在静默跳过。
		cfgEx, err := pathExists(cfgPath)
		if err != nil {
			return "", fmt.Errorf(i18n.Uninstall.CheckFailed, cfgPath, err)
		}
		logsEx, err := pathExists(logsDir)
		if err != nil {
			return "", fmt.Errorf(i18n.Uninstall.CheckFailed, logsDir, err)
		}
		if cfgEx {
			if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf(i18n.Uninstall.RemoveFailed, cfgPath, err)
			}
		}
		if logsEx {
			if err := os.RemoveAll(logsDir); err != nil {
				return "", fmt.Errorf(i18n.Uninstall.RemoveFailed, logsDir, err)
			}
		}
		desc := i18n.Uninstall.NoFiles
		if cfgEx || logsEx {
			desc = fmt.Sprintf(i18n.Uninstall.RemovedFiles, dir)
		}
		// 锁目录固定在用户本地数据目录（与 exe 位置无关），一并清理
		if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
			lockDir := filepath.Join(cacheDir, "serialhub")
			if ex, err := pathExists(lockDir); err == nil && ex {
				if err := os.RemoveAll(lockDir); err != nil {
					return desc, fmt.Errorf(i18n.Uninstall.RemoveFailed, lockDir, err)
				}
				desc += fmt.Sprintf(i18n.Uninstall.LockDir, lockDir)
			}
		}
		return desc, nil
	}
	base := xdgConfigDir()
	if base == "" {
		return i18n.Uninstall.NoConfigDir, nil
	}
	dir := filepath.Join(base, "serialhub")
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf(i18n.Uninstall.NotFound, dir), nil
		}
		return "", fmt.Errorf(i18n.Uninstall.CheckFailed, dir, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf(i18n.Uninstall.RemoveFailed, dir, err)
	}
	return fmt.Sprintf(i18n.Uninstall.RemovedState, dir), nil
}

// installBundledNames 是发布包随附、卸载时应一并清理的文件名；
// upgradeOldGlobs 匹配自升级留下的旧二进制残留（.old-<ns> 及历史命名）。
var installBundledNames = []string{
	"serialhub.ps1", "serialhub.bat", "serialhub.sh",
	"VERSION", "README.md", "README.en.md", "QUICKSTART.md", "MCP.md", "LICENSE",
}

var upgradeOldGlobs = []string{"serialhub.exe.old-*", "serialhub.old-*", "serialhub.exe.*-old"}

// isLaunchScriptName 判断单个文件名是否启动脚本。
func isLaunchScriptName(base string) bool {
	switch base {
	case "serialhub.ps1", "serialhub.bat", "serialhub.sh":
		return true
	}
	return false
}

// hasLaunchScript 判断文件列表是否含启动脚本。
func hasLaunchScript(files []string) bool {
	for _, f := range files {
		base := filepath.Base(f)
		if base == "serialhub.ps1" || base == "serialhub.bat" || base == "serialhub.sh" {
			return true
		}
	}
	return false
}

// hasInstallLayout 判断 exe 同目录是否发布包解压出的安装目录：
// 启动脚本或自升级残留任一在场即认定。两者都是 serialhub 特征文件，
// 不会误伤随手放置 exe 的普通目录；升级残留也参与认定，保证首跑删掉
// 脚本但部分失败后重跑仍能续清（残留 .old 文件还在，目录仍被认定）。
func hasInstallLayout(files []string) bool {
	if hasLaunchScript(files) {
		return true
	}
	for _, f := range files {
		base := filepath.Base(f)
		for _, g := range upgradeOldGlobs {
			if ok, _ := filepath.Match(g, base); ok {
				return true
			}
		}
	}
	return false
}

// installBundledFiles 返回 dir 下实际存在的随包文件与升级残留路径。
func installBundledFiles(dir string) []string {
	var found []string
	for _, name := range installBundledNames {
		p := filepath.Join(dir, name)
		if ok, _ := pathExists(p); ok {
			found = append(found, p)
		}
	}
	for _, g := range upgradeOldGlobs {
		if m, _ := filepath.Glob(filepath.Join(dir, g)); m != nil {
			found = append(found, m...)
		}
	}
	return found
}

// removeInstallDirExtrasIn 删除安装目录（发布包解压部署）中的随包文件：
// 启动脚本、文档与升级残留。用户自建文件不受影响；目录本身的删除由
// removeSelfBinary 兜底（须等 exe 删除后目录才可能为空）。
func removeInstallDirExtrasIn(dir string) (string, bool, error) {
	files := installBundledFiles(dir)
	if !hasInstallLayout(files) {
		// 无启动脚本也无升级残留说明不是发布包安装目录，跳过（probe 已挡，防御）
		return fmt.Sprintf(i18n.Uninstall.NotFound, dir), false, nil
	}
	// 保序删除：普通随包文档 → 升级残留 → 启动脚本。启动脚本与残留
	// 是安装目录的认定特征，放在最后且遇错即中止——任何部分失败的
	// 中间态都保有特征文件，重跑仍能认定为安装目录并续清剩余文件。
	isResidue := func(base string) bool {
		for _, g := range upgradeOldGlobs {
			if ok, _ := filepath.Match(g, base); ok {
				return true
			}
		}
		return false
	}
	var docs, residues, scripts []string
	for _, f := range files {
		switch base := filepath.Base(f); {
		case isLaunchScriptName(base):
			scripts = append(scripts, f)
		case isResidue(base):
			residues = append(residues, f)
		default:
			docs = append(docs, f)
		}
	}
	n := 0
	for _, f := range append(append(docs, residues...), scripts...) {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			// 中止而非跳过：保住后续（特征）文件，留给重跑续清
			return "", false, fmt.Errorf("%s: %v", filepath.Base(f), err)
		}
		n++
	}
	return fmt.Sprintf(i18n.Uninstall.RemovedInstallFiles, n), false, nil
}

// removeInstallDirExtras 是 removeInstallDirExtrasIn 的生产包装（exe 同目录）。
func removeInstallDirExtras() (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, fmt.Errorf(i18n.Uninstall.LocateSelf, err)
	}
	return removeInstallDirExtrasIn(filepath.Dir(exe))
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
// （用 ping 计时代替 timeout：timeout /t 在无交互 stdin 的子进程中会报错；
// 多次重试、间隔递增，见 windowsDelayedRemoveScript），
// 延迟删除也失败时打印路径请用户手动删。
func removeSelfBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf(i18n.Uninstall.LocateSelf, err)
	}
	// exe 删除后尝试移除此刻可能已空的安装目录：非空目录 os.Remove
	// 自然失败、无副作用，故无需预先认定安装目录——认定的意义在防
	// 误删随包白名单文件，删空目录没有该风险。
	tryRmdir := func(note string) string {
		if dir := filepath.Dir(exe); os.Remove(dir) == nil {
			return note + "；" + fmt.Sprintf(i18n.Uninstall.RemovedEmptyDir, dir)
		}
		return note
	}
	if rmErr := os.Remove(exe); rmErr == nil {
		return tryRmdir(fmt.Sprintf(i18n.Uninstall.RemovedBinary, exe)), nil
	} else if !os.IsNotExist(rmErr) && runtime.GOOS != "windows" {
		return "", fmt.Errorf(i18n.Uninstall.ManualRemove, exe, rmErr)
	}
	if runtime.GOOS == "windows" {
		script := windowsDelayedRemoveScript(exe, filepath.Dir(exe))
		del := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		// 延迟进程工作目录须避开安装目录：在安装目录内执行卸载时，
		// 进程自身占用目录会让目录删除失败（此时 exe 已删，无法重跑补救）
		if dir := os.Getenv("SystemRoot"); dir != "" {
			del.Dir = dir
		} else {
			del.Dir = os.Getenv("SystemDrive") + "\\"
		}
		if err := del.Start(); err == nil {
			return fmt.Sprintf(i18n.Uninstall.DelayedBinary, exe), nil
		}
	}
	return "", fmt.Errorf(i18n.Uninstall.BinaryFailed, exe)
}

// windowsDelayedRemoveScript 构造 Windows 延迟自删的 PowerShell 脚本：
// 等待 uninstall 进程退出（约 2s）后，以「文件是否在场」判定成败重试删除
// exe（固定间隔 3s，最多 3 次），最后回收空的安装目录（非空不动）。
// 路径不进入脚本文本，而是以 UTF-16 码元列表（psCharArray）嵌入、
// 脚本内 [char[]] 重建——单引号字面量无法可靠承载路径：除 ASCII '
// 需转义外，PowerShell 还把 U+2018～U+201B 弯引号视为引号，路径
// 含这些字符会破坏（甚至注入）脚本。码元数组对空格/中文/通配符/
// 引号全部免疫。经 exec 传给 powershell.exe（CRT 参数还原规则，
// 无 cmd /c 的引号剥离歧义——此前 cmd 方案因 Go argv 序列化与
// cmd 解析规则不一致导致 del 从未执行）。
// powershellRetryRemove 产出「延迟删除单个文件」的 PowerShell 重试段：
// $<name> 由码元数组重建为路径，先等 2s 让调用方退出，随后最多 3 次
// 尝试（间隔 3s），以文件是否在场判定成败（不依赖 cmdlet 退出码——
// del/Remove-Item 对被锁文件可能报错但退出码为成功）。uninstall 自删
// 与 upgrade 清 .old 共用此段，调整重试策略只改这里。
func powershellRetryRemove(name, path string) string {
	return "$" + name + " = -join [char[]](" + psCharArray(path) + "); " +
		"Start-Sleep -Seconds 2; " +
		"foreach ($i in 1..3) { " +
		"if (Test-Path -LiteralPath $" + name + ") { " +
		"Remove-Item -Force -LiteralPath $" + name + "; " +
		"Start-Sleep -Seconds 3 } " +
		"}"
}

func windowsDelayedRemoveScript(exe, dir string) string {
	return powershellRetryRemove("e", exe) + "; " +
		"$d = -join [char[]](" + psCharArray(dir) + "); " +
		"if ((Test-Path -LiteralPath $d) -and -not (Get-ChildItem -Force -LiteralPath $d)) { " +
		"[void][System.IO.Directory]::Delete($d) }"
}

// psCharArray 把 s 编码为逗号分隔的 UTF-16 码元列表，供 PowerShell
// `-join [char[]](...)` 重建字符串：脚本内不出现任何路径字面量，
// 从根本上消除引号（ASCII/弯引号）、通配符与变量展开的注入面；
// 代理对字符（>U+FFFF）拆为两个码元，与 PS 的 UTF-16 字符串一致。
func psCharArray(s string) string {
	units := utf16.Encode([]rune(s))
	nums := make([]string, len(units))
	for i, u := range units {
		nums[i] = strconv.FormatUint(uint64(u), 10)
	}
	return strings.Join(nums, ",")
}
