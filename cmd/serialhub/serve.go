package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/dongly/serialhub/internal/buffer"
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
var errMasterMetadataUnavailable = errors.New("本机已有运行中的主实例，但其服务元数据暂不可读；请稍后重试或直接启动服务")

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
	// 单实例 lock：已有活主（含 tray/前台/另一终端误启动）时报错退出
	if info, err := instance.Acquire(host, mcpPort); err != nil {
		if errors.Is(err, instance.ErrActive) {
			return fmt.Errorf("本机已有运行中的主实例 %s（pid %d，启动于 %s）；同一配置作用域只允许一个主实例", info.URL(), info.PID, info.StartedAt)
		}
		return fmt.Errorf("获取单实例锁失败：%w", err)
	}
	defer instance.Release()

	sm := newSerialManagerFromConfig(cfg)
	buf := buffer.NewDataBuffer()

	enableTray := runtime.GOOS == "windows"
	logrus.Debugf("[SerialHub] 托盘检查: GOOS=%s, enableTray=%v", runtime.GOOS, enableTray)

	if enableTray {
		return runWithTray(cfg, sm, buf)
	}
	// --no-browser 跳过自动打开浏览器；--minimized 仅控制窗口最小化，不再抑制浏览器
	return runWithoutTray(cfg, sm, buf, !noBrowser)
}

func runWithTray(cfg *config.Config, sm *serial.SerialManager, buf *buffer.DataBuffer) error {
	logrus.Debug("[SerialHub] 系统托盘模式已启用")

	if minimized {
		tray.DisableCloseButton()
		tray.HideConsole()
	}

	trayMgr := tray.NewTrayManager(sm, cfg, host, mcpPort, appVersion, minimized)
	saveConfigFunc := createSaveConfigFunc(cfg)
	trayMgr.SetOnConfigChanged(func(port string, baudRate int, dataBits int, parity string, stopBits float64) {
		saveConfigFunc(&serial.Config{
			Port:     port,
			BaudRate: baudRate,
			DataBits: dataBits,
			Parity:   parity,
			StopBits: float32(stopBits),
		})
	})
	sm.SetConfigChangeHandler(saveConfigFunc)

	var wsSrv *web.WebSocketServer
	var cancelFunc context.CancelFunc
	var svcs *runningServices
	var startupErr error

	sm.SetEventHandler(createEventHandler(sm, trayMgr, wsSrv))

	trayMgr.SetOnReady(func() {
		_, cancel := context.WithCancel(context.Background())
		cancelFunc = cancel

		if err := sm.Connect(); err != nil {
			logrus.Debugf("[SerialHub] 自动连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 已自动连接串口: %s", sm.GetConfig().String())
		}

		wsSrv, err := web.NewWebSocketServer(host, mcpPort, func() string {
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

		// --no-browser（脚本静默启动）不自动打开浏览器
		svcs = startServices(sm, wsSrv, buf, !noBrowser)
		if svcs == nil {
			startupErr = fmt.Errorf("主服务启动失败")
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
		return fmt.Errorf("创建 WebSocket 服务失败: %w", err)
	}

	sm.SetEventHandler(createSerialEventHandler(wsSrv))

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	svcs := startServices(sm, wsSrv, buf, autoOpenBrowser)
	if svcs == nil {
		// 诚实失败：HTTP 监听失败（如端口被占）时明确退出，不做无服务的僵尸进程
		return fmt.Errorf("主服务启动失败（%s 监听失败或初始化异常）", addr)
	}

	logrus.Infof("[SerialHub] MCP HTTP 服务: http://%s/mcp", addr)
	logrus.Infof("[SerialHub] 健康检查: http://%s/health", addr)
	logrus.Infof("[SerialHub] WebSocket 端口: %d", mcpPort)
	logrus.Info("[SerialHub] 服务已启动，按 Ctrl+C 退出")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

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

// runStdio stdio 模式：发现主实例则作透明代理（RunStdioProxy）；
// 否则本进程成为主实例（完整服务面，但不弹浏览器、无托盘），
// 随 stdio 连接断开而整体退出（MCP local 惯例：客户端管理进程生命周期）。
func runStdio(cfg *config.Config) error {
	if info := instance.Read(); info != nil {
		if info.Port > 0 {
			return proxyToMaster(*info)
		}
		// 有活主但元数据尚未写完：有界等待其可读
		if live := instance.WaitForInfo(10 * time.Second); live != nil {
			return proxyToMaster(*live)
		}
		return errMasterMetadataUnavailable
	}

	logrus.Info("[SerialHub] stdio 模式：未发现主实例，本进程成为主实例")
	// 单实例 lock：与前台主实例互斥。若此刻另一进程抢先成为主
	// （Read 与 Acquire 之间的竞态），回退到代理模式（等待其对 HTTP 就绪）。
	if _, err := instance.Acquire(host, mcpPort); err != nil {
		if errors.Is(err, instance.ErrActive) {
			if live := instance.WaitForInfo(10 * time.Second); live != nil {
				return proxyToMaster(*live)
			}
			return errMasterMetadataUnavailable
		}
		return fmt.Errorf("获取单实例锁失败：%w", err)
	}
	defer instance.Release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigChan)
		<-sigChan
		cancel()
	}()

	sm := newSerialManagerFromConfig(cfg)
	buf := buffer.NewDataBuffer()

	wsSrv, err := web.NewWebSocketServer(host, mcpPort, func() string {
		if sm.IsConnected() {
			return sm.GetConfig().String()
		}
		return ""
	})
	if err != nil {
		return fmt.Errorf("创建 WebSocket 服务失败: %w", err)
	}
	sm.SetConfigChangeHandler(createSaveConfigFunc(cfg))
	sm.SetEventHandler(createSerialEventHandler(wsSrv))

	svcs := startServices(sm, wsSrv, buf, false)
	if svcs == nil {
		return fmt.Errorf("主服务启动失败")
	}

	if err := svcs.mcp.RunStdioTransport(ctx); err != nil {
		logAsync(logrus.DebugLevel, fmt.Sprintf("[SerialHub] stdio 传输结束: %v", err))
	}
	// 退出路径控制线程不同步写日志：通知由 gracefulShutdown/有界收尾输出
	gracefulShutdown(sm, svcs)
	return nil
}

// proxyToMaster 等待主实例 HTTP 就绪后以 stdio 透明代理运行。
// 主实例先持锁写元数据、后启动 HTTP；此处的等待不改变锁所有权。
func proxyToMaster(info instance.LockInfo) error {
	if !instance.WaitReady(info, 15*time.Second) {
		return fmt.Errorf("检测到主实例 %s，但其 HTTP 服务未就绪；暂无法代理，请稍后重试", info.URL())
	}
	logrus.Infof("[SerialHub] stdio 模式：主实例 %s 就绪，以透明代理运行", info.URL())
	return mcp.RunStdioProxy(context.Background(), info.URL())
}

func createSaveConfigFunc(cfg *config.Config) func(*serial.Config) {
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

func createEventHandler(sm *serial.SerialManager, trayMgr *tray.TrayManager, wsSrv *web.WebSocketServer) func(serial.Event) {
	return func(event serial.Event) {
		trayMgr.UpdateSerialStatus()

		if wsSrv != nil {
			var msg string
			switch event.Type {
			case serial.EventConnected:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口已连接: %s\r\n", event.Port)
			case serial.EventDisconnected:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口已断开\r\n")
			case serial.EventError:
				msg = fmt.Sprintf("\r\n[SerialHub] 串口错误: %s\r\n", event.Message)
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
			msg = fmt.Sprintf("\r\n[SerialHub] 串口已连接: %s\r\n", event.Port)
		case serial.EventDisconnected:
			msg = fmt.Sprintf("\r\n[SerialHub] 串口已断开\r\n")
		case serial.EventError:
			msg = fmt.Sprintf("\r\n[SerialHub] 串口错误: %s\r\n", event.Message)
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
		bridgeSrv.SetCommandHandler(createCommandHandler(sm))
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
