package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/internal/instance"
	"github.com/dongly/serialhub/internal/logagg"
	"github.com/dongly/serialhub/pkg/bridge"
	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/mcp"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/dongly/serialhub/pkg/tray"
	"github.com/dongly/serialhub/pkg/web"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// errMasterMetadataUnavailable：检测到活主，但 lock 元数据尚未可读
// （主实例刚持锁、写元数据完成之前的短暂窗口）。
var errMasterMetadataUnavailable = errors.New(i18n.ServeErrors.MetadataUnavailable)

// webShutdown 由 Web 终端「退出」按钮触发（shutdown 命令），
// 语义等价于收到 SIGINT：各主循环感知后统一走 gracefulShutdown。
// MCP 工具不提供关闭能力（高危操作仅限人工在 Web 终端确认后执行）。
// 容量 1：主循环尚未开始等待时（启动窗口内）的首次请求也不会丢失。
var webShutdown = make(chan struct{}, 1)

// requestWebShutdown 非阻塞地发起停机请求；重复触发（多个页面同时点击）被忽略。
func requestWebShutdown() {
	select {
	case webShutdown <- struct{}{}:
	default:
	}
}

// runServe 入口分流：--stdio → stdio 模式（lock 发现有主则代理）；
// 否则主实例。
func runServe(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()

	if stdioMode {
		// stdio 模式：stdout 承载 MCP 协议流，日志只写文件
		setupLogger(cfg)
		setLogFileOnly()
		defer closeLogger()
		return runStdio(cfg)
	}

	setupLogger(cfg)
	defer closeLogger()
	logrus.Infof("[SerialHub] SerialHub v%s 启动中...", appVersion)
	if logDataEffective {
		logrus.Info("[SerialHub] 数据内容日志已开启（--log-data / SERIALHUB_LOG_DATA=1），500ms 时间窗聚合输出")
	}

	return runMaster(cfg)
}

// maxPortFallback 是端口被占用时自动 +1 迁移的尝试上限。
const maxPortFallback = 10

// resolveListenPort 预检并解析实际监听端口：请求端口被占
// （EADDRINUSE，典型如 WSL/Windows 双侧同端口时对侧实例的 localhost
// 转发占位）时自动 +1 递增，最多尝试 maxPortFallback 个。
// 仅探测端口占用；预检与真实 bind 之间的短暂窗口由启动失败兜底
// （下一轮启动会继续向后迁移）。非占用类错误（如地址不可用）原样返回。
func resolveListenPort(host string, startPort int) (int, error) {
	var firstErr error
	for i := 0; i <= maxPortFallback; i++ {
		port := startPort + i
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !isAddrInUse(err) {
			return 0, fmt.Errorf(i18n.ServeErrors.ListenFailed, net.JoinHostPort(host, strconv.Itoa(port)), err)
		}
	}
	return 0, fmt.Errorf(i18n.ServeErrors.PortsOccupied, startPort, startPort+maxPortFallback, firstErr)
}

// newSerialManagerFromConfig 构造串口管理器（初始化失败回退默认配置）。
func newSerialManagerFromConfig(cfg *config.Config) *serial.SerialManager {
	serialCfg := configToSerialConfig(&cfg.Serial)
	sm, _ := serial.NewSerialManager(serialCfg)
	if sm == nil {
		serialCfg = &serial.Config{Port: "", BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: 1}
		sm, _ = serial.NewSerialManager(serialCfg)
	}
	return sm
}

// runMaster 主实例：完整服务面（HTTP/MCP/xterm web/本侧串口）。
func runMaster(cfg *config.Config) error {
	// 单实例 lock：已有活主（含 tray/前台/另一终端误启动）时不报错，
	// 与 --stdio 的发现行为一致——本进程转 stdio 透明代理挂起；
	// 主实例退出后原地接管（同 PID 竞锁成主），语义与 --stdio 模式一致。
	// 前台代理的 stdio 是终端而非 MCP 客户端：重试轮次间复用交回的连接
	// （避免重连产生第二个终端 reader 争抢输入），最终接管成主后丢弃
	// （服务面不读终端输入），并以无托盘、不开浏览器的形态运行。
	tookOver := false
	var reused *mcp.StdioHandoff
	var budget takeoverBudget
	port := mcpPort // 记住当前主的端口：失联接管时继承（clients 已连在那里）
	for {
		info, err := instance.Acquire(host, port)
		if err == nil {
			break // 竞得锁：成为主实例（首次启动或接管）
		}
		if !errors.Is(err, instance.ErrActive) {
			return fmt.Errorf(i18n.ServeErrors.LockFailed, err)
		}
		if info.Port <= 0 {
			// 活主但元数据尚未写完：有界等待其可读
			live := instance.WaitForInfo(10 * time.Second)
			if live == nil {
				return errMasterMetadataUnavailable
			}
			info = *live
		}
		logrus.Infof("[SerialHub] 检测到主实例 %s（pid %d），本进程以代理模式运行（Ctrl+C 退出）", info.URL(), info.PID)
		port = info.Port
		reason, handoff, perr := proxyToMaster(info, reused)
		if reason != mcp.ProxyMasterLost {
			if perr != nil {
				return perr
			}
			return nil
		}
		// 失联分诊与计数与 runStdio 共用一套（takeoverBudget.classify）。
		retry, giveUp := budget.classify()
		if giveUp != nil {
			return giveUp
		}
		if retry {
			time.Sleep(takeoverBackoff)
			reused = handoff // 复用同一 stdio 连接，避免重连产生第二个终端 reader
			continue
		}
		logrus.Infof("[SerialHub] 主实例已失联（%v），本进程原地升级为主实例", perr)
		tookOver = true
		reused = handoff // 与 runStdio 对称：退避后被抢占回代理时复用本轮交接物
		// 小退避：等旧主端口与锁完全释放，降低竞态窗口
		time.Sleep(takeoverBackoff)
	}
	defer instance.Release()

	// 接管继承死主端口；首次成主时 port==mcpPort，行为不变
	mcpPort = port

	// 端口占用自动迁移：Windows/WSL 双侧同用默认端口时，后启动一侧的
	// 端口会被对侧实例的 localhost 转发（wslrelay）或其他程序占用；
	// 逐个 +1 试探（最多 10 个），保证双侧都能独立成主实例。
	actualPort, err := resolveListenPort(host, mcpPort)
	if err != nil {
		return fmt.Errorf(i18n.ServeErrors.SelectPortFailed, err)
	}
	if actualPort != mcpPort {
		logrus.Infof("[SerialHub] 端口 %d 已被占用（可能为对侧系统实例的 localhost 转发或其他程序），自动改用 %d", mcpPort, actualPort)
		mcpPort = actualPort
		instance.UpdatePort(actualPort)
	}

	// 成为主实例后才回写配置（含 CLI 参数合并结果）；重复启动的代理实例不落盘。
	// 注意：磁盘始终记录请求端口（cfg.MCP.HTTPPort 未被 actualPort 覆盖）——
	// 端口迁移是本次运行期的适配，不改变用户的端口意图
	persistConfig(cfg)

	sm := newSerialManagerFromConfig(cfg)
	buf := buffer.NewDataBuffer()

	// 接管路径不重初始化托盘：接管者是被拉起的第二实例，
	// 服务面与 stdio 接管保持一致（无托盘、无浏览器）。
	enableTray := runtime.GOOS == "windows" && !tookOver
	logrus.Debugf("[SerialHub] 托盘检查: GOOS=%s, enableTray=%v", runtime.GOOS, enableTray)

	if enableTray {
		return runWithTray(cfg, sm, buf)
	}
	// --no-browser 跳过自动打开浏览器；--minimized 仅控制窗口最小化，不再抑制浏览器。
	// 接管路径不开浏览器（服务面无交互，与 stdio 接管语义一致）。
	autoOpenBrowser := !noBrowser && !tookOver
	return runWithoutTray(cfg, sm, buf, autoOpenBrowser)
}

// autoConnectSerial 启动时自动连接配置中记录的串口（上次使用/连接的端口）。
// 端口为空时跳过；连接失败只记 debug 日志，不阻塞启动（用户可稍后手动连接）。
func autoConnectSerial(sm *serial.SerialManager, cfg *config.Config) {
	if !cfg.Serial.AutoConnect {
		logrus.Debug("[SerialHub] 已禁用启动自动连接串口")
		return
	}
	if sm.GetConfig().Port == "" {
		return
	}
	if err := sm.Connect(); err != nil {
		logrus.Debugf("[SerialHub] 自动连接串口失败: %v", err)
		return
	}
	logrus.Infof("[SerialHub] 已自动连接串口: %s", sm.GetConfig().String())
}

func runWithTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer) error {
	logrus.Debug("[SerialHub] 系统托盘模式已启用")

	if minimized {
		tray.DisableCloseButton()
		tray.HideConsole()
	}

	trayMgr := tray.NewTrayManager(sm, cfg, host, mcpPort, appVersion, minimized)
	saveConfigFunc := createSaveConfigFunc(cfg)
	trayMgr.SetOnConfigChanged(saveConfigFunc)
	sm.SetConfigChangeHandler(saveConfigFunc)

	var wsSrv *web.WebSocketServer
	var cancelFunc context.CancelFunc
	var svcs *runningServices
	var startupErr error

	trayMgr.SetOnReady(func() {
		_, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		autoConnectSerial(sm, cfg)

		var err error
		wsSrv, err = web.NewWebSocketServer(host, mcpPort, func() string {
			if sm.IsConnected() {
				return sm.GetConfig().String()
			}
			return ""
		})
		if err != nil {
			logrus.Errorf("[SerialHub] 创建 WebSocket 服务失败: %v", err)
			startupErr = err
			trayMgr.Quit() // 服务启动失败：退出托盘（而非保留占锁的空壳实例）
			return
		}

		// 事件 handler 必须在 wsSrv 创建成功后注册：createEventHandler 按
		// 值接收 wsSrv，早于此处注册会把 nil 拷进闭包，serial_event 广播
		// 将永远发不到任何 WebSocket 客户端。
		sm.SetEventHandler(createEventHandler(sm, trayMgr, wsSrv))

		// --no-browser（脚本静默启动）不自动打开浏览器
		svcs = startServices(sm, wsSrv, buf, !noBrowser)
		if svcs == nil {
			startupErr = errors.New(i18n.ServeErrors.MasterStartFailed)
			trayMgr.Quit() // MCP/HTTP 启动失败：退出托盘
			return
		}

		addr := fmt.Sprintf("%s:%d", host, mcpPort)
		logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
		logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
		logrus.Infof("[SerialHub] WebSocket 端口: %d", mcpPort)
	})

	trayMgr.SetOnExit(func() {
		if cancelFunc != nil {
			cancelFunc()
		}
	})

	// Web 终端「退出」按钮：退出托盘主循环（systray.Run 返回后走统一停机路径）。
	// goroutine 与进程同生命周期，无需单独回收。
	go func() {
		<-webShutdown
		trayMgr.Quit()
	}()

	trayMgr.Run(context.Background())

	// 「正在关闭」通知由 gracefulShutdown 的停机 goroutine 输出（控制线程不同步写日志）
	gracefulShutdown(sm, svcs)
	closeLogger()
	return startupErr
}

// runningServices 聚合主实例运行期服务句柄，供停机时按序关闭。
type runningServices struct {
	bridge     *bridge.DataBridge
	mcp        *mcp.MCPServer
	httpServer *http.Server
}

// gracefulShutdown 按序停机：数据桥 → MCP HTTP → 串口。
// 服务清理最多等待 3 秒，日志收尾额外最多等待 500ms；任一环节卡死
// （如 USB 掉线导致串口 Close 阻塞）时整体超时后强制退出。
// 「正在关闭」通知放在停机 goroutine 内输出：控制线程（含超时计时）
// 不做任何同步日志调用——日志输出链可能正被卡死的 I/O 占用。
func gracefulShutdown(sm *serial.SerialManager, svcs *runningServices) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		logrus.Info("[SerialHub] 正在关闭...") // 卡死只阻塞本 goroutine，由下方超时兜底
		if svcs != nil {
			if svcs.bridge != nil {
				if err := svcs.bridge.Stop(); err != nil {
					logrus.Warnf("[SerialHub] 停止数据桥接失败: %v", err)
				}
			}
			if svcs.httpServer != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := svcs.httpServer.Shutdown(ctx); err != nil {
					logrus.Warnf("[SerialHub] HTTP 服务关闭异常: %v", err)
				}
				cancel()
			}
		}
		if sm != nil {
			if err := sm.Close(); err != nil {
				logrus.Debugf("[SerialHub] 串口管理器关闭异常: %v", err)
			}
		}
	}()

	select {
	case <-done:
		// 两分支均不在控制线程上同步写日志：日志输出链可能正被卡死的 I/O
		// 占用（stdout 背压/文件锁），同步调用会让停机流程永远无法返回
		// （强退分支则永远到不了 os.Exit）。通知消息与收尾一并放进有界
		// goroutine（服务清理最多 3s + 日志收尾最多 500ms）。
		flushBoundedLog(logrus.InfoLevel, "[SerialHub] 已完成停机")
	case <-time.After(3 * time.Second):
		flushBoundedLog(logrus.WarnLevel, "[SerialHub] 停机超时（3s），强制退出")
		closeLogger()
		os.Exit(0)
	}
}

// logAsync 异步输出一条日志：调用线程绝不等待日志 I/O。退出等
// 控制路径上，日志输出链可能正被卡死的 I/O 占用（stdout 背压/文件锁），
// 同步调用会永久阻塞控制流。消息可能乱序或丢失，仅用于收尾通知。
func logAsync(level logrus.Level, msg string) {
	go func() {
		switch level {
		case logrus.DebugLevel:
			logrus.Debug(msg)
		case logrus.WarnLevel:
			logrus.Warn(msg)
		default:
			logrus.Info(msg)
		}
	}()
}

// flushBoundedLog 在有界 goroutine 内输出一条停机通知并收尾数据内容日志：
// 日志 I/O 异常卡死时不阻断控制线程，超时（500ms）放弃消息与最后一窗
// 直接返回。
func flushBoundedLog(level logrus.Level, msg string) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if msg != "" {
			if level == logrus.WarnLevel {
				logrus.Warn(msg)
			} else {
				logrus.Info(msg)
			}
		}
		logagg.FlushAll()
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}

// runWithoutTray 无托盘前台主实例（Linux/WSL）。
// autoOpenBrowser 控制启动后是否自动打开 xterm web。
func runWithoutTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer, autoOpenBrowser bool) error {
	sm.SetConfigChangeHandler(createSaveConfigFunc(cfg))

	wsSrv, err := web.NewWebSocketServer(host, mcpPort, func() string {
		if sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf(i18n.ServeErrors.WebSocketFailed, err)
	}

	sm.SetEventHandler(createSerialEventHandler(wsSrv))
	autoConnectSerial(sm, cfg)

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	svcs := startServices(sm, wsSrv, buf, autoOpenBrowser)
	if svcs == nil {
		// 诚实失败：HTTP 监听失败（如端口被占）时明确退出，不做无服务的僵尸进程
		return fmt.Errorf(i18n.ServeErrors.HTTPStartFailed, addr)
	}

	logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
	logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
	logrus.Infof("[SerialHub] WebSocket 端口: %d", mcpPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigChan:
	case <-webShutdown: // Web 终端「退出」按钮
	}

	// 「正在关闭」通知由 gracefulShutdown 的停机 goroutine 输出（控制线程不同步写日志）
	gracefulShutdown(sm, svcs)
	closeLogger()
	return nil
}

// closeSerialBounded 有界关闭串口管理器：USB 掉线导致串口 Close/Read
// 阻塞时限时放弃等待。返回 manager 关闭流程是否在时限内返回——
// 不代表底层驱动 Close 已完成（其异步执行，句柄释放不在此等待）；
// 调用方据此决定后续（进程退出场景句柄由 OS 回收，继续运行
// 场景需提示资源滞留）。
// 超时告警异步输出：日志链可能正被卡死的 I/O 占用，控制线程不得等待。
func closeSerialBounded(sm *serial.SerialManager, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := sm.Close(); err != nil {
			logrus.Debugf("[SerialHub] 串口管理器关闭异常: %v", err)
		}
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		logAsync(logrus.WarnLevel, fmt.Sprintf("[SerialHub] 串口管理器关闭超时（%s），放弃等待", timeout))
		return false
	}
}

// maxStdioTakeovers 限制单进程内「代理失联→原地接管」的最大尝试次数，
// 防止主实例反复快速崩溃导致的病态循环。语义＝最多允许这么多次真实的
// 接管尝试（第 6 次失联才放弃）。
const maxStdioTakeovers = 5

// maxWaitReadyWaits 限制「主仍持锁但 HTTP 未就绪」的连续重探次数：慢启动
// 不该消耗接管预算，但主若永久持锁不就绪，代理也不能无界自旋。
const maxWaitReadyWaits = 3

// takeoverBackoff 是判定主失联后、原地接管前的退避；包级变量便于测试注入。
var takeoverBackoff = 500 * time.Millisecond

// waitForMasterReady 等待主实例 HTTP 就绪；包级变量便于测试注入
// （验证「持有 stdio 连接时重试期间主再次失联」的分类行为）。
var waitForMasterReady = instance.WaitReady

// takeoverBudget 聚合「失联→接管」循环的两组计数，runMaster/runStdio 共用。
type takeoverBudget struct {
	takeovers      int // 真失联（锁空）后的接管尝试次数
	waitReadyWaits int // 主持锁但 HTTP 未就绪的连续重探次数
}

// classify 对一次主失联做分诊。锁仍被持有时有两种情况，都不耗接管预算：
// 元数据健康（Port>0）＝主只是慢启动，重探并计入 waitReadyWaits（设独立
// 上限，防主挂死在持锁状态时代理无限自旋）；元数据退化（Port<=0，如锁
// 文件被清）同样「没死透」，交由下一轮重探。只有锁真正空了才计入接管
// 尝试 takeovers（超上限返回放弃错误）。
// 返回：retry=true 回代理重探；giveUp 非 nil 预算耗尽；两者零值＝放行接管。
func (b *takeoverBudget) classify() (retry bool, giveUp error) {
	live := instance.Read()
	if live != nil {
		if live.Port > 0 {
			b.waitReadyWaits++
			if b.waitReadyWaits > maxWaitReadyWaits {
				return false, errMasterMetadataUnavailable
			}
		}
		return true, nil
	}
	b.waitReadyWaits = 0
	b.takeovers++
	if b.takeovers > maxStdioTakeovers {
		return false, fmt.Errorf(i18n.ServeErrors.TakeoverGiveUp, maxStdioTakeovers)
	}
	return false, nil
}

// runStdio stdio 模式：发现主实例则作透明代理（RunStdioProxy）；
// 否则本进程成为主实例（完整服务面，但不弹浏览器、无托盘），
// 随 stdio 连接断开而整体退出（MCP local 惯例：客户端管理进程生命周期）。
//
// 代理与主失联（/health 探活连续失败）时原地升级为主实例：复用既有
// stdio 连接（同一 reader goroutine，避免两个 reader 争抢 os.Stdin），
// 重新竞锁后以主身份继续服务；若锁已被新主占据则回到代理模式。
func runStdio(cfg *config.Config) error {
	var reused *mcp.StdioHandoff
	var lastMaster *instance.LockInfo // 最近一次代理的主：接管时继承其端口
	var budget takeoverBudget
	for {
		info := instance.Read()
		if info == nil {
			// 无活主：尝试成为主实例（竞锁失败则改走代理）。
			// 端口：首次成主用本进程 mcpPort；接管继承死主端口。
			port := mcpPort
			if lastMaster != nil && lastMaster.Port > 0 {
				port = lastMaster.Port
			}
			err := serveAsStdioMaster(cfg, reused, port)
			if !errors.Is(err, instance.ErrActive) {
				return err
			}
			// 竞态：另一进程抢先成为主，等其元数据可读后回代理
			if live := instance.WaitForInfo(10 * time.Second); live != nil {
				info = live
			} else {
				return errMasterMetadataUnavailable
			}
		} else if info.Port <= 0 {
			// 有活主但元数据尚未写完：有界等待其可读
			if live := instance.WaitForInfo(10 * time.Second); live != nil {
				info = live
			} else {
				return errMasterMetadataUnavailable
			}
		}

		// 此处 info 为活主：以透明代理运行
		lastMaster = info
		reason, conn, err := proxyToMaster(*info, reused)
		if reason != mcp.ProxyMasterLost {
			if err != nil {
				return err
			}
			return nil
		}
		// 失联分诊与计数见 takeoverBudget.classify（与前台模式共用一套）。
		retry, giveUp := budget.classify()
		if giveUp != nil {
			return giveUp
		}
		if retry {
			time.Sleep(takeoverBackoff)
			reused = conn
			continue
		}
		logrus.Infof("[SerialHub] 主实例已失联（%v），原地升级为主实例", err)
		// 小退避：等旧主端口与锁完全释放，降低竞态窗口
		time.Sleep(takeoverBackoff)
		reused = conn
	}
}

// proxyToMaster 等待主实例 HTTP 就绪后以 stdio 透明代理运行。
// 主实例先持锁写元数据、后启动 HTTP；此处的等待不改变锁所有权。
// reused 非 nil 时复用该 stdio 连接（上轮代理交回），nil 则全新连接。
//
// 就绪等待失败时统一归为失联（ProxyMasterLost），交由上层有界重试重新
// 探锁：锁空则原地升级为主，锁仍被占则继续等待/代理；已持有的 stdio
// 连接（接管重试中）原样保留，避免代理直接退出丢弃连接。
func proxyToMaster(info instance.LockInfo, reused *mcp.StdioHandoff) (mcp.ProxyExitReason, *mcp.StdioHandoff, error) {
	if !waitForMasterReady(info, 15*time.Second) {
		// 元数据存在但 HTTP 未就绪：主实例可能在启动中，也可能已死。
		// 首次无连接时 handoff 为 nil，升级时才创建 stdin reader
		//（此刻尚无 reader，不存在争抢）。
		return mcp.ProxyMasterLost, reused, fmt.Errorf(i18n.ServeErrors.MasterNotReady, info.URL())
	}
	logrus.Infof("[SerialHub] 主实例 %s 就绪，以透明代理运行", info.URL())
	logrus.Infof("[SerialHub] Web 终端地址: %s/terminal", info.URL())
	return mcp.RunStdioProxy(context.Background(), info.URL(), reused)
}

// serveAsStdioMaster 以主实例身份运行（stdio 模式完整服务面）。
// handoff 非 nil 时复用其 stdio 连接与会话状态服务（原地接管场景）；
// nil 则全新连接 stdin/stdout。竞锁失败返回 instance.ErrActive，
// 由调用方回到代理模式。
// 注意：接管路径使用启动时的配置快照，不重新加载配置文件。
// port 指定监听端口：首次成主用本进程 mcpPort，接管则继承死主端口
// （clients 已经连在那里），并按 runMaster 同款策略回退相邻端口。
func serveAsStdioMaster(cfg *config.Config, handoff *mcp.StdioHandoff, port int) error {
	logrus.Info("[SerialHub] stdio 模式：本进程成为主实例")
	// 单实例 lock：与前台主实例互斥。若此刻另一进程抢先成为主
	// （Read 与 Acquire 之间的竞态），返回 ErrActive 由调用方回代理。
	if _, err := instance.Acquire(host, port); err != nil {
		if errors.Is(err, instance.ErrActive) {
			return err
		}
		return fmt.Errorf(i18n.ServeErrors.LockFailed, err)
	}
	defer instance.Release()

	// 成为主实例后才回写配置；stdio 代理模式不落盘
	persistConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigChan)
		select {
		case <-sigChan:
		case <-webShutdown: // Web 终端「退出」按钮
		}
		cancel()
	}()

	sm := newSerialManagerFromConfig(cfg)
	buf := buffer.NewDataBuffer()

	// 与 runMaster 同款端口回退：死主端口可能仍处于 TIME_WAIT 或被临时占用。
	actualPort, err := resolveListenPort(host, port)
	if err != nil {
		return fmt.Errorf(i18n.ServeErrors.SelectPortFailed, err)
	}
	// 同步全局：下游 startServices/HTTP 监听均按包级 mcpPort 建址
	//（与 runMaster 的端口迁移先例一致）。
	mcpPort = actualPort
	if actualPort != port {
		logrus.Infof("[SerialHub] 请求端口 %d 不可用，回退到 %d", port, actualPort)
		instance.UpdatePort(actualPort)
	}

	wsSrv, err := web.NewWebSocketServer(host, actualPort, func() string {
		if sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf(i18n.ServeErrors.WebSocketFailed, err)
	}
	sm.SetConfigChangeHandler(createSaveConfigFunc(cfg))
	sm.SetEventHandler(createSerialEventHandler(wsSrv))
	svcs := startServices(sm, wsSrv, buf, false)
	if svcs == nil {
		return errors.New(i18n.ServeErrors.MasterStartFailed)
	}

	// 原地接管时复用代理建立的 stdio 连接（同一 reader goroutine），
	// 避免二次连接 stdin 造成两个 reader 争抢字节流。
	var serveErr error
	if handoff != nil {
		serveErr = svcs.mcp.RunStdioConnection(ctx, handoff)
	} else {
		serveErr = svcs.mcp.RunStdioTransport(ctx)
	}
	if serveErr != nil {
		logAsync(logrus.DebugLevel, fmt.Sprintf("[SerialHub] stdio 传输结束: %v", serveErr))
	}
	// 退出路径控制线程不同步写日志：通知由 gracefulShutdown/有界收尾输出
	gracefulShutdown(sm, svcs)
	return nil
}

func createSaveConfigFunc(cfg *config.Config) func(*serial.Config) {
	if configPath == "" {
		logrus.Warn("[SerialHub] 未指定配置文件路径，托盘/串口配置修改不会持久化")
	}
	return func(serialCfg *serial.Config) {
		cfg.Serial.Port = serialCfg.Port
		cfg.Serial.BaudRate = serialCfg.BaudRate
		cfg.Serial.DataBits = serialCfg.DataBits
		cfg.Serial.Parity = serialCfg.Parity
		cfg.Serial.StopBits = float64(serialCfg.StopBits)
		if configPath != "" {
			if err := config.Save(configPath, cfg); err != nil {
				logrus.Warnf("[SerialHub] 保存配置失败: %v", err)
			}
		}
	}
}

// webSerialEvent 为每个 Web 页面广播语言无关的事件，由页面按自身语言渲染。
func webSerialEvent(code, port string) string {
	b, _ := json.Marshal(map[string]interface{}{
		"type": "serial_event",
		"data": map[string]string{"code": code, "port": port},
	})
	return string(b)
}

func createEventHandler(sm *serial.SerialManager, trayMgr *tray.TrayManager, wsSrv *web.WebSocketServer) func(serial.Event) {
	return func(event serial.Event) {
		trayMgr.UpdateSerialStatus()

		if wsSrv != nil {
			var msg string
			switch event.Type {
			case serial.EventConnected:
				msg = webSerialEvent(i18n.WebEvent.Connected, event.Port)
			case serial.EventDisconnected:
				msg = webSerialEvent(i18n.WebEvent.Disconnected, "")
			case serial.EventError:
				// 驱动错误可能含本地化文本；事件只携带标识，详细错误在服务日志。
				msg = webSerialEvent(i18n.WebEvent.SerialError, "")
			}
			if msg != "" {
				wsSrv.Broadcast([]byte(msg))
			}
		}
	}
}

func createSerialEventHandler(wsSrv *web.WebSocketServer) func(serial.Event) {
	return func(event serial.Event) {
		var msg string
		switch event.Type {
		case serial.EventConnected:
			msg = webSerialEvent(i18n.WebEvent.Connected, event.Port)
		case serial.EventDisconnected:
			msg = webSerialEvent(i18n.WebEvent.Disconnected, "")
		case serial.EventError:
			msg = webSerialEvent(i18n.WebEvent.SerialError, "")
		}
		if msg != "" {
			wsSrv.Broadcast([]byte(msg))
		}
	}
}

// startServices 启动主实例服务面：数据桥 + MCP HTTP。
// 返回服务句柄集合（stdio 模式需叠跑 stdio 传输）；启动失败返回 nil。
func startServices(sm *serial.SerialManager, wsSrv *web.WebSocketServer, buf *buffer.DataBuffer, autoOpenBrowser bool) *runningServices {
	svcs := &runningServices{}
	bridgeSrv, err := bridge.NewDataBridge(sm, wsSrv, buf)
	if err != nil {
		logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
	} else {
		bridgeSrv.SetCommandHandler(createCommandHandler(sm, requestWebShutdown))
		bridgeSrv.Start()
		svcs.bridge = bridgeSrv
		logrus.Info("[SerialHub] 数据桥接已启动")
	}

	mcpSrv, err := mcp.NewMCPServer(sm, buf, wsSrv)
	if err != nil {
		logrus.Errorf("[SerialHub] 创建 MCP 服务失败: %v", err)
		return nil
	}
	if err := mcpSrv.RegisterTools(); err != nil {
		logrus.Errorf("[SerialHub] 注册 MCP 工具失败: %v", err)
		return nil
	}
	svcs.mcp = mcpSrv

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	httpSrv, err := mcpSrv.StartHTTPServer(addr, autoOpenBrowser)
	if err != nil {
		logrus.Errorf("[SerialHub] 启动 HTTP 服务失败: %v", err)
		return nil
	}
	svcs.httpServer = httpSrv
	return svcs
}
