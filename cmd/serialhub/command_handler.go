package main

import (
	"encoding/json"

	"github.com/dongly/serialhub/internal/i18n"
	"github.com/dongly/serialhub/pkg/serial"
)

// createCommandHandler 构造 Web 终端命令处理函数。
// requestShutdown 由 Web 终端「退出」按钮触发（shutdown 命令），
// 语义等价于收到 SIGINT；为 nil 时 shutdown 命令不可用。
func createCommandHandler(sm *serial.SerialManager, requestShutdown func()) func(cmd []byte) []byte {
	return func(cmd []byte) []byte {
		var msg map[string]interface{}
		if err := json.Unmarshal(cmd, &msg); err != nil {
			return nil
		}

		cmdType, ok := msg["type"].(string)
		if !ok {
			return nil
		}

		switch cmdType {
		case "get_ports":
			ports, err := sm.ListPorts()
			if err != nil {
				return webError(i18n.WebEvent.ListPortsFailed)
			}
			return jsonResponse("ports", ports)

		case "get_status":
			status := map[string]interface{}{
				"connected": sm.IsConnected(),
			}
			if sm.IsConnected() {
				cfg := sm.GetConfig()
				status["port"] = cfg.String()
			}
			return jsonResponse("status", status)

		case "connect":
			return handleConnectCommand(sm, msg)

		case "disconnect":
			if err := sm.Disconnect(); err != nil {
				return webError(i18n.WebEvent.DisconnectFailed)
			}
			return jsonResponse("disconnected", nil)

		case "shutdown":
			// Web 终端「退出」按钮：返回响应（尽力送达——广播仅入队，
			// 队列满或进程退出时可能丢失，前端点击后立即给出反馈兜底），
			// 停机由 requestShutdown 非阻塞触发，主循环收到 webShutdown
			// 后统一走 gracefulShutdown（与 Ctrl-C 同路径：bridge→HTTP→串口、释放锁）。
			if requestShutdown == nil {
				return webError(i18n.WebEvent.ShutdownUnsupported)
			}
			requestShutdown()
			return jsonResponse("shutting_down", nil)

		default:
			return nil
		}
	}
}

func handleConnectCommand(sm *serial.SerialManager, msg map[string]interface{}) []byte {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return webError(i18n.WebEvent.MissingConnection)
	}

	port, _ := data["port"].(string)
	if port == "" {
		return webError(i18n.WebEvent.MissingPort)
	}

	baudRate := 115200
	if br, ok := data["baudRate"].(float64); ok {
		baudRate = int(br)
	}

	dataBits := 8
	if db, ok := data["dataBits"].(float64); ok {
		dataBits = int(db)
	}

	parity := "none"
	if p, ok := data["parity"].(string); ok {
		parity = p
	}

	stopBits := 1
	if sb, ok := data["stopBits"].(float64); ok {
		stopBits = int(sb)
	}

	cfg := sm.GetConfig()
	cfg.Port = port
	cfg.BaudRate = baudRate
	cfg.DataBits = dataBits
	cfg.Parity = parity
	cfg.StopBits = float32(stopBits)
	if err := sm.UpdateConfig(cfg); err != nil {
		return webError(i18n.WebEvent.UpdateSettingsFailed)
	}

	if err := sm.Connect(); err != nil {
		return webError(i18n.WebEvent.ConnectFailed)
	}
	return jsonResponse("connected", sm.GetConfig().String())
}

func jsonResponse(msgType string, data interface{}) []byte {
	response := map[string]interface{}{
		"type": msgType,
		"data": data,
	}
	jsonData, _ := json.Marshal(response)
	return jsonData
}

func webError(code string) []byte {
	return jsonResponse("error", map[string]string{"code": code})
}
