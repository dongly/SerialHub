package federation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// Manager 运行在主实例上：接纳从实例注册，维护聚合端口视图，
// 并将 MCP 工具请求路由到对应从实例。
type Manager struct {
	mu sync.RWMutex

	// localSide 主实例所在侧（windows/wsl）。
	localSide string
	// workers 已注册的从实例会话。
	workers map[*workerSession]struct{}
	// ports 联邦端口表："wsl:ttyUSB1" → 所属会话。
	ports map[string]*workerSession
	// activePort 当前经联邦打开的端口全名（"" 表示无）。
	activePort string

	// onData 从侧数据上行回调（主侧注入数据桥）。
	onData func(portKey string, data []byte)
}

// workerSession 表示一个已连接的从实例。
type workerSession struct {
	conn  *wsConn
	side  string   // 从实例上报的 OS 侧
	ports []string // 从实例注册的裸端口名
}

// NewManager 创建主侧联邦管理器。localSide 为本侧标识（SideWindows/SideWSL）。
func NewManager(localSide string) *Manager {
	return &Manager{
		localSide: localSide,
		workers:   make(map[*workerSession]struct{}),
		ports:     make(map[string]*workerSession),
	}
}

// SetDataHandler 设置从侧数据上行回调；须在 HTTP 服务启动前调用。
func (m *Manager) SetDataHandler(f func(portKey string, data []byte)) {
	m.mu.Lock()
	m.onData = f
	m.mu.Unlock()
}

// LocalSide 返回主实例所在侧。
func (m *Manager) LocalSide() string { return m.localSide }

// WorkerCount 返回当前已注册的从实例数。
func (m *Manager) WorkerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// FederatedPorts 返回全部联邦端口（从实例上报）。
func (m *Manager) FederatedPorts() []PortInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []PortInfo
	for key := range m.ports {
		side, bare := splitPortKey(key)
		out = append(out, PortInfo{Name: key, Origin: "federated", Side: side, Port: bare})
	}
	return out
}

// ActiveFederatedPort 返回当前经联邦打开的端口全名，无则空串。
func (m *Manager) ActiveFederatedPort() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activePort
}

// IsFederated 判断端口名是否属于联邦（即从实例上报的端口）。
func (m *Manager) IsFederated(portName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.ports[portName]
	return ok
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // 联邦为本地/局域网信任通道
}

// HandleWS 处理 /federation WebSocket 升级，进入从实例会话循环。
func (m *Manager) HandleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Warnf("[Federation] WebSocket 升级失败: %v", err)
		return
	}

	sess := &workerSession{}
	sess.conn = newWSConn(ws,
		func(method string, params json.RawMessage) (any, *RPCError) {
			return m.handleWorkerRequest(sess, method, params)
		},
		func(method string, params json.RawMessage) {
			m.handleWorkerNotify(sess, method, params)
		},
	)

	if err := sess.conn.readLoop(context.Background()); err != nil {
		logrus.Debugf("[Federation] 从实例连接退出: %v", err)
	}
	m.removeWorker(sess)
}

// handleWorkerRequest 处理从实例请求（当前仅 federation/register）。
func (m *Manager) handleWorkerRequest(sess *workerSession, method string, params json.RawMessage) (any, *RPCError) {
	switch method {
	case MethodRegister:
		var p RegisterParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{Code: -32602, Message: "register 参数错误: " + err.Error()}
		}
		m.registerWorker(sess, p)
		return SerialResult{Success: true, Message: "注册成功"}, nil
	default:
		return nil, &RPCError{Code: -32601, Message: "未知方法: " + method}
	}
}

// handleWorkerNotify 处理从实例通知（serial/data 上行）。
func (m *Manager) handleWorkerNotify(sess *workerSession, method string, params json.RawMessage) {
	if method != MethodSerialData {
		return
	}
	var p DataParams
	if err := json.Unmarshal(params, &p); err != nil {
		logrus.Warnf("[Federation] 数据通知解析失败: %v", err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		logrus.Warnf("[Federation] 数据 base64 解码失败: %v", err)
		return
	}
	m.mu.RLock()
	f := m.onData
	m.mu.RUnlock()
	if f != nil {
		f(portKey(sess.side, p.Port), data)
	}
}

// registerWorker 记录从实例身份与端口列表。
func (m *Manager) registerWorker(sess *workerSession, p RegisterParams) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 先清理该会话旧端口（重新注册场景）
	for key, s := range m.ports {
		if s == sess {
			delete(m.ports, key)
		}
	}
	sess.side = p.OS
	sess.ports = p.Ports
	m.workers[sess] = struct{}{}
	for _, bare := range p.Ports {
		m.ports[portKey(p.OS, bare)] = sess
	}
	logrus.Infof("[Federation] 从实例已注册: side=%s ports=%v", p.OS, p.Ports)
}

// removeWorker 会话断开时清理其端口与活动连接状态。
func (m *Manager) removeWorker(sess *workerSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workers[sess]; !ok {
		return
	}
	delete(m.workers, sess)
	for key, s := range m.ports {
		if s == sess {
			delete(m.ports, key)
			if m.activePort == key {
				m.activePort = ""
			}
		}
	}
	logrus.Infof("[Federation] 从实例已断开: side=%s", sess.side)
}

// callSerial 向从实例发送 serial/* 请求并解析响应。
func (m *Manager) callSerial(portName, method string, params SerialParams) SerialResult {
	m.mu.RLock()
	sess, ok := m.ports[portName]
	m.mu.RUnlock()
	if !ok {
		return SerialResult{Success: false, Message: "联邦端口不存在或从实例已断开: " + portName}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := sess.conn.Call(ctx, method, params)
	if err != nil {
		return SerialResult{Success: false, Message: "联邦请求失败: " + err.Error()}
	}
	if resp.Error != nil {
		return SerialResult{Success: false, Message: resp.Error.Message}
	}
	// 响应 Result 即 SerialResult
	b, _ := json.Marshal(resp.Result)
	var result SerialResult
	if err := json.Unmarshal(b, &result); err != nil {
		return SerialResult{Success: false, Message: "联邦响应解析失败: " + err.Error()}
	}
	return result
}

// Open 经联邦打开从侧串口。portName 为全名（如 "wsl:ttyUSB1"）。
func (m *Manager) Open(portName string, baudRate int) SerialResult {
	_, bare := splitPortKey(portName)
	result := m.callSerial(portName, MethodSerialOpen, SerialParams{Port: bare, BaudRate: baudRate})
	if result.Success {
		m.mu.Lock()
		m.activePort = portName
		m.mu.Unlock()
	}
	return result
}

// Write 经联邦向从侧串口写数据。
func (m *Manager) Write(portName string, data []byte) SerialResult {
	_, bare := splitPortKey(portName)
	return m.callSerial(portName, MethodSerialWrite, SerialParams{Port: bare, Data: base64.StdEncoding.EncodeToString(data)})
}

// Close 经联邦关闭从侧串口。
func (m *Manager) Close(portName string) SerialResult {
	_, bare := splitPortKey(portName)
	result := m.callSerial(portName, MethodSerialClose, SerialParams{Port: bare})
	if result.Success {
		m.mu.Lock()
		if m.activePort == portName {
			m.activePort = ""
		}
		m.mu.Unlock()
	}
	return result
}

// portKey 生成联邦端口全名。
func portKey(side, bare string) string {
	return side + ":" + bare
}

// splitPortKey 拆解 "side:port" 全名；无前缀时原样返回。
func splitPortKey(key string) (side, bare string) {
	if i := strings.Index(key, ":"); i > 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}
