package tray

import (
	"context"
	"embed"
	"fmt"
	"strconv"

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

type TrayManager struct {
	serial         *serial.SerialManager
	config         *config.Config
	telnetPort     int
	mcpPort        int
	version        string
	state          TrayState
	quitChan       chan struct{}
	mSerial        *systray.MenuItem
	mSelectPort    *systray.MenuItem
	mRefresh       *systray.MenuItem
	mPortItems     map[string]*systray.MenuItem
	mSerialConfig  *systray.MenuItem
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

func (t *TrayManager) Run(ctx context.Context) {
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
}

func (t *TrayManager) createMenu() {
	t.mSerial = systray.AddMenuItem(t.getSerialMenuTitle(), "串口连接")

	t.mSelectPort = systray.AddMenuItem("选择串口", "选择串口")
	t.mRefresh = t.mSelectPort.AddSubMenuItem("刷新列表", "刷新串口列表")
	systray.AddSeparator()

	t.mSerialConfig = systray.AddMenuItem("串口参数", "串口参数")

	t.mBaudRate = t.mSerialConfig.AddSubMenuItem("波特率", "选择波特率")
	for _, rate := range baudRates {
		item := t.mBaudRate.AddSubMenuItem(strconv.Itoa(rate), fmt.Sprintf("波特率 %d", rate))
		t.mBaudRateItems[rate] = item
	}

	t.mDataBits = t.mSerialConfig.AddSubMenuItem("数据位", "选择数据位")
	for _, bits := range dataBitsList {
		item := t.mDataBits.AddSubMenuItem(strconv.Itoa(bits), fmt.Sprintf("数据位 %d", bits))
		t.mDataBitsItems[bits] = item
	}

	t.mStopBits = t.mSerialConfig.AddSubMenuItem("停止位", "选择停止位")
	for _, bits := range stopBitsList {
		label := fmt.Sprintf("%.0f", bits)
		if bits == 1.5 {
			label = "1.5"
		}
		item := t.mStopBits.AddSubMenuItem(label, fmt.Sprintf("停止位 %s", label))
		t.mStopBitsItems[bits] = item
	}

	t.mParity = t.mSerialConfig.AddSubMenuItem("校验位", "选择校验位")
	for _, p := range parityList {
		item := t.mParity.AddSubMenuItem(p, fmt.Sprintf("校验位 %s", p))
		t.mParityItems[p] = item
	}

	t.mCurrentConfig = t.mSerialConfig.AddSubMenuItem(t.getConfigSummary(), "当前配置")
	t.mCurrentConfig.Disable()

	systray.AddSeparator()

	t.mNetworkStatus = systray.AddMenuItem(t.getNetworkStatus(), "网络状态")
	t.mNetworkStatus.Disable()

	systray.AddSeparator()

	t.mShowLog = systray.AddMenuItem("显示日志", "显示日志窗口")

	systray.AddSeparator()

	mVersion := systray.AddMenuItem(fmt.Sprintf("版本 %s", t.version), "版本")
	mVersion.Disable()

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("退出", "退出 SerialHub")

	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()
}

func (t *TrayManager) setupEventHandlers() {
	go t.handleRefresh()
	go t.handleSerialToggle()
	go t.handleShowLog()
	go t.handleBaudRateSelection()
	go t.handleDataBitsSelection()
	go t.handleStopBitsSelection()
	go t.handleParitySelection()
	go t.handlePortSelection()
}

func (t *TrayManager) handleRefresh() {
	for range t.mRefresh.ClickedCh {
		t.refreshPortList()
	}
}

func (t *TrayManager) handleSerialToggle() {
	for range t.mSerial.ClickedCh {
		t.toggleSerial()
	}
}

func (t *TrayManager) handleShowLog() {
	for range t.mShowLog.ClickedCh {
		ShowConsole()
	}
}

func (t *TrayManager) handleBaudRateSelection() {
	for rate, item := range t.mBaudRateItems {
		go func(r int, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setBaudRate(r)
			}
		}(rate, item)
	}
}

func (t *TrayManager) handleDataBitsSelection() {
	for bits, item := range t.mDataBitsItems {
		go func(b int, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setDataBits(b)
			}
		}(bits, item)
	}
}

func (t *TrayManager) handleStopBitsSelection() {
	for bits, item := range t.mStopBitsItems {
		go func(b float64, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setStopBits(b)
			}
		}(bits, item)
	}
}

func (t *TrayManager) handleParitySelection() {
	for parity, item := range t.mParityItems {
		go func(p string, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setParity(p)
			}
		}(parity, item)
	}
}

func (t *TrayManager) handlePortSelection() {
	for port, item := range t.mPortItems {
		go func(p string, i *systray.MenuItem) {
			for range i.ClickedCh {
				t.setPort(p)
			}
		}(port, item)
	}
}

func (t *TrayManager) refreshPortList() {
	ports, err := t.serial.ListPorts()
	if err != nil {
		logrus.Warnf("[SerialHub] 获取串口列表失败: %v", err)
		return
	}

	for _, item := range t.mPortItems {
		item.Hide()
	}
	t.mPortItems = make(map[string]*systray.MenuItem)

	for _, port := range ports {
		item := t.mSelectPort.AddSubMenuItem(port, fmt.Sprintf("选择串口 %s", port))
		t.mPortItems[port] = item
		if port == t.config.Serial.Port {
			item.Check()
		}
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

	for p, item := range t.mPortItems {
		if p == port {
			item.Check()
		} else {
			item.Uncheck()
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

	for r, item := range t.mBaudRateItems {
		if r == rate {
			item.Check()
		} else {
			item.Uncheck()
		}
	}

	t.config.Serial.BaudRate = rate
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置波特率: %d", rate)
}

func (t *TrayManager) setDataBits(bits int) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改数据位")
		return
	}

	for b, item := range t.mDataBitsItems {
		if b == bits {
			item.Check()
		} else {
			item.Uncheck()
		}
	}

	t.config.Serial.DataBits = bits
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置数据位: %d", bits)
}

func (t *TrayManager) setStopBits(bits float64) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改停止位")
		return
	}

	for b, item := range t.mStopBitsItems {
		if b == bits {
			item.Check()
		} else {
			item.Uncheck()
		}
	}

	t.config.Serial.StopBits = int(bits)
	t.updateConfigDisplay()
	logrus.Infof("[SerialHub] 设置停止位: %.1f", bits)
}

func (t *TrayManager) setParity(parity string) {
	if t.serial.IsConnected() {
		logrus.Warn("[SerialHub] 请先断开串口连接再修改校验位")
		return
	}

	for p, item := range t.mParityItems {
		if p == parity {
			item.Check()
		} else {
			item.Uncheck()
		}
	}

	t.config.Serial.Parity = parity
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
	return fmt.Sprintf("当前: %d %d%s%d",
		t.config.Serial.BaudRate,
		t.config.Serial.DataBits,
		parity,
		t.config.Serial.StopBits)
}

func (t *TrayManager) getNetworkStatus() string {
	return fmt.Sprintf("127.0.0.1 | Telnet:%d | MCP:%d", t.telnetPort, t.mcpPort)
}

func (t *TrayManager) getSerialMenuTitle() string {
	if t.serial.IsConnected() {
		return fmt.Sprintf("断开 %s", t.serial.CurrentPort())
	}
	return fmt.Sprintf("连接 %s", t.config.Serial.Port)
}

func (t *TrayManager) onExit() {
	logrus.Info("[SerialHub] 系统托盘已退出")
	close(t.quitChan)
}

func (t *TrayManager) QuitChan() <-chan struct{} {
	return t.quitChan
}

func (t *TrayManager) UpdateState(state TrayState) {
	t.state = state
	systray.SetIcon(t.getIcon())
	logrus.Infof("[SerialHub] 托盘状态更新: %s", state)
}

func (t *TrayManager) UpdateSerialStatus() {
	if t.serial.IsConnected() {
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
		logrus.Warnf("[SerialHub] 加载图标失败: %v", err)
		return []byte{}
	}
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
