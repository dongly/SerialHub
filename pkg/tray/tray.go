package tray

import (
	"context"
	"embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"

	"github.com/yourname/serialhub/pkg/config"
	"github.com/yourname/serialhub/pkg/serial"
)

type TrayState string

const (
	TrayIdle      TrayState = "idle"
	TrayConnected TrayState = "connected"
	TrayError     TrayState = "error"
)

type OnReadyFunc func()

type TrayManager struct {
	serial         *serial.SerialManager
	config         *config.Config
	telnetPort     int
	mcpPort        int
	version        string
	state          TrayState
	quitChan       chan struct{}
	readyCallback  OnReadyFunc
	exitCallback   func()
	mSerial        *systray.MenuItem
	mSelectPort    *systray.MenuItem
	mRefresh       *systray.MenuItem
	mPortItems     map[string]*systray.MenuItem
	mBaudRate      *systray.MenuItem
	mBaudRateItems map[int]*systray.MenuItem
	mDataBits      *systray.MenuItem
	mDataBitsItems map[int]*systray.MenuItem
	mStopBits      *systray.MenuItem
	mStopBitsItems map[float64]*systray.MenuItem
	mParity        *systray.MenuItem
	mParityItems   map[string]*systray.MenuItem
	mCurrentConfig *systray.MenuItem
	mNetworkStatus *systray.MenuItem
	mShowLog       *systray.MenuItem
}

var baudRates = []int{9600, 19200, 38400, 57600, 115200, 230400}
var dataBitsList = []int{5, 6, 7, 8}
var stopBitsList = []float64{1, 1.5, 2}
var parityList = []string{"none", "even", "odd"}

//go:embed assets/*.ico
var iconFS embed.FS

func NewTrayManager(serialMgr *serial.SerialManager, cfg *config.Config, telnetPort, mcpPort int, version string) *TrayManager {
	return &TrayManager{
		serial:         serialMgr,
		config:         cfg,
		telnetPort:     telnetPort,
		mcpPort:        mcpPort,
		version:        version,
		state:          TrayIdle,
		quitChan:       make(chan struct{}),
		mPortItems:     make(map[string]*systray.MenuItem),
		mBaudRateItems: make(map[int]*systray.MenuItem),
		mDataBitsItems: make(map[int]*systray.MenuItem),
		mStopBitsItems: make(map[float64]*systray.MenuItem),
		mParityItems:   make(map[string]*systray.MenuItem),
	}
}

// SetOnReady 设置托盘就绪后的回调函数（用于启动服务）
func (t *TrayManager) SetOnReady(fn OnReadyFunc) {
	t.readyCallback = fn
}

// SetOnExit 设置托盘退出时的回调函数（用于清理服务）
func (t *TrayManager) SetOnExit(fn func()) {
	t.exitCallback = fn
}

// Run 启动系统托盘，必须在主线程调用（Windows 要求）。
// systray.Run 会阻塞直到 systray.Quit() 被调用。
func (t *TrayManager) Run(_ context.Context) {
	systray.Run(func() {
		t.onReady()
	}, func() {
		t.onExit()
	})
}

func (t *TrayManager) onReady() {
	logrus.Info("[SerialHub] 系统托盘已启动")

	systray.SetIcon(t.getIcon())
	systray.SetTitle("SerialHub")
	systray.SetTooltip(fmt.Sprintf("SerialHub v%s", t.version))

	t.createMenu()
	t.setupEventHandlers()
	t.refreshPortList()
	t.updateConfigDisplay()

	if t.readyCallback != nil {
		t.readyCallback()
	}
}

func (t *TrayManager) createMenu() {
	// 1. 串口连接/断开
	t.mSerial = systray.AddMenuItem(t.getSerialMenuTitle(), "串口连接")

	// 2. 选择串口子菜单
	t.mSelectPort = systray.AddMenuItem("选择串口 ▶", "选择串口")
	t.mRefresh = t.mSelectPort.AddSubMenuItem("刷新列表", "刷新串口列表")
	t.mSelectPort.AddSubMenuItem("──────────", "分隔线").Disable()

	// 3. 串口参数子菜单
	t.mBaudRate = systray.AddMenuItem(fmt.Sprintf("波特率: %d ▶", t.config.Serial.BaudRate), "选择波特率")
	for _, rate := range baudRates {
		label := strconv.Itoa(rate)
		if rate == t.config.Serial.BaudRate {
			label = "✓ " + label
		}
		item := t.mBaudRate.AddSubMenuItem(label, fmt.Sprintf("波特率 %d", rate))
		t.mBaudRateItems[rate] = item
	}

	t.mDataBits = systray.AddMenuItem(fmt.Sprintf("数据位: %d ▶", t.config.Serial.DataBits), "选择数据位")
	for _, bits := range dataBitsList {
		label := strconv.Itoa(bits)
		if bits == t.config.Serial.DataBits {
			label = "✓ " + label
		}
		item := t.mDataBits.AddSubMenuItem(label, fmt.Sprintf("数据位 %d", bits))
		t.mDataBitsItems[bits] = item
	}

	t.mStopBits = systray.AddMenuItem(fmt.Sprintf("停止位: %.0f ▶", float64(t.config.Serial.StopBits)), "选择停止位")
	for _, bits := range stopBitsList {
		label := fmt.Sprintf("%.0f", bits)
		if bits == 1.5 {
			label = "1.5"
		}
		if bits == t.config.Serial.StopBits {
			label = "✓ " + label
		}
		item := t.mStopBits.AddSubMenuItem(label, fmt.Sprintf("停止位 %s", label))
		t.mStopBitsItems[bits] = item
	}

	t.mParity = systray.AddMenuItem(fmt.Sprintf("校验位: %s ▶", t.config.Serial.Parity), "选择校验位")
	for _, p := range parityList {
		label := p
		if strings.EqualFold(p, t.config.Serial.Parity) {
			label = "✓ " + label
		}
		item := t.mParity.AddSubMenuItem(label, fmt.Sprintf("校验位 %s", p))
		t.mParityItems[p] = item
	}

	// 4. 当前配置显示
	t.mCurrentConfig = systray.AddMenuItem(t.getConfigSummary(), "当前配置")
	t.mCurrentConfig.Disable()

	systray.AddSeparator()

	// 5. 网络状态
	t.mNetworkStatus = systray.AddMenuItem(t.getNetworkStatus(), "网络状态")
	t.mNetworkStatus.Disable()

	systray.AddSeparator()

	// 6. 显示日志
	t.mShowLog = systray.AddMenuItem("显示日志", "显示日志窗口")

	systray.AddSeparator()

	// 7. 版本
	mVersion := systray.AddMenuItem(fmt.Sprintf("版本 %s", t.version), "版本")
	mVersion.Disable()

	systray.AddSeparator()

	// 8. 退出
	mQuit := systray.AddMenuItem("❌ 退出", "退出 SerialHub")

	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

func (t *TrayManager) setupEventHandlers() {
	// 串口连接/断开
	go func() {
		for range t.mSerial.ClickedCh {
			t.toggleSerial()
		}
	}()

	// 刷新列表
	go func() {
		for range t.mRefresh.ClickedCh {
			t.refreshPortList()
		}
	}()

	// 显示日志
	go func() {
		for range t.mShowLog.ClickedCh {
			ShowConsole()
		}
	}()

	// 波特率选择
	for rate, item := range t.mBaudRateItems {
		go func(r int, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setBaudRate(r)
			}
		}(rate, item)
	}

	// 数据位选择
	for bits, item := range t.mDataBitsItems {
		go func(b int, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setDataBits(b)
			}
		}(bits, item)
	}

	// 停止位选择
	for bits, item := range t.mStopBitsItems {
		go func(b float64, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setStopBits(b)
			}
		}(bits, item)
	}

	// 校验位选择
	for parity, item := range t.mParityItems {
		go func(p string, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setParity(p)
			}
		}(parity, item)
	}
}

func (t *TrayManager) refreshPortList() {
	ports, err := t.serial.ListPorts()
	if err != nil {
		logrus.Warnf("[SerialHub] 获取串口列表失败: %v", err)
		return
	}

	// 清除旧的串口菜单项
	for _, item := range t.mPortItems {
		item.Hide()
	}
	t.mPortItems = make(map[string]*systray.MenuItem)

	// 添加新的串口菜单项
	for _, port := range ports {
		label := port
		if port == t.config.Serial.Port {
			label = "✓ " + port
		}
		item := t.mSelectPort.AddSubMenuItem(label, fmt.Sprintf("选择串口 %s", port))
		t.mPortItems[port] = item

		go func(p string, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setPort(p)
			}
		}(port, item)
	}

	logrus.Infof("[SerialHub] 刷新串口列表: %d 个串口", len(ports))
}

func (t *TrayManager) setPort(port string) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再切换串口")
		return
	}

	// 更新选中标记
	for p, item := range t.mPortItems {
		if p == port {
			item.SetTitle("✓ " + port)
		} else {
			item.SetTitle(p)
		}
	}

	t.config.Serial.Port = port
	logrus.Infof("[SerialHub] 切换串口: %s", port)
}

func (t *TrayManager) setBaudRate(rate int) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改波特率")
		return
	}

	// 更新选中标记
	for r, item := range t.mBaudRateItems {
		if r == rate {
			item.SetTitle("✓ " + strconv.Itoa(rate))
		} else {
			item.SetTitle(strconv.Itoa(rate))
		}
	}

	t.config.Serial.BaudRate = rate
	t.mBaudRate.SetTitle(fmt.Sprintf("波特率: %d ▶", rate))
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置波特率: %d", rate)
}

func (t *TrayManager) setDataBits(bits int) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改数据位")
		return
	}

	// 更新选中标记
	for b, item := range t.mDataBitsItems {
		if b == bits {
			item.SetTitle("✓ " + strconv.Itoa(bits))
		} else {
			item.SetTitle(strconv.Itoa(bits))
		}
	}

	t.config.Serial.DataBits = bits
	t.mDataBits.SetTitle(fmt.Sprintf("数据位: %d ▶", bits))
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置数据位: %d", bits)
}

func (t *TrayManager) setStopBits(bits float64) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改停止位")
		return
	}

	// 更新选中标记
	for b, item := range t.mStopBitsItems {
		label := fmt.Sprintf("%.0f", b)
		if b == 1.5 {
			label = "1.5"
		}
		if b == bits {
			item.SetTitle("✓ " + label)
		} else {
			item.SetTitle(label)
		}
	}

	t.config.Serial.StopBits = bits
	label := fmt.Sprintf("%.0f", bits)
	if bits == 1.5 {
		label = "1.5"
	}
	t.mStopBits.SetTitle(fmt.Sprintf("停止位: %s ▶", label))
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置停止位: %s", label)
}

func (t *TrayManager) setParity(parity string) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改校验位")
		return
	}

	// 更新选中标记
	for p, item := range t.mParityItems {
		if strings.EqualFold(p, parity) {
			item.SetTitle("✓ " + p)
		} else {
			item.SetTitle(p)
		}
	}

	t.config.Serial.Parity = parity
	t.mParity.SetTitle(fmt.Sprintf("校验位: %s ▶", parity))
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置校验位: %s", parity)
}

func (t *TrayManager) updateConfigDisplay() {
	t.mCurrentConfig.SetTitle(t.getConfigSummary())
}

func (t *TrayManager) getConfigSummary() string {
	parity := t.config.Serial.Parity
	if parity == "none" {
		parity = "N"
	} else if parity == "even" {
		parity = "E"
	} else if parity == "odd" {
		parity = "O"
	}
	stopBits := fmt.Sprintf("%.0f", t.config.Serial.StopBits)
	if t.config.Serial.StopBits == 1.5 {
		stopBits = "1.5"
	}
	return fmt.Sprintf("当前: %d %d%s%s",
		t.config.Serial.BaudRate,
		t.config.Serial.DataBits,
		parity,
		stopBits)
}

func (t *TrayManager) getNetworkStatus() string {
	return fmt.Sprintf("Telnet: %d | MCP: %d", t.telnetPort, t.mcpPort)
}

func (t *TrayManager) getSerialMenuTitle() string {
	if t.serial.IsConnected() {
		return fmt.Sprintf("已连接 %s @ %d", t.serial.CurrentPort(), t.config.Serial.BaudRate)
	}
	return fmt.Sprintf("连接 %s", t.config.Serial.Port)
}

func (t *TrayManager) onExit() {
	logrus.Info("[SerialHub] 系统托盘已退出")
	if t.exitCallback != nil {
		t.exitCallback()
	}
	close(t.quitChan)
}

func (t *TrayManager) QuitChan() <-chan struct{} {
	return t.quitChan
}

func (t *TrayManager) UpdateState(state TrayState) {
	t.state = state
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[SerialHub] UpdateState panic: %v", r)
		}
	}()
	systray.SetIcon(t.getIcon())
	logrus.Infof("[SerialHub] 托盘状态更新: %s", state)
}

func (t *TrayManager) UpdateSerialStatus() {
	connected := t.serial != nil && t.serial.IsConnected()
	logrus.Debugf("[SerialHub] UpdateSerialStatus: connected=%v, serial=%v", connected, t.serial != nil)
	if connected {
		t.UpdateState(TrayConnected)
		systray.SetTooltip(fmt.Sprintf("SerialHub - 已连接 %s", t.serial.CurrentPort()))
	} else {
		t.UpdateState(TrayIdle)
		systray.SetTooltip("SerialHub - 未连接")
	}
	t.mSerial.SetTitle(t.getSerialMenuTitle())
}

func (t *TrayManager) getIcon() []byte {
	var iconPath string
	switch t.state {
	case TrayConnected:
		iconPath = "assets/tray-connected.ico"
	case TrayError:
		iconPath = "assets/tray-error.ico"
	default:
		iconPath = "assets/tray-idle.ico"
	}

	data, err := iconFS.ReadFile(iconPath)
	if err != nil {
		logrus.Errorf("[SerialHub] 加载图标失败 %s: %v", iconPath, err)
		return []byte{}
	}

	if len(data) == 0 {
		logrus.Errorf("[SerialHub] 图标数据为空: %s", iconPath)
		return []byte{}
	}

	if len(data) < 22 {
		logrus.Errorf("[SerialHub] 图标文件过小(%d bytes)，可能无效: %s", len(data), iconPath)
		return []byte{}
	}

	if data[2] != 1 || data[3] != 0 {
		logrus.Errorf("[SerialHub] 图标文件不是有效 ICO 格式: %s (header: %x)", iconPath, data[:6])
		return []byte{}
	}

	imageCount := int(data[4]) | int(data[5])<<8
	logrus.Infof("[SerialHub] 加载图标 %s: %d bytes, %d 个图像尺寸", iconPath, len(data), imageCount)
	return data
}

func (t *TrayManager) toggleSerial() {
	if t.serial.IsConnected() {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Errorf("[SerialHub] 断开串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 从托盘断开串口")
		}
	} else {
		if err := t.serial.Connect(); err != nil {
			logrus.Errorf("[SerialHub] 连接串口失败: %v", err)
		} else {
			logrus.Infof("[SerialHub] 从托盘连接串口 %s", t.config.Serial.Port)
		}
	}
	t.UpdateSerialStatus()
}
