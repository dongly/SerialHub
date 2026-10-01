// Package serial manages serial port connections.
package serial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	serial "go.bug.st/serial"
	"go.bug.st/serial/enumerator"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/internal/logagg"
	"github.com/sirupsen/logrus"
)

const (
	// 意外断开自动重连：1s 起指数退避，封顶 30s，最多尝试 30 次。
	// 覆盖 usbipd detach/attach、USB 重新插拔（含端口改名）等场景。
	defaultReconnectBaseDelay   = 1 * time.Second
	defaultReconnectMaxDelay    = 30 * time.Second
	defaultReconnectMaxAttempts = 30
)

// usbPortDetails 抽象 enumerator.PortDetails，便于测试注入假 USB 身份。
type usbPortDetails interface {
	Name() string
	IsUSB() bool
	VID() string
	PID() string
}

// EventType 定义串口事件类型
type EventType string

const (
	EventConnected    EventType = "connected"
	EventDisconnected EventType = "disconnected"
	EventError        EventType = "error"
)

// Event 表示串口状态事件
type Event struct {
	Type EventType
	Port string
	// Message 是诊断文本（中文），只供日志与测试使用，不随 SERIALHUB_LANG
	// 切换；面向 Web 终端的提示走语言无关的事件码（见 webSerialEvent）。
	Message string
}

// EventHandler 是串口事件处理函数类型
type EventHandler func(event Event)

// Port defines the serial port interface for dependency injection
type Port interface {
	io.Reader
	io.Writer
	io.Closer
	SetReadTimeout(timeout time.Duration) error
}

// SerialManager manages serial port connections
type SerialManager struct {
	config              *Config
	port                Port
	dataChan            chan []byte
	errChan             chan error
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	logger              *logrus.Logger
	eventHandler        EventHandler
	configChangeHandler func(*Config)

	// USB 身份与自动重连：连接成功时记录 VID/PID，
	// 意外断开后优先按原名重连，端口改名时按 VID/PID 匹配新端口。
	portVID string
	portPID string
	// reconnectGen 重连代次：用户主动干预（连接/断开/改配置）或新断开
	// 发生时递增，使挂起的旧重连任务失效（任一时刻至多一个有效任务）。
	reconnectGen uint64
	// closed 管理器已关闭：置位后拒绝新连接，Close 与 Connect 在锁内
	// 互斥检查，避免 Close 的 wg.Wait 之后仍有 Connect 发布 readLoop。
	closed bool
	// 以下字段为依赖注入点（测试替换假实现），生产路径用默认值。
	openPort             func(name string, mode *serial.Mode) (Port, error)
	listDetailedPorts    func() []usbPortDetails
	listPortsFn          func() ([]string, error)
	reconnectBaseDelay   time.Duration
	reconnectMaxAttempts int
}

// NewSerialManager creates a new serial manager
func NewSerialManager(cfg *Config) (*SerialManager, error) {
	if cfg == nil {
		return nil, errors.New(i18n.Serial.ConfigNil)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SerialManager{
		config:   cfg.Clone(),
		dataChan: make(chan []byte, 256),
		errChan:  make(chan error, 16),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logrus.New(),
		openPort: func(name string, mode *serial.Mode) (Port, error) {
			return serial.Open(name, mode)
		},
		listDetailedPorts:    listRealDetailedPorts,
		listPortsFn:          serial.GetPortsList,
		reconnectBaseDelay:   defaultReconnectBaseDelay,
		reconnectMaxAttempts: defaultReconnectMaxAttempts,
	}, nil
}

// enumeratorPortDetails 适配 *enumerator.PortDetails（字段非方法）到 usbPortDetails。
type enumeratorPortDetails struct{ d *enumerator.PortDetails }

func (w enumeratorPortDetails) Name() string { return w.d.Name }
func (w enumeratorPortDetails) IsUSB() bool  { return w.d.IsUSB }
func (w enumeratorPortDetails) VID() string  { return w.d.VID }
func (w enumeratorPortDetails) PID() string  { return w.d.PID }

// listRealDetailedPorts 枚举系统串口的 USB 详细信息（VID/PID）。
// 枚举失败或无 USB 信息时返回 nil，调用方按"无 VID/PID"降级处理。
func listRealDetailedPorts() []usbPortDetails {
	details, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil
	}
	out := make([]usbPortDetails, 0, len(details))
	for _, d := range details {
		out = append(out, enumeratorPortDetails{d})
	}
	return out
}

// errReconnectCancelled 重连任务因用户干预（连接/断开/改配置）或管理器关闭而失效。
var errReconnectCancelled = errors.New(i18n.Serial.ReconnectCancelled)

// Connect connects to the serial port
func (sm *SerialManager) Connect() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.connectLocked()
}

// connectLocked 连接主体（调用方需持有 sm.mu 写锁）。
func (sm *SerialManager) connectLocked() error {
	if sm.closed {
		return errors.New(i18n.Serial.ManagerClosed)
	}

	if sm.port != nil {
		return fmt.Errorf(i18n.Serial.AlreadyConnected, sm.config.Port)
	}

	logrus.Debugf("[SerialHub] 尝试连接串口: %s", sm.config.String())

	mode, err := sm.config.ToMode()
	if err != nil {
		logrus.Errorf("[SerialHub] 串口配置转换失败: %v", err)
		return fmt.Errorf(i18n.Serial.ConvertConfigFailed, err)
	}

	logrus.Debugf("[SerialHub] 串口模式: BaudRate=%d, DataBits=%d, Parity=%v, StopBits=%v",
		mode.BaudRate, mode.DataBits, mode.Parity, mode.StopBits)

	port, err := sm.openPort(sm.config.Port, mode)
	if err != nil {
		logrus.Errorf("[SerialHub] 打开串口失败 %s: %v", sm.config.Port, err)
		return fmt.Errorf(i18n.Serial.OpenFailed, err)
	}

	sm.port = port
	// 记录 USB 身份（VID/PID），供意外断开后匹配改名端口；非 USB 串口为空
	sm.portVID, sm.portPID = sm.lookupPortVIDPID(sm.config.Port)

	if err := port.SetReadTimeout(500 * time.Millisecond); err != nil {
		logrus.Warnf("[SerialHub] 设置读超时失败: %v", err)
	}

	logrus.Infof("[SerialHub] 串口已连接: %s", sm.config.String())

	sm.wg.Add(1)
	go sm.readLoop(port)

	sm.emitEvent(Event{
		Type: EventConnected,
		Port: sm.config.Port,
	})

	return nil
}

// ConnectionGen 返回当前连接代次：连接/断开/改配置或意外断开均会递增。
// 调用方（如 serial_script）可在启动时记录代次并周期比较，
// 检测"断连后快速重连"这类瞬态，避免旧任务继续写向新连接。
func (sm *SerialManager) ConnectionGen() uint64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.reconnectGen
}

// Disconnect disconnects from the serial port
// 幂等：未连接时同样使挂起的自动重连失效并返回成功（用户意图是"确保断开"）。
// 端口关闭在锁外异步执行：USB 掉线（usbipd detach）时驱动 Close 可能
// 长时间阻塞，不能让它卡住锁或调用方；关闭失败仅记日志。
func (sm *SerialManager) Disconnect() error {
	sm.mu.Lock()
	// 用户主动断开：递增代次，取消一切挂起的自动重连
	sm.reconnectGen++
	port := sm.port
	if port == nil {
		sm.mu.Unlock()
		return nil
	}
	portName := sm.config.Port
	sm.port = nil
	sm.portVID, sm.portPID = "", ""
	sm.mu.Unlock()

	go func() {
		if err := port.Close(); err != nil {
			logrus.Debugf("[SerialHub] 关闭串口 %s 出错: %v", portName, err)
		}
	}()

	logrus.Infof("[SerialHub] 串口已断开: %s", portName)

	sm.emitEvent(Event{
		Type: EventDisconnected,
		Port: portName,
	})

	return nil
}

// ErrStaleConnection 表示写入时连接代次已变（期间发生过断连/重连/改配置）。
// 供 WriteIfSameGen 返回，调用方以 errors.Is 判定"不得再写向当前连接"。
var ErrStaleConnection = errors.New("连接代次已变")

// Write writes data to the serial port
func (sm *SerialManager) Write(data []byte) (int, error) {
	sm.mu.RLock()
	port := sm.port
	if port == nil {
		sm.mu.RUnlock()
		return 0, errors.New(i18n.Serial.NotConnected)
	}
	n, err := sm.writeUnderLock(port, data)
	sm.mu.RUnlock()
	return sm.finishWrite(n, err, data)
}

// WriteIfSameGen 仅在连接代次仍等于 gen 时写入，否则返回 ErrStaleConnection。
// 校验与写入处于同一读锁临界区：与 Disconnect/Connect 的代次递增互斥，
// 消除"检查后、写入前连接被替换"的竞态（serial_script 断连语义依赖此保证）。
func (sm *SerialManager) WriteIfSameGen(gen uint64, data []byte) (int, error) {
	sm.mu.RLock()
	port := sm.port
	if port == nil || sm.reconnectGen != gen {
		sm.mu.RUnlock()
		return 0, ErrStaleConnection
	}
	n, err := sm.writeUnderLock(port, data)
	sm.mu.RUnlock()
	return sm.finishWrite(n, err, data)
}

// writeUnderLock 实际写入 + 错误投递；调用方须持有 sm.mu.RLock。
// 错误投递必须与 Close 的通道关闭互斥：Close 在写锁内关闭 errChan，
// 此处持读锁完成非阻塞发送（select+default 不会阻塞）；若在锁外
// 发送，存在 send on closed channel 的 panic 窗口。
func (sm *SerialManager) writeUnderLock(port Port, data []byte) (int, error) {
	n, err := port.Write(data)
	if err != nil {
		select {
		case sm.errChan <- fmt.Errorf(i18n.Serial.WriteError, err):
		default:
			// 通道满时静默丢弃：finishWrite 仍会记录本次错误
		}
	}
	return n, err
}

// finishWrite 锁外的数据日志与错误包装。
func (sm *SerialManager) finishWrite(n int, err error, data []byte) (int, error) {
	// 数据内容日志（聚合）在锁外输出，避免日志 I/O 拖住连接管理
	logagg.Add(logagg.TagSerialWrite, data)
	if err != nil {
		logrus.Errorf("[SerialHub] 串口写入失败: %v", err)
		return n, fmt.Errorf(i18n.Serial.WriteFailed, err)
	}
	return n, nil
}

// WriteLine writes a line of data to the serial port (adds newline automatically)
func (sm *SerialManager) WriteLine(line string) error {
	data := []byte(line + "\n")
	_, err := sm.Write(data)
	return err
}

// ListPorts lists all available serial ports
func (sm *SerialManager) ListPorts() ([]string, error) {
	ports, err := sm.listPortsFn()
	if err != nil {
		return nil, fmt.Errorf(i18n.Serial.ListFailed, err)
	}
	// WSL 环境下 hypervisor 会注入打不开的假串口（ttyS0~ttyS4 等），
	// 枚举时只保留真实 USB/ACM 串口设备，避免假端口混入连接目标。
	if isWSL() {
		filtered := make([]string, 0, len(ports))
		for _, p := range ports {
			base := p
			if i := strings.LastIndexByte(p, '/'); i >= 0 {
				base = p[i+1:]
			}
			if strings.HasPrefix(base, "ttyUSB") || strings.HasPrefix(base, "ttyACM") {
				filtered = append(filtered, p)
			}
		}
		ports = filtered
	}
	return ports, nil
}

// isWSL 判断当前 Linux 是否运行在 WSL 下（/proc/version 含 microsoft 标记）。
func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if v := os.Getenv("WSL_DISTRO_NAME"); v != "" {
		return true
	}
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// IsConnected checks if the serial port is connected
func (sm *SerialManager) IsConnected() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.port != nil
}

// CurrentPort returns the current connected port name
func (sm *SerialManager) CurrentPort() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.port == nil {
		return ""
	}
	return sm.config.Port
}

// GetConfig returns a copy of the current configuration
func (sm *SerialManager) GetConfig() *Config {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config.Clone()
}

// UpdateConfig updates the configuration (must be called when not connected)
// 用户显式改配置：递增重连代次，使挂起的自动重连失效（用户已更换目标）。
func (sm *SerialManager) UpdateConfig(cfg *Config) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.reconnectGen++
	return sm.updateConfigLocked(cfg)
}

// updateConfigLocked 配置更新主体（调用方需持有 sm.mu 写锁）。
// 重连任务内部改名同步走此路径，不递增代次（那是重连自身的合法操作）。
func (sm *SerialManager) updateConfigLocked(cfg *Config) error {
	if sm.port != nil {
		return errors.New(i18n.Serial.UpdateWhileConnected)
	}

	if cfg == nil {
		return errors.New(i18n.Serial.ConfigNil)
	}
	if cfg.Port == "" {
		return errors.New(i18n.Serial.PortEmpty)
	}

	sm.config = cfg.Clone()

	// 触发配置变更回调
	if sm.configChangeHandler != nil {
		sm.configChangeHandler(sm.config.Clone())
	}

	return nil
}

// SetConfigChangeHandler 设置配置变更处理函数
func (sm *SerialManager) SetConfigChangeHandler(handler func(*Config)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.configChangeHandler = handler
}

// SetEventHandler 设置串口事件处理函数
func (sm *SerialManager) SetEventHandler(handler EventHandler) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.eventHandler = handler
}

// emitEvent 触发事件
// 注意：调用者可能持有 sm.mu 写锁，因此这里不能再获取锁。
// eventHandler 在启动时一次性设置，之后只读，无需加锁。
func (sm *SerialManager) emitEvent(event Event) {
	handler := sm.eventHandler
	if handler == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[SerialHub] 事件处理 panic: %v", r)
			}
		}()
		handler(event)
	}()
}
func (sm *SerialManager) DataChan() <-chan []byte {
	return sm.dataChan
}

// ErrChan returns the error channel
func (sm *SerialManager) ErrChan() <-chan error {
	return sm.errChan
}

// Close closes the manager and cleans up resources
// 顺序：锁内先置 closed 并摘除端口（异步 Close 唤醒阻塞中的 Read），
// 再等读取循环退出。closed 与 Connect 在锁内互斥，保证 wg.Wait 之后
// 不会再有新的 readLoop 发布（WaitGroup 复用 panic）。
// 若驱动 Close/Read 均卡死（usbipd 极端场景），wg.Wait 会无限等待，
// 由上层调用方的整体限时兜底（见 serve.go gracefulShutdown）。
func (sm *SerialManager) Close() error {
	sm.mu.Lock()
	if sm.closed {
		sm.mu.Unlock()
		return nil
	}
	sm.closed = true
	sm.cancel()
	port := sm.port
	sm.port = nil
	sm.portVID, sm.portPID = "", ""
	sm.mu.Unlock()

	if port != nil {
		// 异步关闭，唤醒阻塞在 Read 上的读取循环；进程退出时由 OS 回收
		go func() {
			if err := port.Close(); err != nil {
				logrus.Debugf("[SerialHub] 关闭串口出错: %v", err)
			}
		}()
	}

	sm.wg.Wait()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Debug("[SerialHub] dataChan 已关闭或关闭时发生错误")
			}
		}()
		close(sm.dataChan)
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Debug("[SerialHub] errChan 已关闭或关闭时发生错误")
			}
		}()
		close(sm.errChan)
	}()

	return nil
}

// readLoop 串口读取循环：固定服务于启动时传入的端口实例。
// 端口被摘除（主动断开/意外断开/Close）后本循环退出，绝不切换到
// sm.port 上的后续新连接——否则断开后重连期间会短暂出现新旧两个
// 循环同时读同一串口。发布数据前同样校验归属：慢速 Read 返回期间
// 端口可能已被替换，旧连接的在途数据不得发布到新连接。
func (sm *SerialManager) readLoop(port Port) {
	defer sm.wg.Done()
	if port == nil {
		return
	}
	buf := make([]byte, 1024)
	logrus.Debug("[SerialHub] 串口读取循环已启动")

	// 用于保存不完整的 UTF-8 字符尾部
	var incompleteBuf []byte

	for {
		select {
		case <-sm.ctx.Done():
			logrus.Debug("[SerialHub] 串口读取循环已停止")
			return
		default:
			// 归属校验：端口已被摘除（断开/意外断开/关闭）即退出，
			// 不再发起下一次 Read——驱动 Close 生效前 Read 可能仍正常空转
			sm.mu.RLock()
			current := sm.port
			sm.mu.RUnlock()
			if current != port {
				logrus.Debug("[SerialHub] 端口已被替换/摘除，读取循环退出")
				return
			}

			n, err := port.Read(buf)
			if err != nil {
				sm.mu.RLock()
				current := sm.port
				sm.mu.RUnlock()
				if current != port {
					// 连接已被主动断开/关闭（Disconnect/Close 已清理状态），静默退出
					logrus.Debug("[SerialHub] 串口已被主动关闭，读取循环退出")
					return
				}
				// 意外断开（USB 掉线/对方关闭）：清状态、广播、自动重连
				sm.handleUnexpectedDisconnect(port, err)
				return
			}

			if n > 0 {
				// 发布前校验归属：Read 阻塞期间端口可能已被替换为新连接
				sm.mu.RLock()
				current := sm.port
				sm.mu.RUnlock()
				if current != port {
					logrus.Debug("[SerialHub] 端口已切换，旧读取循环退出（丢弃在途数据）")
					return
				}

				// 合并之前不完整的 UTF-8 字符
				data := append(incompleteBuf, buf[:n]...)
				incompleteBuf = nil

				// 检查最后一个字符是否完整的 UTF-8
				if len(data) > 0 {
					_, size := utf8.DecodeLastRune(data)
					if size == 0 || (size == 1 && data[len(data)-1] >= 0x80) {
						// 最后一个字符不完整，找到 UTF-8 序列的起始位置
						for i := len(data) - 1; i >= 0; i-- {
							// UTF-8 continuation byte: 10xxxxxx (0x80-0xBF)
							// UTF-8 start byte: 0xxxxxx (0x00-0x7F) or 11xxxxxx (0xC0-0xFF)
							if data[i] < 0x80 || data[i] >= 0xC0 {
								// 找到起始字节，从这里开始都是不完整的
								incompleteBuf = data[i:]
								data = data[:i]
								break
							}
							if i == 0 {
								// 整个 buffer 都是 continuation bytes
								incompleteBuf = data
								data = nil
							}
						}
					}
				}

				if len(data) > 0 {
					// 数据内容日志：记录实际读到的字节（与入队归属无关），聚合输出
					logagg.Add(logagg.TagSerialRead, data)
					// 持锁完成"归属校验 + 非阻塞入队"：校验后解锁再发送的话，
					// 间隙里端口可能被切换，旧连接的在途数据会混入新连接的流。
					// select 带 default 不会阻塞；丢弃日志在锁外输出，避免文件 I/O 拖住 Disconnect/Connect。
					dropped := false
					sm.mu.RLock()
					if sm.port != port {
						sm.mu.RUnlock()
						logrus.Debug("[SerialHub] 端口已切换，丢弃旧连接在途数据")
						return
					}
					select {
					case sm.dataChan <- data:
					case <-sm.ctx.Done():
						sm.mu.RUnlock()
						return
					default:
						dropped = true
					}
					sm.mu.RUnlock()
					if dropped {
						logrus.Warnln("[SerialHub] 数据通道已满，丢弃数据")
					}
				}
			}
		}
	}
}

// handleUnexpectedDisconnect 处理意外断开（EOF/读错误，且未被主动关闭）：
// 1. 立即清理连接状态（修复"假连接"：port 不清理导致 IsConnected 恒真、无法重连）；
// 2. 广播断开事件（EOF 视为断开，其余视为错误）；
// 3. 启动自动重连循环。
func (sm *SerialManager) handleUnexpectedDisconnect(port Port, cause error) {
	sm.mu.Lock()
	if sm.port != port { // 双检：与主动断开竞争时放弃
		sm.mu.Unlock()
		return
	}
	sm.port = nil
	portName := sm.config.Port
	vid, pid := sm.portVID, sm.portPID
	sm.portVID, sm.portPID = "", ""
	// 作废此前挂起的重连任务：任一时刻至多一个有效（连续掉线场景）
	sm.reconnectGen++
	gen := sm.reconnectGen
	sm.mu.Unlock()

	// 异步关闭失效端口：USB 掉线（usbipd detach）时 Close 可能阻塞，
	// 不能让它卡住锁或读取循环
	go func() {
		if err := port.Close(); err != nil {
			logrus.Debugf("[SerialHub] 关闭失效串口 %s 出错: %v", portName, err)
		}
	}()

	// 诊断文本只进 logrus 与 errChan，保持中文，不随 SERIALHUB_LANG 切换
	reason := "连接已关闭 (EOF)"
	evType := EventDisconnected
	if cause != io.EOF {
		reason = fmt.Sprintf("读取错误: %v", cause)
		evType = EventError
	}
	logrus.Warnf("[SerialHub] 串口 %s 意外断开: %s", portName, reason)

	// 非阻塞投递诊断错误：errChan 无持续消费方时不能卡住断开处理
	//（否则事件广播与自动重连都被背压阻塞）
	errMsg := fmt.Errorf("串口 %s %s", portName, reason)
	select {
	case sm.errChan <- errMsg:
	default:
		logrus.Debug("[SerialHub] 错误通道已满，丢弃断开诊断错误")
	}

	sm.emitEvent(Event{
		Type:    evType,
		Port:    portName,
		Message: reason + "，将自动重连",
	})

	if sm.ctx.Err() == nil {
		go sm.reconnectLoop(portName, vid, pid, gen)
	}
}

// reconnectLoop 意外断开后的自动重连：指数退避（base 起、maxDelay 封顶），
// 最多 maxAttempts 次。目标端口优先原名，端口消失时按 USB VID/PID 匹配
// 改名后的新端口（应对 usbipd 重新 attach 后 ttyUSB0→ttyUSB1 改名）。
// gen 是任务启动时的重连代次：用户主动干预（连接/断开/改配置）、
// 管理器关闭或发生新的掉线都会使代次递增，旧任务随即退出；
// 代次校验与连接动作在同一锁临界区内完成，杜绝"校验后被插队"的窗口。
func (sm *SerialManager) reconnectLoop(portName, vid, pid string, gen uint64) {
	delay := sm.reconnectBaseDelay
	for attempt := 1; attempt <= sm.reconnectMaxAttempts; attempt++ {
		timer := time.NewTimer(delay)
		select {
		case <-sm.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay *= 2
		if delay > defaultReconnectMaxDelay {
			delay = defaultReconnectMaxDelay
		}

		// 代次已失效（用户干预/关闭/已被其他途径恢复）→ 退出
		if !sm.reconnectActive(gen) {
			logrus.Debug("[SerialHub] 重连任务已失效（用户干预或管理器关闭），退出")
			return
		}

		ports, err := sm.ListPorts()
		if err != nil {
			logrus.Warnf("[SerialHub] 自动重连 %d/%d：获取串口列表失败: %v", attempt, sm.reconnectMaxAttempts, err)
			continue
		}
		target := matchReconnectPort(ports, sm.listDetailedPorts(), portName, vid, pid)
		if target == "" {
			logrus.Warnf("[SerialHub] 自动重连 %d/%d：未找到端口 %s（VID=%s PID=%s），继续等待...",
				attempt, sm.reconnectMaxAttempts, portName, vid, pid)
			continue
		}
		// 目标与当前配置不一致时同步（按当前配置而非任务初始名判断：
		// 之前拍次可能已把配置改成别的端口，例如改名目标打开失败后原名恢复）
		if cfg := sm.GetConfig(); cfg.Port != target {
			logrus.Infof("[SerialHub] 重连目标 %s 与当前配置 %s 不一致，同步配置", target, cfg.Port)
			cfg.Port = target
			if !sm.reconnectUpdateConfig(gen, cfg) {
				logrus.Debug("[SerialHub] 重连任务在更新配置时失效，退出")
				return
			}
		}

		if err := sm.reconnectConnect(gen); err != nil {
			if errors.Is(err, errReconnectCancelled) {
				logrus.Debug("[SerialHub] 重连任务在连接时失效，退出")
				return
			}
			logrus.Warnf("[SerialHub] 自动重连 %d/%d 失败: %v", attempt, sm.reconnectMaxAttempts, err)
			continue
		}
		logrus.Infof("[SerialHub] 串口 %s 自动重连成功（第 %d 次尝试）", target, attempt)
		return
	}

	// 最终放弃前再校验代次：最后一次尝试期间用户可能已干预（连接/断开/改配置），
	// 失效的任务不应再广播"自动重连失败"误导界面
	if !sm.reconnectActive(gen) {
		logrus.Debug("[SerialHub] 重连任务在放弃前已失效，跳过失败广播")
		return
	}
	logrus.Errorf("[SerialHub] 自动重连放弃：端口 %s 在 %d 次尝试后仍未恢复，请检查 USB 连接（usbipd attach）后手动重连",
		portName, sm.reconnectMaxAttempts)
	sm.emitEvent(Event{
		Type:    EventError,
		Port:    portName,
		Message: "自动重连失败：端口未恢复，请手动重连",
	})
}

// reconnectActive 重连任务的存续检查：代次未失效、管理器未关闭、
// 当前确实未连接（已被其他途径恢复时同样让位）。
func (sm *SerialManager) reconnectActive(gen uint64) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.reconnectGen == gen && !sm.closed && sm.port == nil
}

// reconnectConnect 重连专用连接入口：锁内校验代次后连接，
// 与 connectLocked 在同一临界区，消除校验与连接之间的插队窗口。
func (sm *SerialManager) reconnectConnect(gen uint64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.reconnectGen != gen || sm.closed || sm.port != nil {
		return errReconnectCancelled
	}
	return sm.connectLocked()
}

// reconnectUpdateConfig 重连专用配置更新（端口改名时同步配置）：
// 锁内校验代次，失效则返回 false；不递增代次（重连自身的合法操作）。
func (sm *SerialManager) reconnectUpdateConfig(gen uint64, cfg *Config) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.reconnectGen != gen || sm.closed || sm.port != nil {
		return false
	}
	if err := sm.updateConfigLocked(cfg); err != nil {
		logrus.Warnf("[SerialHub] 自动重连更新端口配置失败: %v", err)
		return false
	}
	return true
}

// matchReconnectPort 从可用端口中解析重连目标：优先原名；
// 原名消失时按 USB VID/PID 匹配（仅在可用列表内的才算命中）。
// 纯函数，便于测试。
func matchReconnectPort(ports []string, details []usbPortDetails, portName, vid, pid string) string {
	for _, p := range ports {
		if p == portName {
			return portName
		}
	}
	if vid == "" || pid == "" {
		return ""
	}
	for _, d := range details {
		if d.IsUSB() && d.VID() == vid && d.PID() == pid {
			for _, p := range ports {
				if p == d.Name() {
					return d.Name()
				}
			}
		}
	}
	return ""
}

// lookupPortVIDPID 查指定端口名的 USB VID/PID；找不到或非 USB 时返回空串。
// 调用方需持有 sm.mu（Connect 内使用）。
func (sm *SerialManager) lookupPortVIDPID(name string) (vid, pid string) {
	for _, d := range sm.listDetailedPorts() {
		if d.IsUSB() && d.Name() == name {
			return d.VID(), d.PID()
		}
	}
	return "", ""
}
