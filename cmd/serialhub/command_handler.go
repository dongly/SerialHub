package main

import (
	"encoding/json"

	"github.com/yourname/serialhub/pkg/serial"
)

func createCommandHandler(sm *serial.SerialManager) func(cmd []byte) []byte {
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
				return jsonResponse("error", err.Error())
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
				return jsonResponse("error", err.Error())
			}
			return jsonResponse("disconnected", nil)

		default:
			return nil
		}
	}
}

func handleConnectCommand(sm *serial.SerialManager, msg map[string]interface{}) []byte {
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		return jsonResponse("error", "缺少连接参数")
	}

	port, _ := data["port"].(string)
	if port == "" {
		return jsonResponse("error", "缺少端口参数")
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
	sm.UpdateConfig(cfg)

	if err := sm.Connect(); err != nil {
		return jsonResponse("error", err.Error())
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