package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/dongly/serialhub/internal/buffer"
	"github.com/dongly/serialhub/internal/federation"
	"github.com/dongly/serialhub/pkg/bridge"
	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/mcp"
	"github.com/dongly/serialhub/pkg/serial"
	"github.com/dongly/serialhub/pkg/tray"
	"github.com/dongly/serialhub/pkg/web"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// runServe 入口分流：--stdio → stdio 模式；联邦发现命中 → 从实例；否则主实例。
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

	masterURL := federation.DiscoverMaster(mcpPort)
	if masterURL != "" {
		logrus.Infof("[SerialHub] 检测到主实例 %s，以从实例模式运行", masterURL)
		return runWorker(cfg, masterURL)
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

// runMaster 主实例：完整服务面（HTTP/MCP/xterm web/本侧串口/联邦入口）。
func runMaster(cfg *config.Config) error {
	sm := newSerialManagerFromConfig(cfg)
	buf := buffer.NewDataBuffer()

	enableTray := runtime.GOOS == "windows"
	logrus.Debugf("[SerialHub] 托盘检查: GOOS=%s, enableTray=%v", runtime.GOOS, enableTray)

	if enableTray {
		return runWithTray(cfg, sm, buf)
	}
	// --minimized 跨平台生效：脚本静默启动不自动打开浏览器
	return runWithoutTray(cfg, sm, buf, !minimized)
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
			return
		}

		// --minimized（脚本静默启动）不自动打开浏览器
		startServices(sm, wsSrv, buf, !minimized)

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

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	return nil
}

// runWithoutTray 无托盘前台主实例（Linux/WSL 或晋升场景）。
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
	if startServices(sm, wsSrv, buf, autoOpenBrowser) == nil {
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

	logrus.Info("[SerialHub] 正在关闭...")
	closeLogger()
	return nil
}

// runWorker 从实例：前台进程，贡献本侧串口给主实例，本侧反代 /mcp + /health。
// Ctrl+C 退出即脱离联邦；主实例失联且重连失败时自动晋升为主实例。
func runWorker(cfg *config.Config, masterURL string) error {
	sm := newSerialManagerFromConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		cancel()
	}()

	promoteCh := make(chan struct{}, 1)
	onPromote := func() {
		select {
		case promoteCh <- struct{}{}:
		default:
		}
	}

	// 联邦客户端：上报本侧端口 + 响应主侧串口操作 + 数据上行
	go func() {
		if err := federation.RunWorker(ctx, masterURL, federation.LocalSide(), sm, onPromote); err != nil {
			logrus.Errorf("[SerialHub] 联邦客户端异常退出: %v", err)
			cancel()
		}
	}()

	// 本侧反代：/mcp → 主实例，/health 本地应答（同侧端口被占则跳过）
	stopProxy := startWorkerProxy(masterURL)
	defer stopProxy() // 进程退出兜底（幂等）

	logrus.Info("[SerialHub] 从实例已启动（Ctrl+C 退出；主实例失联时自动晋升）")

	select {
	case <-ctx.Done():
		logrus.Info("[SerialHub] 从实例退出")
		return nil
	case <-promoteCh:
		// 晋升前复查：主实例可能只是网络抖动，或已有新主接管
		logrus.Info("[SerialHub] 与主实例失联，正在确认是否晋升...")
		if newMaster := federation.DiscoverMaster(mcpPort); newMaster != "" {
			logrus.Infof("[SerialHub] 发现新主实例 %s，重新以从实例模式接入", newMaster)
			stopProxy() // 停旧反代（指向旧主），释放本侧端口
			cancel()
			return runWorker(cfg, newMaster)
		}
		logrus.Info("[SerialHub] 确认无主实例，晋升为主实例")
		stopProxy() // 释放本侧端口给晋升后的主服务，避免 EADDRINUSE
		cancel()    // 停止旧联邦客户端
		return runPromotedMaster(cfg, sm)
	}
}

// runPromotedMaster 晋升：复用从实例的串口管理器，启动完整主服务（不弹浏览器）。
func runPromotedMaster(cfg *config.Config, sm *serial.SerialManager) error {
	buf := buffer.NewDataBuffer()
	return runWithoutTray(cfg, sm, buf, false)
}

// startWorkerProxy 在从实例本侧监听 host:mcpPort：/mcp 反代到主实例，
// /health 本地应答（role=worker，供实例发现区分主从），其余路径提示走主侧。
// 返回幂等的停止函数（等待反代完全退出后返回），供换主/晋升前释放端口；
// 监听失败（同侧主实例已占端口）仅告警并返回 no-op（降级为仅贡献串口）。
func startWorkerProxy(masterURL string) (stop func()) {
	noop := func() {}
	target, err := url.Parse(masterURL)
	if err != nil {
		logrus.Warnf("[SerialHub] 反代目标地址无效: %v", err)
		return noop
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","role":"worker"}`))
	})
	mux.Handle("/mcp", proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "从实例仅提供 /mcp 与 /health，xterm web 请访问主实例", http.StatusNotFound)
	})

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logrus.Warnf("[SerialHub] 从实例反代监听 %s 失败（同侧端口被占用？），跳过反代，仅贡献串口: %v", addr, err)
		return noop
	}

	srv := &http.Server{Handler: mux}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logrus.Warnf("[SerialHub] 从实例反代退出: %v", err)
		}
	}()
	logrus.Infof("[SerialHub] 从实例反代已启动: http://%s（/mcp → 主实例）", ln.Addr())

	var once sync.Once
	return func() {
		once.Do(func() {
			srv.Close() // 关闭 listener 与活跃连接，Serve 返回 ErrServerClosed
			<-done      // 等待 Serve goroutine 完全退出，端口确定释放
		})
	}
}

// runStdio stdio 模式：发现主实例则作透明代理（RunStdioProxy）；
// 否则本进程成为主实例（完整服务面，但不弹浏览器、无托盘），
// 随 stdio 连接断开而整体退出（MCP local 惯例：客户端管理进程生命周期）。
func runStdio(cfg *config.Config) error {
	masterURL := federation.DiscoverMaster(mcpPort)
	if masterURL != "" {
		return mcp.RunStdioProxy(context.Background(), masterURL)
	}

	logrus.Info("[SerialHub] stdio 模式：未发现主实例，本进程成为主实例")

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

	mcpSrv := startServices(sm, wsSrv, buf, false)
	if mcpSrv == nil {
		return fmt.Errorf("主服务启动失败")
	}

	if err := mcpSrv.RunStdioTransport(ctx); err != nil {
		logrus.Debugf("[SerialHub] stdio 传输结束: %v", err)
	}
	logrus.Info("[SerialHub] stdio 连接断开，进程退出")
	return nil
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

// startServices 启动主实例服务面：数据桥、MCP HTTP（含联邦端点 /federation）。
// 返回 MCPServer（stdio 模式需叠跑 stdio 传输）；启动失败返回 nil。
func startServices(sm *serial.SerialManager, wsSrv *web.WebSocketServer, buf *buffer.DataBuffer, autoOpenBrowser bool) *mcp.MCPServer {
	bridgeSrv, err := bridge.NewDataBridge(sm, wsSrv, buf)
	if err != nil {
		logrus.Warnf("[SerialHub] 创建数据桥接失败: %v", err)
	} else {
		bridgeSrv.SetCommandHandler(createCommandHandler(sm))
		bridgeSrv.Start()
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

	// 联邦主侧：聚合从实例端口；从侧上行数据注入双通道（与 bridge 等价）
	fedMgr := federation.NewManager(federation.LocalSide())
	fedMgr.SetDataHandler(func(portKey string, data []byte) {
		// 仅注入当前活动联邦端口的数据；从侧其他连接（如自留口）不上行混流
		if fedMgr.ActiveFederatedPort() != portKey {
			return
		}
		data = bridge.ConvertLFToCRLF(data)
		wsSrv.Broadcast(data)
		buf.Append(data)
	})
	mcpSrv.SetFederation(fedMgr, http.HandlerFunc(fedMgr.HandleWS))

	addr := fmt.Sprintf("%s:%d", host, mcpPort)
	if _, err := mcpSrv.StartHTTPServer(addr, autoOpenBrowser); err != nil {
		logrus.Errorf("[SerialHub] 启动 HTTP 服务失败: %v", err)
		return nil
	}
	return mcpSrv
}
