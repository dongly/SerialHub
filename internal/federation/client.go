package federation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/dongly/serialhub/pkg/mcp/tools"
	"github.com/dongly/serialhub/pkg/serial"
)

// reconnectAttempts 断线重连次数；全部失败后触发晋升评估。
const reconnectAttempts = 3

// reconnectInterval 重连间隔。
const reconnectInterval = time.Second

// RunWorker 运行从实例联邦客户端：连接主实例、注册本侧端口、
// 响应主侧串口请求并上行数据。断线重连 3 次失败后调用 onPromote
// （晋升为主实例）并返回。
func RunWorker(ctx context.Context, masterURL, side string, sm *serial.SerialManager, onPromote func()) error {
	wsURL := strings.Replace(masterURL, "http://", "ws://", 1) + "/federation"

	for attempt := 0; ; attempt++ {
		if attempt >= reconnectAttempts {
			logrus.Warnf("[Federation] 与主实例失去联系（重连 %d 次失败），触发晋升评估", reconnectAttempts)
			if onPromote != nil {
				onPromote()
			}
			return nil
		}
		if attempt > 0 {
			logrus.Infof("[Federation] 正在重连主实例 (%d/%d)...", attempt, reconnectAttempts)
		}

		err := workerSessionOnce(ctx, wsURL, side, sm)
		if ctx.Err() != nil {
			return nil // 本地主动退出（Ctrl+C 等），不晋升
		}
		if err != nil {
			logrus.Warnf("[Federation] 会话中断: %v", err)
			select {
			case <-time.After(reconnectInterval):
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// workerSessionOnce 建立一次从实例会话直至断开。
func workerSessionOnce(ctx context.Context, wsURL, side string, sm *serial.SerialManager) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	ws, resp, err := websocket.DefaultDialer.DialContext(dialCtx, wsURL, nil)
	cancel()
	if err != nil {
		return err
	}
	if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
		ws.Close()
		return errUnexpectedStatus(resp.Status)
	}
	logrus.Infof("[Federation] 已连接主实例: %s", wsURL)

	conn := newWSConn(ws, func(method string, params json.RawMessage) (any, *RPCError) {
		return handleMasterRequest(sm, method, params)
	}, nil)

	// 读循环必须先于首个 Call 启动：Call 的响应分派依赖 readLoop，
	// 否则 register 响应无人读取，形成死锁（实测 Recv-Q 滞留即此因）。
	readDone := make(chan error, 1)
	go func() { readDone <- conn.readLoop(ctx) }()

	// 注册本侧端口
	ports, err := sm.ListPorts()
	if err != nil {
		ports = nil
	}
	registerResp, err := conn.Call(ctx, MethodRegister, RegisterParams{OS: side, Ports: ports})
	if err != nil {
		conn.close()
		return err
	}
	if registerResp.Error != nil {
		conn.close()
		return errRegisterRejected(registerResp.Error.Message)
	}

	// 数据上行循环
	pumpCtx, pumpCancel := context.WithCancel(ctx)
	defer pumpCancel()
	go dataUpstream(pumpCtx, conn, sm)

	defer conn.close()
	return <-readDone
}

// dataUpstream 将本地串口数据经联邦通道上行至主实例。
func dataUpstream(ctx context.Context, conn *wsConn, sm *serial.SerialManager) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.done():
			return
		case data, ok := <-sm.DataChan():
			if !ok {
				return
			}
			port := sm.CurrentPort()
			if port == "" {
				continue
			}
			if err := conn.Notify(MethodSerialData, DataParams{
				Port: port,
				Data: base64.StdEncoding.EncodeToString(data),
			}); err != nil {
				return
			}
		}
	}
}

// handleMasterRequest 响应主实例下发的 serial/* 请求，复用 tools 层纯函数。
func handleMasterRequest(sm *serial.SerialManager, method string, params json.RawMessage) (any, *RPCError) {
	var p SerialParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &RPCError{Code: -32602, Message: "参数解析失败: " + err.Error()}
		}
	}

	switch method {
	case MethodSerialOpen:
		result := tools.ExecuteSerialConnect(sm, tools.ConnectInput{Port: p.Port, BaudRate: p.BaudRate})
		return serialResultFromTool(result), nil

	case MethodSerialWrite:
		data, err := base64.StdEncoding.DecodeString(p.Data)
		if err != nil {
			return SerialResult{Success: false, Message: "数据 base64 解码失败"}, nil
		}
		result := tools.ExecuteSerialWrite(sm, tools.WriteInput{Data: string(data)})
		return serialResultFromTool(result), nil

	case MethodSerialClose:
		result := tools.ExecuteSerialDisconnect(sm)
		return serialResultFromTool(result), nil

	default:
		return nil, &RPCError{Code: -32601, Message: "未知方法: " + method}
	}
}

// serialResultFromTool 将 ToolResult 转为联邦 SerialResult。
func serialResultFromTool(r tools.ToolResult) SerialResult {
	return SerialResult{Success: r.Success, Message: r.Message, Data: r.Data}
}

// errUnexpectedStatus / errRegisterRejected 轻量错误构造。
func errUnexpectedStatus(status string) error {
	return &RPCError{Code: -32000, Message: "主实例拒绝 WebSocket 升级: " + status}
}
func errRegisterRejected(msg string) error {
	return &RPCError{Code: -32000, Message: "注册被拒绝: " + msg}
}
