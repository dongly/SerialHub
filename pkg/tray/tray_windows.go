//go:build windows

package tray

import (
	"context"
	"embed"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/getlantern/systray"
	"github.com/sirupsen/logrus"

	"github.com/dongly/serialhub/internal/i18n"

	"github.com/dongly/serialhub/pkg/config"
	"github.com/dongly/serialhub/pkg/serial"
)

type TrayState string

const (
	TrayIdle      TrayState = "idle"
	TrayConnected TrayState = "connected"
	TrayError     TrayState = "error"
)

type OnReadyFunc func()
type OnConfigChangedFunc func(port string, baudRate int, dataBits int, parity string, stopBits float64)

type TrayManager struct {
	serial          *serial.SerialManager
	config          *config.Config
	host            string
	mcpPort         int
	version         string
	state           TrayState
	consoleVisible  bool
	showConsoleMenu bool
	quitChan        chan struct{}
	readyCallback   OnReadyFunc
	exitCallback    func()
	onConfigChanged OnConfigChangedFunc
	mSerial         *systray.MenuItem
	mSelectPort     *systray.MenuItem
	mRefresh        *systray.MenuItem
	mPortItems      map[string]*systray.MenuItem
	mSerialConfig   *systray.MenuItem
	mBaudRate       *systray.MenuItem
	mBaudRateItems  map[int]*systray.MenuItem
	mDataBits       *systray.MenuItem
	mDataBitsItems  map[int]*systray.MenuItem
	mStopBits       *systray.MenuItem
	mStopBitsItems  map[float64]*systray.MenuItem
	mParity         *systray.MenuItem
	mParityItems    map[string]*systray.MenuItem
	mCurrentConfig  *systray.MenuItem
	mNetworkStatus  *systray.MenuItem
	mOpenTerminal   *systray.MenuItem
	mShowLog        *systray.MenuItem
}

var baudRates = []int{9600, 19200, 38400, 57600, 115200, 230400}
var dataBitsList = []int{5, 6, 7, 8}
var stopBitsList = []float64{1, 1.5, 2}
var parityList = []string{"none", "even", "odd"}

//go:embed assets/*.ico
var iconFS embed.FS

func NewTrayManager(serialMgr *serial.SerialManager, cfg *config.Config, host string, mcpPort int, version string, showConsoleMenu bool) *TrayManager {
	return &TrayManager{
		serial:          serialMgr,
		config:          cfg,
		host:            host,
		mcpPort:         mcpPort,
		version:         version,
		state:           TrayIdle,
		consoleVisible:  false,
		showConsoleMenu: showConsoleMenu,
		quitChan:        make(chan struct{}),
		mPortItems:      make(map[string]*systray.MenuItem),
		mBaudRateItems:  make(map[int]*systray.MenuItem),
		mDataBitsItems:  make(map[int]*systray.MenuItem),
		mStopBitsItems:  make(map[float64]*systray.MenuItem),
		mParityItems:    make(map[string]*systray.MenuItem),
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

// SetOnConfigChanged 设置配置变更回调（用于保存配置和自动重连）
func (t *TrayManager) SetOnConfigChanged(fn OnConfigChangedFunc) {
	t.onConfigChanged = fn
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

// Quit 请求托盘退出，服务启动失败时避免占锁但无服务。
func (t *TrayManager) Quit() {
	systray.Quit()
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
	t.mSerial = systray.AddMenuItem(t.getSerialMenuTitle(), i18n.Tray.MenuSerialTip)

	// 2. 选择串口子菜单
	t.mSelectPort = systray.AddMenuItem(i18n.Tray.MenuSelectPort, i18n.Tray.MenuSelectPortTip)
	t.mRefresh = t.mSelectPort.AddSubMenuItem(i18n.Tray.MenuRefresh, i18n.Tray.MenuRefreshTip)
	t.mSelectPort.AddSubMenuItem("──────────", i18n.Tray.SeparatorTip).Disable()

	// 3. 串口设置子菜单（连接时禁用）
	t.mSerialConfig = systray.AddMenuItem(i18n.Tray.MenuSerialConfig, i18n.Tray.MenuSerialConfigTip)

	t.mBaudRate = t.mSerialConfig.AddSubMenuItem(fmt.Sprintf(i18n.Tray.BaudRateMenu, t.config.Serial.BaudRate), i18n.Tray.BaudRateTip)
	for _, rate := range baudRates {
		label := strconv.Itoa(rate)
		if rate == t.config.Serial.BaudRate {
			label = "✓ " + label
		}
		item := t.mBaudRate.AddSubMenuItem(label, fmt.Sprintf(i18n.Tray.BaudRateItem, rate))
		t.mBaudRateItems[rate] = item
	}

	t.mDataBits = t.mSerialConfig.AddSubMenuItem(fmt.Sprintf(i18n.Tray.DataBitsMenu, t.config.Serial.DataBits), i18n.Tray.DataBitsTip)
	for _, bits := range dataBitsList {
		label := strconv.Itoa(bits)
		if bits == t.config.Serial.DataBits {
			label = "✓ " + label
		}
		item := t.mDataBits.AddSubMenuItem(label, fmt.Sprintf(i18n.Tray.DataBitsItem, bits))
		t.mDataBitsItems[bits] = item
	}

	t.mStopBits = t.mSerialConfig.AddSubMenuItem(fmt.Sprintf(i18n.Tray.StopBitsMenu, stopBitsLabel(t.config.Serial.StopBits)), i18n.Tray.StopBitsTip)
	for _, bits := range stopBitsList {
		label := stopBitsLabel(bits)
		if bits == t.config.Serial.StopBits {
			label = "✓ " + label
		}
		item := t.mStopBits.AddSubMenuItem(label, fmt.Sprintf(i18n.Tray.StopBitsItem, label))
		t.mStopBitsItems[bits] = item
	}

	t.mParity = t.mSerialConfig.AddSubMenuItem(fmt.Sprintf(i18n.Tray.ParityMenu, t.config.Serial.Parity), i18n.Tray.ParityTip)
	for _, p := range parityList {
		label := p
		if strings.EqualFold(p, t.config.Serial.Parity) {
			label = "✓ " + label
		}
		item := t.mParity.AddSubMenuItem(label, fmt.Sprintf(i18n.Tray.ParityItem, p))
		t.mParityItems[p] = item
	}

	// 4. 当前配置显示
	t.mCurrentConfig = systray.AddMenuItem(t.getConfigSummary(), i18n.Tray.CurrentConfigTip)
	t.mCurrentConfig.Disable()

	systray.AddSeparator()

	// 5. 网络状态
	t.mNetworkStatus = systray.AddMenuItem(t.getNetworkStatus(), i18n.Tray.NetworkStatusTip)
	t.mNetworkStatus.Disable()

	// 6. 打开终端 (xterm.js)
	t.mOpenTerminal = systray.AddMenuItem(i18n.Tray.MenuOpenTerminal, i18n.Tray.MenuOpenTerminalTip)
	go func() {
		for range t.mOpenTerminal.ClickedCh {
			t.openTerminal()
		}
	}()

	systray.AddSeparator()

	// 7. 显示/隐藏窗口（仅当 showConsoleMenu=true 时显示）
	if t.showConsoleMenu {
		t.mShowLog = systray.AddMenuItem(i18n.Tray.MenuShowWindow, i18n.Tray.MenuShowWindowTip)
		systray.AddSeparator()
	}

	// 7. 版本
	mVersion := systray.AddMenuItem(fmt.Sprintf(i18n.Tray.MenuVersion, t.version), i18n.Tray.MenuVersionTip)
	mVersion.Disable()

	systray.AddSeparator()

	// 8. 退出
	mQuit := systray.AddMenuItem(i18n.Tray.MenuQuit, i18n.Tray.MenuQuitTip)

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

	// 显示/隐藏窗口（仅当菜单存在时注册）
	if t.showConsoleMenu && t.mShowLog != nil {
		go func() {
			for range t.mShowLog.ClickedCh {
				t.toggleConsoleWindow()
			}
		}()
	}

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
		item := t.mSelectPort.AddSubMenuItem(label, fmt.Sprintf(i18n.Tray.SelectPortItemTip, port))
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
	wasConnected := t.serial.IsConnected()
	if wasConnected {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Warnf("[SerialHub] 断开串口失败: %v", err)
			return
		}
	}

	for p, item := range t.mPortItems {
		if p == port {
			item.SetTitle("✓ " + port)
		} else {
			item.SetTitle(p)
		}
	}

	t.config.Serial.Port = port
	t.syncSerialConfig()
	t.updateConfigDisplay()
	t.notifyConfigChangedAndReconnect()

	if wasConnected {
		t.autoReconnect()
	} else {
		// 未连接时 autoReconnect 不执行，仍需刷新菜单文字（显示新端口）
		t.UpdateSerialStatus()
	}
	logrus.Infof("[SerialHub] 选择串口: %s", port)
}

func (t *TrayManager) setBaudRate(rate int) {
	wasConnected := t.serial.IsConnected()
	if wasConnected {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Warnf("[SerialHub] 断开串口失败: %v", err)
			return
		}
	}

	for r, item := range t.mBaudRateItems {
		if r == rate {
			item.SetTitle("✓ " + strconv.Itoa(rate))
		} else {
			item.SetTitle(strconv.Itoa(rate))
		}
	}

	t.config.Serial.BaudRate = rate
	t.syncSerialConfig()
	t.mBaudRate.SetTitle(fmt.Sprintf(i18n.Tray.BaudRateMenu, rate))
	t.updateConfigDisplay()
	t.notifyConfigChangedAndReconnect()

	if wasConnected {
		t.autoReconnect()
	}
	logrus.Infof("[SerialHub] 设置波特率: %d", rate)
}

func (t *TrayManager) setDataBits(bits int) {
	wasConnected := t.serial.IsConnected()
	if wasConnected {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Warnf("[SerialHub] 断开串口失败: %v", err)
			return
		}
	}

	for b, item := range t.mDataBitsItems {
		if b == bits {
			item.SetTitle("✓ " + strconv.Itoa(bits))
		} else {
			item.SetTitle(strconv.Itoa(bits))
		}
	}

	t.config.Serial.DataBits = bits
	t.syncSerialConfig()
	t.mDataBits.SetTitle(fmt.Sprintf(i18n.Tray.DataBitsMenu, bits))
	t.updateConfigDisplay()
	t.notifyConfigChangedAndReconnect()

	if wasConnected {
		t.autoReconnect()
	}
	logrus.Infof("[SerialHub] 设置数据位: %d", bits)
}

// stopBitsLabel 返回停止位的显示文案：1.5 保留一位小数，其余取整。
// 菜单标题、子项与配置概览共用，避免各自格式化导致文案不一致。
func stopBitsLabel(bits float64) string {
	if bits == 1.5 {
		return "1.5"
	}
	return fmt.Sprintf("%.0f", bits)
}

func (t *TrayManager) setStopBits(bits float64) {
	wasConnected := t.serial.IsConnected()
	if wasConnected {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Warnf("[SerialHub] 断开串口失败: %v", err)
			return
		}
	}

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
	t.syncSerialConfig()
	label := stopBitsLabel(bits)
	t.mStopBits.SetTitle(fmt.Sprintf(i18n.Tray.StopBitsMenu, label))
	t.updateConfigDisplay()
	t.notifyConfigChangedAndReconnect()

	if wasConnected {
		t.autoReconnect()
	}
	logrus.Infof("[SerialHub] 设置停止位: %s", label)
}

func (t *TrayManager) setParity(parity string) {
	wasConnected := t.serial.IsConnected()
	if wasConnected {
		if err := t.serial.Disconnect(); err != nil {
			logrus.Warnf("[SerialHub] 断开串口失败: %v", err)
			return
		}
	}

	for p, item := range t.mParityItems {
		if strings.EqualFold(p, parity) {
			item.SetTitle("✓ " + p)
		} else {
			item.SetTitle(p)
		}
	}

	t.config.Serial.Parity = parity
	t.syncSerialConfig()
	t.mParity.SetTitle(fmt.Sprintf(i18n.Tray.ParityMenu, parity))
	t.updateConfigDisplay()
	t.notifyConfigChangedAndReconnect()

	if wasConnected {
		t.autoReconnect()
	}
	logrus.Infof("[SerialHub] 设置校验位: %s", parity)
}

// syncSerialConfig 同步更新 SerialManager 的配置
func (t *TrayManager) syncSerialConfig() {
	serialCfg := &serial.Config{
		Port:     t.config.Serial.Port,
		BaudRate: t.config.Serial.BaudRate,
		DataBits: t.config.Serial.DataBits,
		Parity:   t.config.Serial.Parity,
		StopBits: float32(t.config.Serial.StopBits),
	}
	if err := t.serial.UpdateConfig(serialCfg); err != nil {
		logrus.Debugf("[SerialHub] 更新串口配置失败: %v", err)
	}
}

func (t *TrayManager) notifyConfigChangedAndReconnect() {
	if t.onConfigChanged != nil {
		t.onConfigChanged(t.config.Serial.Port, t.config.Serial.BaudRate, t.config.Serial.DataBits, t.config.Serial.Parity, t.config.Serial.StopBits)
	}
}

func (t *TrayManager) autoReconnect() {
	if t.config.Serial.Port == "" {
		return
	}
	if err := t.serial.Connect(); err != nil {
		logrus.Warnf("[SerialHub] 自动重连失败: %v", err)
	} else {
		logrus.Infof("[SerialHub] 自动重连成功: %s", t.config.Serial.Port)
	}
	t.UpdateSerialStatus()
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
	stopBits := stopBitsLabel(t.config.Serial.StopBits)
	port := t.config.Serial.Port
	if port == "" {
		port = i18n.Tray.PortNotSelected
	}
	return fmt.Sprintf(i18n.Tray.ConfigSummary,
		port,
		t.config.Serial.BaudRate,
		t.config.Serial.DataBits,
		parity,
		stopBits)
}

func (t *TrayManager) getNetworkStatus() string {
	return fmt.Sprintf("HTTP: %d", t.mcpPort)
}

func (t *TrayManager) getSerialMenuTitle() string {
	if t.serial.IsConnected() {
		return fmt.Sprintf(i18n.Tray.TitleConnected, t.serial.CurrentPort(), t.config.Serial.BaudRate)
	}
	return fmt.Sprintf(i18n.Tray.TitleConnect, t.config.Serial.Port)
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

// serialMenuStatus 是一次串口状态刷新的目标呈现（菜单文字/tooltip/图标状态）。
type serialMenuStatus struct {
	State     TrayState
	Connected bool
	Tooltip   string
	Title     string
}

// computeSerialMenuStatus 计算当前串口状态对应的菜单呈现（纯计算，
// 供 UpdateSerialStatus 与测试使用）。
func (t *TrayManager) computeSerialMenuStatus() serialMenuStatus {
	connected := t.serial != nil && t.serial.IsConnected()
	st := serialMenuStatus{
		State:     TrayIdle,
		Connected: connected,
		Tooltip:   i18n.Tray.TooltipIdle,
		Title:     t.getSerialMenuTitle(),
	}
	if connected {
		st.State = TrayConnected
		st.Tooltip = fmt.Sprintf(i18n.Tray.TooltipConnected, t.serial.CurrentPort())
	}
	return st
}

func (t *TrayManager) UpdateSerialStatus() {
	st := t.computeSerialMenuStatus()

	// 菜单文字与 tooltip 反映当前端口/波特率：切换串口等配置变更后即使
	// 连接状态未变（下面的去重命中）也必须刷新，否则菜单仍显示旧端口。
	if t.mSerial != nil {
		t.mSerial.SetTitle(st.Title)
		systray.SetTooltip(st.Tooltip)
	}

	if t.state == st.State {
		return
	}

	t.UpdateState(st.State)

	if st.Connected {
		t.mSerialConfig.Disable()
	} else {
		t.mSerialConfig.Enable()
	}
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

	return data
}

func (t *TrayManager) toggleConsoleWindow() {
	if t.consoleVisible {
		HideConsole()
		t.consoleVisible = false
		t.mShowLog.SetTitle(i18n.Tray.MenuShowWindow)
		logrus.Debug("[SerialHub] 菜单已更新: 显示窗口")
	} else {
		ShowConsole()
		t.consoleVisible = true
		t.mShowLog.SetTitle(i18n.Tray.MenuHideWindow)
		logrus.Debug("[SerialHub] 菜单已更新: 隐藏窗口")
	}
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

// openTerminal 在浏览器中打开 Web 终端页面
func (t *TrayManager) openTerminal() {
	url := fmt.Sprintf("http://%s:%d/terminal", t.host, t.mcpPort)
	logrus.Infof("[SerialHub] 打开终端: %s", url)

	// 使用 Windows 的 start 命令打开浏览器
	cmd := exec.Command("cmd", "/c", "start", url)
	if err := cmd.Start(); err != nil {
		logrus.Errorf("[SerialHub] 打开浏览器失败: %v", err)
	}
}
