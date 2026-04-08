// Package bridge provides data bridging functionality between serial, WebSocket, and MCP.
package bridge

import (
	"context"
	"fmt"
	"strings"
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
	Broadcast(data []byte) int
}

// DataBridge 管理串口、WebSocket 和 MCP 之间的数据转发
type DataBridge struct {
	serial    SerialReader
	ws        WebSocketBroadcaster
	mcpBuffer *buffer.DataBuffer
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

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

// Start 启动数据桥接器
func (db *DataBridge) Start() {
	db.wg.Add(1)
	go db.forwardLoop()
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

// forwardSerialToBoth 将串口数据同时转发到 WebSocket 和 MCP
func (db *DataBridge) forwardSerialToBoth(data []byte) {
	// xterm.js 需要 \r\n 来正确换行
	// 将单独的 \n 转换为 \r\n，保留已有的 \r\n
	original := string(data)

	// 先处理 \r\n，避免重复转换
	// 将 \r\n 临时替换为特殊标记
	marker := "\x00CRLF\x00"
	withMarker := strings.ReplaceAll(original, "\r\n", marker)

	// 将剩余的 \r 或 \n 统一转换为 \r\n
	withMarker = strings.ReplaceAll(withMarker, "\r", "\r\n")
	withMarker = strings.ReplaceAll(withMarker, "\n", "\r\n")

	// 恢复原始的 \r\n
	cleaned := strings.ReplaceAll(withMarker, marker, "\r\n")

	cleanedData := []byte(cleaned)

	wsCount := db.ws.Broadcast(cleanedData)
	if wsCount > 0 {
		logrus.Debugf("[SerialHub] 转发串口数据到 WebSocket: %d 字节, %d 客户端", len(cleanedData), wsCount)
	}

	db.mcpBuffer.Append(cleanedData)
	logrus.Debugf("[SerialHub] 转发串口数据到 MCP 缓冲区: %d 字节", len(cleanedData))
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
