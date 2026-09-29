package i18n

// 本文件集中定义用户可见文案（CLI、托盘、串口错误）和 Web 事件标识。
// 按使用方分组为结构体，调用点引用具名字段（如 i18n.Serial.NotConnected），
// 避免双语文案散落各处。logrus 日志（进文件排障）不在此列，保持中文。

// Serial 是 pkg/serial 的用户可见消息（错误与事件文本）。
var Serial = struct {
	ConfigNil            string // 配置不能为空（Connect/UpdateConfig）
	PortEmpty            string // 端口不能为空
	PortStringEmpty      string // 端口字符串不能为空（ParsePort）
	NotConnected         string // 串口未连接
	AlreadyConnected     string // 串口已连接: %s
	ManagerClosed        string // 串口管理器已关闭，无法连接
	ConvertConfigFailed  string // 配置转换失败: %w
	OpenFailed           string // 打开串口失败: %w
	WriteError           string // 写入错误: %w
	WriteFailed          string // 写入失败: %w
	ListFailed           string // 获取串口列表失败: %w
	UpdateWhileConnected string // 串口已连接，无法更新配置
	ReconnectCancelled   string // 重连已取消（哨兵错误）
	InvalidBaudRate      string // 无效的波特率: %d
	InvalidBaudRateStr   string // 无效的波特率: %s（字符串解析失败）
	InvalidDataBits      string // 无效的数据位: %d（支持 5/6/7/8）
	InvalidParity        string // 无效的校验位: %s（支持 none/even/odd）
	InvalidStopBits      string // 无效的停止位: %v（支持 1/1.5/2）
	EventConnClosedEOF   string // 连接已关闭 (EOF)
	EventReadError       string // 读取错误: %v
	EventPortError       string // 串口 %s %s（errChan 诊断 fmt 模板）
	EventWillReconnect   string // ，将自动重连（拼接后缀）
	EventReconnectFailed string // 自动重连失败：端口未恢复，请手动重连
}{
	ConfigNil:            T("配置不能为空", "config must not be nil"),
	PortEmpty:            T("端口不能为空", "port must not be empty"),
	PortStringEmpty:      T("端口字符串不能为空", "port string must not be empty"),
	NotConnected:         T("串口未连接", "serial port not connected"),
	AlreadyConnected:     T("串口已连接: %s", "serial port already connected: %s"),
	ManagerClosed:        T("串口管理器已关闭，无法连接", "serial manager is closed, cannot connect"),
	ConvertConfigFailed:  T("配置转换失败: %w", "failed to convert config: %w"),
	OpenFailed:           T("打开串口失败: %w", "failed to open serial port: %w"),
	WriteError:           T("写入错误: %w", "write error: %w"),
	WriteFailed:          T("写入失败: %w", "write failed: %w"),
	ListFailed:           T("获取串口列表失败: %w", "failed to list serial ports: %w"),
	UpdateWhileConnected: T("串口已连接，无法更新配置", "cannot update config while connected"),
	ReconnectCancelled:   T("重连已取消", "reconnect cancelled"),
	InvalidBaudRate:      T("无效的波特率: %d", "invalid baud rate: %d"),
	InvalidBaudRateStr:   T("无效的波特率: %s", "invalid baud rate: %s"),
	InvalidDataBits:      T("无效的数据位: %d（支持 5/6/7/8）", "invalid data bits: %d (supported: 5/6/7/8)"),
	InvalidParity:        T("无效的校验位: %s（支持 none/even/odd）", "invalid parity: %s (supported: none/even/odd)"),
	InvalidStopBits:      T("无效的停止位: %v（支持 1/1.5/2）", "invalid stop bits: %v (supported: 1/1.5/2)"),
	EventConnClosedEOF:   T("连接已关闭 (EOF)", "connection closed (EOF)"),
	EventReadError:       T("读取错误: %v", "read error: %v"),
	EventPortError:       T("串口 %s %s", "serial port %s %s"),
	EventWillReconnect:   T("，将自动重连", ", will auto-reconnect"),
	EventReconnectFailed: T("自动重连失败：端口未恢复，请手动重连", "auto-reconnect failed: port not recovered, please reconnect manually"),
}

// Tray 是 Windows 系统托盘的菜单 UI 文本（pkg/tray）。
var Tray = struct {
	MenuSerialTip                                                  string // 串口连接（连接/断开菜单项 tooltip）
	MenuSelectPort                                                 string // 选择串口 ▶
	MenuSelectPortTip                                              string // 选择串口
	MenuRefresh                                                    string // 刷新列表
	MenuRefreshTip                                                 string // 刷新串口列表
	MenuSerialConfig                                               string // 串口设置 ▶
	MenuSerialConfigTip                                            string // 串口参数配置
	BaudRateMenu                                                   string // 波特率: %d ▶
	BaudRateItem                                                   string // 波特率 %d
	DataBitsMenu                                                   string // 数据位: %d ▶
	DataBitsItem                                                   string // 数据位 %d
	StopBitsMenu                                                   string // 停止位: %.0f ▶
	StopBitsItem                                                   string // 停止位 %s
	ParityMenu                                                     string // 校验位: %s ▶
	ParityItem                                                     string // 校验位 %s
	ConfigSummary                                                  string // 当前: %s %d %d%s%s
	PortNotSelected                                                string // 未选择
	MenuOpenTerminal                                               string // 打开终端 🌐
	MenuOpenTerminalTip                                            string // 在浏览器中打开 Web 终端
	MenuShowWindow                                                 string // 显示窗口
	MenuShowWindowTip                                              string // 显示控制台窗口
	MenuHideWindow                                                 string // 隐藏窗口
	MenuVersion                                                    string // 版本 %s
	MenuVersionTip                                                 string // 版本
	MenuQuit                                                       string // ❌ 退出
	MenuQuitTip                                                    string // 退出 SerialHub
	TitleConnected                                                 string // 已连接 %s @ %d
	TitleConnect                                                   string // 连接 %s
	TooltipIdle                                                    string // SerialHub - 未连接
	TooltipConnected                                               string // SerialHub - 已连接 %s
	SelectPortItemTip                                              string // 选择串口 %s
	SeparatorTip, BaudRateTip, DataBitsTip, StopBitsTip, ParityTip string
	CurrentConfigTip, NetworkStatusTip                             string
}{
	MenuSerialTip:       T("串口连接", "Serial connection"),
	MenuSelectPort:      T("选择串口 ▶", "Select Port ▶"),
	MenuSelectPortTip:   T("选择串口", "Select serial port"),
	MenuRefresh:         T("刷新列表", "Refresh List"),
	MenuRefreshTip:      T("刷新串口列表", "Refresh serial port list"),
	MenuSerialConfig:    T("串口设置 ▶", "Serial Settings ▶"),
	MenuSerialConfigTip: T("串口参数配置", "Serial port settings"),
	BaudRateMenu:        T("波特率: %d ▶", "Baud Rate: %d ▶"),
	BaudRateItem:        T("波特率 %d", "Baud rate %d"),
	DataBitsMenu:        T("数据位: %d ▶", "Data Bits: %d ▶"),
	DataBitsItem:        T("数据位 %d", "Data bits %d"),
	StopBitsMenu:        T("停止位: %.0f ▶", "Stop Bits: %.0f ▶"),
	StopBitsItem:        T("停止位 %s", "Stop bits %s"),
	ParityMenu:          T("校验位: %s ▶", "Parity: %s ▶"),
	ParityItem:          T("校验位 %s", "Parity %s"),
	ConfigSummary:       T("当前: %s %d %d%s%s", "Current: %s %d %d%s%s"),
	PortNotSelected:     T("未选择", "Not selected"),
	MenuOpenTerminal:    T("打开终端 🌐", "Open Terminal 🌐"),
	MenuOpenTerminalTip: T("在浏览器中打开 Web 终端", "Open Web terminal in browser"),
	MenuShowWindow:      T("显示窗口", "Show Window"),
	MenuShowWindowTip:   T("显示控制台窗口", "Show console window"),
	MenuHideWindow:      T("隐藏窗口", "Hide Window"),
	MenuVersion:         T("版本 %s", "Version %s"),
	MenuVersionTip:      T("版本", "Version"),
	MenuQuit:            T("❌ 退出", "❌ Quit"),
	MenuQuitTip:         T("退出 SerialHub", "Quit SerialHub"),
	TitleConnected:      T("已连接 %s @ %d", "Connected %s @ %d"),
	TitleConnect:        T("连接 %s", "Connect %s"),
	TooltipIdle:         T("SerialHub - 未连接", "SerialHub - Disconnected"),
	TooltipConnected:    T("SerialHub - 已连接 %s", "SerialHub - Connected %s"),
	SelectPortItemTip:   T("选择串口 %s", "Select port %s"),
	SeparatorTip:        T("分隔线", "Separator"),
	BaudRateTip:         T("选择波特率", "Choose baud rate"),
	DataBitsTip:         T("选择数据位", "Choose data bits"),
	StopBitsTip:         T("选择停止位", "Choose stop bits"),
	ParityTip:           T("选择校验位", "Choose parity"),
	CurrentConfigTip:    T("当前配置", "Current configuration"),
	NetworkStatusTip:    T("网络状态", "Network status"),
}

// CLI 是根命令及 setup、upgrade 的帮助和交互文本。
var CLI = struct {
	RootShort, RootLong                                                             string
	FlagSerialPort, FlagBaudRate, FlagConfig, FlagDebug, FlagLogData                string
	FlagMCPPort, FlagHost, FlagMinimized, FlagNoBrowser, FlagStdio                  string
	SetupShort, SetupLong                                                           string
	SetupClient, SetupURL, SetupMode, SetupScope, SetupYes                          string
	ChooseClient, ChooseNumber, InvalidNumber, ChooseMode, InvalidChoice            string
	InvalidMode, InvalidScope, ConfirmOverwrite, EntrySkipped, SetupDone, SetupHint string
	UpgradeShort, UpgradeLong, LocateExe, CheckVersion, FetchFailed, AlreadyLatest  string
	FoundVersion, DownloadFailed, DownloadChecksumFailed, VerifyFailed, ChecksumOK  string
	ExtractFailed, ReplaceFailed, UpgradeDone                                       string
}{
	RootShort:              T("SerialHub - 串口与网络连接的双向桥接器", "SerialHub - a bidirectional serial and network bridge"),
	RootLong:               T("SerialHub 将 MCU 串口数据同时转发到 WebSocket（人工监视）和 MCP（AI 工具程序化访问）。", "SerialHub forwards MCU serial data to WebSocket (human monitoring) and MCP (AI tools)."),
	FlagSerialPort:         T("串口名（如 COM9 或 /dev/ttyUSB0）", "Serial port (e.g. COM9 or /dev/ttyUSB0)"),
	FlagBaudRate:           T("波特率", "Baud rate"),
	FlagConfig:             T("配置文件路径", "Configuration file path"),
	FlagDebug:              T("启用调试模式", "Enable debug mode"),
	FlagLogData:            T("输出数据内容日志（500ms 时间窗聚合、单条截断 512 字节；可用 SERIALHUB_LOG_DATA=1，--log-data=false 显式关闭）", "Log data contents (500ms aggregation, 512-byte display limit; SERIALHUB_LOG_DATA=1 also enables this, --log-data=false overrides it)"),
	FlagMCPPort:            T("MCP HTTP 服务端口", "MCP HTTP server port"),
	FlagHost:               T("监听地址", "Listen address"),
	FlagMinimized:          T("由脚本启动，窗口最小化", "Start minimized from a script"),
	FlagNoBrowser:          T("跳过自动打开浏览器", "Do not open the browser automatically"),
	FlagStdio:              T("以 stdio 模式运行（MCP 客户端本地拉起）", "Run in stdio mode (launched locally by an MCP client)"),
	SetupShort:             T("为 MCP 客户端自动配置 SerialHub 接入", "Configure SerialHub access for an MCP client"),
	SetupLong:              T("交互式向导：选择 MCP 客户端（OpenCode / Claude Code / Cursor / Windsurf / VS Code / Codex）→ 接入模式（stdio 或 HTTP，默认 stdio 本地模式，客户端自动拉起）→ 写入层级（项目级/用户级），\n然后合并写入该客户端的配置文件（不动其他服务条目；已有 serialhub 条目时交互模式会确认，-y 直接更新，可借此切换接入模式；\nCodex 与 Claude 用户级经官方 CLI 写入，已有条目的处理遵循该 CLI 行为）。\n非交互用法：serialhub setup --client cursor -y（如需 HTTP：serialhub setup --client cursor --mode http -y）", "Interactive setup: choose an MCP client, stdio (default) or HTTP transport, and project or user scope.\nOther services are preserved. Existing serialhub entries require confirmation; -y updates them without asking.\nCodex and Claude user-level entries use their official CLI and follow its behavior.\nNon-interactive: serialhub setup --client cursor -y (HTTP: add --mode http)."),
	SetupClient:            T("客户端 ID: opencode/claude/cursor/windsurf/vscode/codex", "Client ID: opencode/claude/cursor/windsurf/vscode/codex"),
	SetupURL:               T("HTTP 端点（仅 --mode http 时生效）", "HTTP endpoint (only for --mode http)"),
	SetupMode:              T("接入模式: stdio | http（默认 stdio 本地模式）", "Transport: stdio | http (default: local stdio)"),
	SetupScope:             T("写入层级: project | user（codex 仅 user）", "Scope: project | user (codex supports user only)"),
	SetupYes:               T("非交互：确认全部默认选择", "Non-interactive: accept all defaults"),
	ChooseClient:           T("选择 MCP 客户端:", "Choose an MCP client:"),
	ChooseNumber:           T("请输入编号 [1]: ", "Enter number [1]: "),
	InvalidNumber:          T("无效编号: %s", "Invalid number: %s"),
	ChooseMode:             T("接入模式: 1) stdio（推荐，客户端自动拉起） 2) HTTP [1]: ", "Transport: 1) stdio (recommended, launched by client) 2) HTTP [1]: "),
	InvalidChoice:          T("无效选项 %q（可选 1/2）", "Invalid choice %q (choose 1 or 2)"),
	InvalidMode:            T("无效模式 %q（可选 http/stdio）", "Invalid mode %q (choose http or stdio)"),
	InvalidScope:           T("无效层级 %q（可选 project/user）", "Invalid scope %q (choose project or user)"),
	ConfirmOverwrite:       T("%s 中已有 serialhub 条目，覆盖更新? (y/N): ", "serialhub entry already exists in %s. Overwrite? (y/N): "),
	EntrySkipped:           T("已存在 serialhub 条目，按选择跳过，未做修改。", "Existing serialhub entry skipped; no changes made."),
	SetupDone:              T("[SerialHub] 已为 %s 写入接入配置（%s，%s 级）\n  目标: %s\n", "[SerialHub] Configured %s (%s, %s scope)\n  Target: %s\n"),
	SetupHint:              T("  提示: 项目级配置文件可提交到版本库与团队共享。", "  Tip: project-level configuration can be committed and shared with your team."),
	UpgradeShort:           T("从 GitHub Releases 升级到最新版本", "Upgrade to the latest GitHub Release"),
	UpgradeLong:            T("从 GitHub Releases 检查并升级 SerialHub：\n  1. 查询最新 release，与当前版本比较\n  2. 下载对应平台压缩包（serialhub-<版本>-<os>-<arch>.tar.gz / .zip）并校验 sha256\n  3. 解出二进制，经同目录临时文件原子替换自身（配置与日志保留不动）\n网络代理遵从 HTTPS_PROXY/HTTP_PROXY 环境变量；私有 GitHub 加速可设 SERIALHUB_GITHUB_API。\n自定义 -c/--config 的配置文件不受影响。", "Check and upgrade from GitHub Releases:\n  1. Compare the latest release with the current version\n  2. Download the platform archive and verify its sha256 checksum\n  3. Extract and replace the executable, preserving configuration and logs\nHTTPS_PROXY/HTTP_PROXY are supported; SERIALHUB_GITHUB_API overrides the API base URL.\nCustom -c/--config files are unaffected."),
	LocateExe:              T("无法定位当前二进制：%w", "cannot locate current executable: %w"),
	CheckVersion:           T("[SerialHub] 当前版本 %s，正在检查更新...\n", "[SerialHub] Current version %s; checking for updates...\n"),
	FetchFailed:            T("查询最新版本失败：%w", "failed to check latest release: %w"),
	AlreadyLatest:          T("[SerialHub] 已是最新版本 %s（latest release: %s）。\n", "[SerialHub] Already up to date: %s (latest release: %s).\n"),
	FoundVersion:           T("[SerialHub] 发现新版本 %s，下载 %s ...\n", "[SerialHub] New version %s; downloading %s...\n"),
	DownloadFailed:         T("下载 %s 失败：%w", "failed to download %s: %w"),
	DownloadChecksumFailed: T("下载校验文件失败：%w", "failed to download checksum file: %w"),
	VerifyFailed:           T("校验失败：%w", "checksum verification failed: %w"),
	ChecksumOK:             T("[SerialHub] sha256 校验通过。", "[SerialHub] sha256 checksum verified."),
	ExtractFailed:          T("解压 %s 失败：%w", "failed to extract %s: %w"),
	ReplaceFailed:          T("替换二进制失败：%w", "failed to replace executable: %w"),
	UpgradeDone:            T("[SerialHub] 已升级 %s → %s（配置与日志保留不动）。\n", "[SerialHub] Upgraded %s → %s (configuration and logs preserved).\n"),
}

// Uninstall 是卸载向导及各动作的用户可见文案。
var Uninstall = struct {
	Short, Long, Yes, Running, Plan, Empty, Confirm, Cancel, Failed, ManualDone, Done   string
	SkipBinary, ActionFailed, Entry, CLIEntryCodex, CLIEntryClaude, Pending, FileExists string
	State, Binary, Exists, LockDir, ConfigAndLogs, NoFiles, RemovedFiles                string
	LocateDir, CheckFailed, RemoveFailed, NoConfigDir, NotFound, RemovedState           string
	LocateSelf, RemovedBinary, DelayedBinary, BinaryFailed, ManualRemove                string
}{
	Short:          T("卸载 SerialHub（清理 MCP 接入条目、配置日志与二进制）", "Uninstall SerialHub (MCP entries, configuration, logs and executable)"),
	Long:           T("卸载 SerialHub：\n  1. 移除各 MCP 客户端中的 serialhub 条目（OpenCode/Claude/Cursor/Windsurf/VS Code/Codex，\n     含当前目录的项目级配置；Codex 与 Claude 用户级经官方 CLI 移除，遵循该 CLI 行为）\n  2. 删除配置与日志目录（Linux/macOS: ~/.config/serialhub/；Windows: exe 同目录 config.toml 与 logs/ + 锁目录 %LOCALAPPDATA%\\serialhub\\）\n  3. 删除二进制本身（Windows 下经延迟删除命令）\n默认先列出将清理的项（dry-run），确认后执行；全程幂等，不存在的项自动跳过。\n运行实例检测覆盖默认端口与用户配置文件的地址端口（含 Windows exe 同目录配置）；\n其他自定义端口（-m/-c 临时指定）的实例请自行确认已退出。\n前置清理项失败时会跳过二进制删除并返回非零退出码，修复后重跑即可。\n注意：-c/--config 指定的自定义路径配置不在清理范围，需手动删除。", "Uninstall SerialHub:\n  1. Remove serialhub entries from MCP clients (including project entries in the current directory).\n     Codex and Claude user entries use their official CLIs.\n  2. Remove configuration and logs (on Windows, also remove the LOCALAPPDATA lock directory).\n  3. Remove the executable (delayed deletion on Windows).\nA preview is shown before confirmation; missing items are skipped. Running instances detected on the default and configured addresses block uninstall.\nStop instances using custom -m/-c settings yourself. On failure, executable deletion is skipped so you can retry.\nCustom -c/--config files are not removed."),
	Yes:            T("跳过确认直接卸载", "Uninstall without confirmation"),
	Running:        T("检测到 SerialHub 正在运行（%s/health），请先退出再卸载", "SerialHub is running (%s/health); stop it before uninstalling"),
	Plan:           T("[SerialHub] 卸载将清理以下内容：", "[SerialHub] Uninstall will remove:"),
	Empty:          T("  （无可清理项，已是干净状态）", "  (Nothing to remove; already clean)"),
	Confirm:        T("确认执行卸载? (y/N): ", "Proceed with uninstall? (y/N): "),
	Cancel:         T("已取消卸载。", "Uninstall cancelled."),
	Failed:         T("卸载未完全完成，%d 项失败：\n%s", "Uninstall incomplete: %d item(s) failed:\n%s"),
	ManualDone:     T("[SerialHub] 卸载完成（存在待手动处理项，见上方说明）。", "[SerialHub] Uninstall finished; manual steps remain (see above)."),
	Done:           T("[SerialHub] 卸载完成。感谢使用！", "[SerialHub] Uninstall complete. Thank you!"),
	SkipBinary:     T("  跳过二进制删除：存在失败项，请修复后重跑卸载", "  Skipping executable deletion: fix failures and retry."),
	ActionFailed:   T("  失败 %s：%v\n", "  Failed %s: %v\n"),
	Entry:          T("%s（%s 级）serialhub 条目", "%s (%s scope) serialhub entry"),
	CLIEntryCodex:  T("Codex（用户级，经 codex CLI）serialhub 条目", "Codex (user scope, via codex CLI) serialhub entry"),
	CLIEntryClaude: T("Claude Code（用户级，经 claude CLI）serialhub 条目", "Claude Code (user scope, via claude CLI) serialhub entry"),
	Pending:        T("待检查", "to check"), FileExists: T("配置文件存在", "config file exists"),
	State:  T("配置与日志目录（%s）", "Configuration and logs (%s)"),
	Binary: T("二进制 %s", "Executable %s"), Exists: T("存在", "exists"),
	LockDir:       T("，锁目录 %s", ", lock directory %s"),
	ConfigAndLogs: T("%s 下 config.toml 与 logs/", "config.toml and logs/ in %s"),
	NoFiles:       T("无配置与日志文件，跳过", "No configuration or log files; skipping"),
	RemovedFiles:  T("已删除 %s 下 config.toml 与 logs/", "Removed config.toml and logs/ in %s"),
	LocateDir:     T("无法定位二进制目录：%w", "cannot locate executable directory: %w"),
	CheckFailed:   T("检查 %s 失败：%w", "failed to check %s: %w"),
	RemoveFailed:  T("删除 %s 失败：%w", "failed to remove %s: %w"),
	NoConfigDir:   T("无法定位用户配置目录，跳过", "Cannot locate user configuration directory; skipping"),
	NotFound:      T("%s 不存在，跳过", "%s not found; skipping"),
	RemovedState:  T("已删除 %s/（配置与日志）", "Removed %s/ (configuration and logs)"),
	LocateSelf:    T("无法定位自身：%w", "cannot locate executable: %w"),
	RemovedBinary: T("已删除二进制 %s", "Removed executable %s"),
	DelayedBinary: T("已安排延迟删除二进制 %s（进程退出后生效）", "Scheduled deletion of %s after this process exits"),
	BinaryFailed:  T("二进制删除失败（请手动删除 %s）", "Failed to remove executable; delete %s manually"),
	ManualRemove:  T("删除 %s 失败：%w（请手动删除）", "failed to remove %s: %w (delete it manually)"),
}

// UpgradeErrors 是下载、校验和替换阶段可能直接显示给用户的错误。
var UpgradeErrors = struct {
	APIStatus, MissingTag, MissingAsset, MissingChecksum, ArchiveLimit             string
	DownloadStatus, ResponseLimit, EmptyChecksum, DigestMismatch                   string
	NoEntry, MultipleEntries, MultipleCount, ExtractLimit, EmptyEntry, ReadArchive string
	MoveOld, InstallAndRollback, KeepOld                                           string
}{
	APIStatus:          T("API 返回 %s", "API returned %s"),
	MissingTag:         T("响应缺少 tag_name", "response is missing tag_name"),
	MissingAsset:       T("release %s 未提供 %s（可能暂未构建该平台）", "release %s has no %s asset (platform may not be supported yet)"),
	MissingChecksum:    T("release %s 未提供 %s.sha256 校验文件，为安全起见拒绝升级", "release %s has no %s.sha256 checksum; refusing to upgrade"),
	ArchiveLimit:       T("归档累计解压数据超过上限 %d 字节", "total decompressed archive data exceeds %d bytes"),
	DownloadStatus:     T("下载返回 %s", "download returned %s"),
	ResponseLimit:      T("响应超过大小上限 %d 字节", "response exceeds %d-byte limit"),
	EmptyChecksum:      T("校验文件内容为空", "checksum file is empty"),
	DigestMismatch:     T("摘要不匹配（期望 %s，实际 %s）", "checksum mismatch (expected %s, got %s)"),
	NoEntry:            T("压缩包中未找到普通文件 %s", "archive has no regular file named %s"),
	MultipleEntries:    T("压缩包中有多个 %s 候选，拒绝自动选择", "archive contains multiple %s candidates; refusing to choose"),
	MultipleCount:      T("压缩包中有 %d 个 %s 候选，拒绝自动选择", "archive contains %d %s candidates; refusing to choose"),
	ExtractLimit:       T("解压内容超过上限 %d 字节，拒绝安装", "extracted content exceeds %d-byte limit; refusing to install"),
	EmptyEntry:         T("%s 内容为空", "%s is empty"),
	ReadArchive:        T("读取归档失败：%w", "failed to read archive: %w"),
	MoveOld:            T("移开旧版本失败：%w", "failed to move old executable: %w"),
	InstallAndRollback: T("安装新版本失败：%v；恢复旧版本也失败：%v（旧版本备份在 %s，请手动恢复）", "failed to install new version: %v; rollback also failed: %v (restore backup at %s manually)"),
	KeepOld:            T("[SerialHub] 旧版本 %s 将保留，请稍后手动删除。\n", "[SerialHub] Old version %s remains; delete it manually later.\n"),
}

// MCPSetupInstall 是 mcpsetup 安装路径返回给 CLI 的消息。
var MCPSetupInstall = struct {
	EntryExists, UnknownClient, UnsupportedProject, UnsupportedUser, NoFileTarget                string
	InvalidJSON, MissingCodex, CodexFailed, CodexAdded, MissingClaude, ClaudeFailed, ClaudeAdded string
}{
	EntryExists:        T("目标配置中已存在 serialhub 条目", "serialhub entry already exists in target configuration"),
	UnknownClient:      T("未知客户端 %q（可选：%s）", "unknown client %q (choose %s)"),
	UnsupportedProject: T("%s 不支持项目级配置", "%s does not support project scope"),
	UnsupportedUser:    T("%s 不支持用户级配置", "%s does not support user scope"),
	NoFileTarget:       T("客户端 %s 无文件配置面", "client %s has no file-based configuration"),
	InvalidJSON:        T("解析 %s 失败（不是有效 JSON）：%w", "invalid JSON in %s: %w"),
	MissingCodex:       T("未找到 codex 命令，请先安装 Codex CLI；手动配置：~/.codex/config.toml 中添加 [mcp_servers.serialhub]", "codex command not found; install Codex CLI or add [mcp_servers.serialhub] to ~/.codex/config.toml manually"),
	CodexFailed:        T("codex mcp add 失败：%v\n%s", "codex mcp add failed: %v\n%s"),
	CodexAdded:         T("~/.codex/config.toml（经 codex mcp add 写入）", "~/.codex/config.toml (via codex mcp add)"),
	MissingClaude:      T("未找到 claude 命令，请先安装 Claude Code；或改用项目级 .mcp.json", "claude command not found; install Claude Code or use project-level .mcp.json"),
	ClaudeFailed:       T("claude mcp add 失败：%v\n%s", "claude mcp add failed: %v\n%s"),
	ClaudeAdded:        T("~/.claude.json（经 claude mcp add 写入）", "~/.claude.json (via claude mcp add)"),
}

// MCPSetupRemoval 是 mcpsetup 卸载路径返回给 CLI 的消息。
var MCPSetupRemoval = struct {
	NoProject, NoUser, BadScope, Removed, Absent, Read, Parse              string
	CreateTemp, WriteTemp, CloseTemp, ChmodTemp, Reread, Modified, Replace string
	NoCodex, CodexAbsent, CodexFailed, CodexRemoved                        string
	NoClaude, ClaudeAbsent, ClaudeFailed, ClaudeRemoved                    string
}{
	NoProject:     T("%s 不支持项目级配置", "%s does not support project scope"),
	NoUser:        T("%s 不支持用户级配置", "%s does not support user scope"),
	BadScope:      T("无效层级 %q（可选 project/user）", "invalid scope %q (choose project or user)"),
	Removed:       T("已从 %s 移除 serialhub 条目", "Removed serialhub entry from %s"),
	Absent:        T("%s 中无 serialhub 条目，跳过", "No serialhub entry in %s; skipping"),
	Read:          T("读取 %s 失败：%w", "failed to read %s: %w"),
	Parse:         T("解析 %s 失败（不是有效 JSON）：%w", "invalid JSON in %s: %w"),
	CreateTemp:    T("创建临时文件失败：%w", "failed to create temporary file: %w"),
	WriteTemp:     T("写入临时文件失败：%w", "failed to write temporary file: %w"),
	CloseTemp:     T("关闭临时文件失败：%w", "failed to close temporary file: %w"),
	ChmodTemp:     T("设置临时文件权限失败：%w", "failed to set temporary file permissions: %w"),
	Reread:        T("提交前重读 %s 失败（疑似被外部修改或权限变化）：%w", "failed to reread %s before commit (external change or permissions): %w"),
	Modified:      T("%s 在卸载过程中被其他程序修改，为避免覆盖新改动已中止；请重跑卸载", "%s changed during uninstall; refusing to overwrite it. Retry uninstall"),
	Replace:       T("替换 %s 失败：%w", "failed to replace %s: %w"),
	NoCodex:       T("未找到 codex 命令；请手动编辑 ~/.codex/config.toml 删除 [mcp_servers.serialhub] 段", "codex command not found; manually remove [mcp_servers.serialhub] from ~/.codex/config.toml"),
	CodexAbsent:   T("~/.codex/config.toml 中无 serialhub 条目，跳过", "No serialhub entry in ~/.codex/config.toml; skipping"),
	CodexFailed:   T("codex mcp remove 失败：%v\n%s", "codex mcp remove failed: %v\n%s"),
	CodexRemoved:  T("~/.codex/config.toml（经 codex mcp remove 移除）", "Removed from ~/.codex/config.toml via codex mcp remove"),
	NoClaude:      T("未找到 claude 命令；请运行 claude mcp remove serialhub --scope user 或改用项目级卸载", "claude command not found; run claude mcp remove serialhub --scope user or remove the project entry"),
	ClaudeAbsent:  T("~/.claude.json 中无 serialhub 条目，跳过", "No serialhub entry in ~/.claude.json; skipping"),
	ClaudeFailed:  T("claude mcp remove 失败：%v\n%s", "claude mcp remove failed: %v\n%s"),
	ClaudeRemoved: T("~/.claude.json（经 claude mcp remove --scope user 移除）", "Removed from ~/.claude.json via claude mcp remove --scope user"),
}

// Web 是 WebSocket 服务初始化时返回给 CLI 的错误；WebSocket 内容仍由前端翻译。
var Web = struct {
	InvalidPort string
}{
	InvalidPort: T("端口号必须在 0-65535 范围内", "port must be in the range 0-65535"),
}

// WebEvent 是 WebSocket 系统事件的稳定标识；Web 终端按各自的语言翻译。
// 不把服务端进程语言选出的文本广播给可能使用不同语言的页面。
var WebEvent = struct {
	ListPortsFailed, DisconnectFailed, ShutdownUnsupported string
	MissingConnection, MissingPort, UpdateSettingsFailed   string
	ConnectFailed, Connected, Disconnected, SerialError    string
}{
	ListPortsFailed:      "listPortsFailed",
	DisconnectFailed:     "disconnectFailed",
	ShutdownUnsupported:  "shutdownUnsupported",
	MissingConnection:    "missingConnection",
	MissingPort:          "missingPort",
	UpdateSettingsFailed: "updateSettingsFailed",
	ConnectFailed:        "connectFailed",
	Connected:            "serialConnected",
	Disconnected:         "serialDisconnected",
	SerialError:          "serialError",
}

// ServeErrors 是服务入口返回到 CLI 的错误；运行日志仍保持原文。
var ServeErrors = struct {
	MetadataUnavailable, ListenFailed, PortsOccupied, LockFailed, SelectPortFailed string
	MasterStartFailed, WebSocketFailed, HTTPStartFailed, MasterNotReady            string
}{
	MetadataUnavailable: T("本机已有运行中的主实例，但其服务元数据暂不可读；请稍后重试或直接启动服务", "a master instance is running but its metadata is not readable yet; retry shortly"),
	ListenFailed:        T("监听 %s 失败: %w", "failed to listen on %s: %w"),
	PortsOccupied:       T("端口 %d~%d 均被占用: %w", "ports %d–%d are all in use: %w"),
	LockFailed:          T("获取单实例锁失败：%w", "failed to acquire instance lock: %w"),
	SelectPortFailed:    T("选择监听端口失败: %w", "failed to select listen port: %w"),
	MasterStartFailed:   T("主服务启动失败", "failed to start main service"),
	WebSocketFailed:     T("创建 WebSocket 服务失败: %w", "failed to create WebSocket service: %w"),
	HTTPStartFailed:     T("主服务启动失败（%s 监听失败或初始化异常）", "failed to start main service (listen or initialization failed at %s)"),
	MasterNotReady:      T("检测到主实例 %s，但其 HTTP 服务未就绪；暂无法代理，请稍后重试", "master %s is running but its HTTP service is not ready; retry shortly"),
}
