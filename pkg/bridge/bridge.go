// Package bridge provides data bridging functionality between serial, telnet, and MCP.
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

// TelnetBroadcaster 接口定义了 Telnet 数据读取和广播行为
type TelnetBroadcaster interface {
	DataChan() <-chan []byte
	Broadcast(data []byte) int
}

// DataBridge 管理串口、Telnet 和 MCP 之间的数据转发
type DataBridge struct {
	serial    SerialReader
	telnet    TelnetBroadcaster
	mcpBuffer *buffer.DataBuffer
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewDataBridge 创建新的数据桥接器
func NewDataBridge(serialMgr SerialReader, telnetSrv TelnetBroadcaster, mcpBuf *buffer.DataBuffer) (*DataBridge, error) {
	if serialMgr == nil {
		return nil, fmt.Errorf("串口管理器不能为空")
	}
	if telnetSrv == nil {
		return nil, fmt.Errorf("Telnet 服务器不能为空")
	}
	if mcpBuf == nil {
		return nil, fmt.Errorf("MCP 缓冲区不能为空")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DataBridge{
		serial:    serialMgr,
		telnet:    telnetSrv,
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
	// 取消上下文
	db.cancel()

	// 等待 goroutine 结束
	db.wg.Wait()

	return nil
}

// forwardLoop 数据转发主循环，使用 select 监听多个 channel
func (db *DataBridge) forwardLoop() {
	defer db.wg.Done()

	serialDataChan := db.serial.DataChan()
	telnetDataChan := db.telnet.DataChan()

	for {
		select {
		case <-db.ctx.Done():
			// 上下文取消，退出循环
			return

		case data, ok := <-serialDataChan:
			if !ok {
				// 串口 channel 已关闭
				logrus.Debugln("[SerialHub] 串口数据通道已关闭")
				return
			}
			if len(data) > 0 {
				// 串口数据 → Telnet 广播 + MCP 缓冲区
				db.forwardSerialToBoth(data)
			}

		case data, ok := <-telnetDataChan:
			if !ok {
				logrus.Warnln("[SerialHub] Telnet 数据通道已关闭")
				return
			}
			if len(data) > 0 {
				logrus.Infof("[SerialHub] 收到 Telnet 数据: %d 字节, 内容: %q", len(data), string(data))
				db.forwardTelnetToSerial(data)
			}
		}
	}
}

// forwardSerialToBoth 将串口数据同时转发到 Telnet 和 MCP
func (db *DataBridge) forwardSerialToBoth(data []byte) {
	// 转换换行符：将 \r\n 或 \r 统一转换为 \n
	// 这样可以避免 Telnet 客户端显示时出现重复行或空行
	cleaned := strings.ReplaceAll(string(data), "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	cleanedData := []byte(cleaned)

	// 转发到 Telnet（广播给所有客户端）
	telnetCount := db.telnet.Broadcast(cleanedData)
	if telnetCount > 0 {
		logrus.Debugf("[SerialHub] 转发串口数据到 Telnet: %d 字节, %d 客户端", len(cleanedData), telnetCount)
	}

	// 转发到 MCP 缓冲区
	db.mcpBuffer.Append(cleanedData)
	logrus.Debugf("[SerialHub] 转发串口数据到 MCP 缓冲区: %d 字节", len(cleanedData))
}

// forwardTelnetToSerial 将 Telnet 数据转发到串口
func (db *DataBridge) forwardTelnetToSerial(data []byte) {
	_, err := db.serial.Write(data)
	if err != nil {
		logrus.Errorf("[SerialHub] 转发 Telnet 数据到串口失败: %v", err)
	} else {
		logrus.Debugf("[SerialHub] 转发 Telnet 数据到串口: %d 字节", len(data))
	}
}
