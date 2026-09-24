// Package bridge provides data bridging functionality between serial, WebSocket, and MCP.
package bridge

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/yourname/serialhub/internal/buffer"
)

// SerialReader 接口定义了串口数据读取和写入行为
type SerialReader interface {
	DataChan() <-chan []byte
	Write(data []byte) (int, error)
}

// WebSocketBroadcaster 接口定义了 WebSocket 数据读取和广播行为
type WebSocketBroadcaster interface {
	DataChan() <-chan []byte
	CmdChan() <-chan []byte
	Broadcast(data []byte) int
}

// DataBridge 管理串口、WebSocket 和 MCP 之间的数据转发
type DataBridge struct {
	serial      SerialReader
	ws          WebSocketBroadcaster
	mcpBuffer   *buffer.DataBuffer
	cmdHandler  CommandHandler
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

type CommandHandler func(cmd []byte) []byte

// NewDataBridge 创建新的数据桥接器
func NewDataBridge(serialMgr SerialReader, wsSrv WebSocketBroadcaster, mcpBuf *buffer.DataBuffer) (*DataBridge, error) {
	if serialMgr == nil {
		return nil, fmt.Errorf("串口管理器不能为空")
	}
	if wsSrv == nil {
		return nil, fmt.Errorf("WebSocket 服务器不能为空")
	}
	if mcpBuf == nil {
		return nil, fmt.Errorf("MCP 缓冲区不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DataBridge{
		serial:    serialMgr,
		ws:        wsSrv,
		mcpBuffer: mcpBuf,
		ctx:       ctx,
		cancel:    cancel,
	}, nil
}

func (db *DataBridge) SetCommandHandler(handler CommandHandler) {
	db.cmdHandler = handler
}

// Start 启动数据桥接器
func (db *DataBridge) Start() {
	db.wg.Add(2)
	go db.forwardLoop()
	go db.cmdLoop()
}

// Stop 停止数据桥接器
func (db *DataBridge) Stop() error {
	db.cancel()
	db.wg.Wait()
	return nil
}

// forwardLoop 数据转发主循环，使用 select 监听多个 channel
func (db *DataBridge) forwardLoop() {
	defer db.wg.Done()

	serialDataChan := db.serial.DataChan()
	wsDataChan := db.ws.DataChan()

	for {
		select {
		case <-db.ctx.Done():
			return

		case data, ok := <-serialDataChan:
			if !ok {
				logrus.Debugln("[SerialHub] 串口数据通道已关闭")
				return
			}
			if len(data) > 0 {
				db.forwardSerialToBoth(data)
			}

		case data, ok := <-wsDataChan:
			if !ok {
				logrus.Warnln("[SerialHub] WebSocket 数据通道已关闭")
				return
			}
			if len(data) > 0 {
				logrus.Infof("[SerialHub] 收到 WebSocket 数据: %d 字节, 内容: %q", len(data), string(data))
				db.forwardWsToSerial(data)
			}
		}
	}
}

func (db *DataBridge) cmdLoop() {
	defer db.wg.Done()

	cmdChan := db.ws.CmdChan()

	for {
		select {
		case <-db.ctx.Done():
			return

		case cmd, ok := <-cmdChan:
			if !ok {
				return
			}
			db.handleCommand(cmd)
		}
	}
}

func (db *DataBridge) handleCommand(cmd []byte) {
	if db.cmdHandler != nil {
		response := db.cmdHandler(cmd)
		if response != nil {
			db.ws.Broadcast(response)
		}
	}
}

// forwardSerialToBoth 将串口数据同时转发到 WebSocket 和 MCP
func (db *DataBridge) forwardSerialToBoth(data []byte) {
	cleanedData := ConvertLFToCRLF(data)

	wsCount := db.ws.Broadcast(cleanedData)
	if wsCount > 0 {
		logrus.Debugf("[SerialHub] 转发串口数据到 WebSocket: %d 字节, %d 客户端", len(cleanedData), wsCount)
	}

	db.mcpBuffer.Append(cleanedData)
	logrus.Debugf("[SerialHub] 转发串口数据到 MCP 缓冲区: %d 字节", len(cleanedData))
}

func ConvertLFToCRLF(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	result := make([]byte, 0, len(data)+bytes.Count(data, []byte{'\n'}))

	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i == 0 || data[i-1] != '\r' {
				result = append(result, '\r')
			}
		}
		result = append(result, data[i])
	}

	return result
}

// forwardWsToSerial 将 WebSocket 数据转发到串口
func (db *DataBridge) forwardWsToSerial(data []byte) {
	_, err := db.serial.Write(data)
	if err != nil {
		logrus.Errorf("[SerialHub] 转发 WebSocket 数据到串口失败: %v", err)
	} else {
		logrus.Debugf("[SerialHub] 转发 WebSocket 数据到串口: %d 字节", len(data))
	}
}
