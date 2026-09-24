package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// wsConn 是 WebSocket 连接上的轻量 JSON-RPC 2.0 双向通道，主从共用。
//
// 读循环将消息分派为三类：响应（唤醒对应 Call 等待者）、
// 请求（调用 handler 并回写响应）、通知（调用 handler 不回写）。
type wsConn struct {
	ws *websocket.Conn

	writeMu   sync.Mutex
	pending   map[int64]chan *Response
	pendingMu sync.Mutex
	nextID    int64

	// handler 处理对端请求，返回结果；返回 (*RPCError, false) 表示错误。
	handler func(method string, params json.RawMessage) (any, *RPCError)
	// notifyHandler 处理对端通知。
	notifyHandler func(method string, params json.RawMessage)

	closed chan struct{}
}

func newWSConn(ws *websocket.Conn, handler func(string, json.RawMessage) (any, *RPCError), notifyHandler func(string, json.RawMessage)) *wsConn {
	return &wsConn{
		ws:            ws,
		pending:       make(map[int64]chan *Response),
		handler:       handler,
		notifyHandler: notifyHandler,
		closed:        make(chan struct{}),
	}
}

// readLoop 持续读取并分派消息，直到连接关闭或 ctx 取消。
// 返回 nil 表示正常关闭（对端断开）。
func (c *wsConn) readLoop(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.close()
	}()

	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			c.failPending(err)
			c.close()
			return err
		}

		// 先尝试按响应解析（有 id 且无 method）
		var resp Response
		if err := json.Unmarshal(raw, &resp); err == nil && resp.ID != 0 && resp.JSONRPC == "2.0" {
			// 可能是响应也可能是带 id 的请求：响应没有 method 字段。
			var probe struct {
				Method string `json:"method"`
			}
			_ = json.Unmarshal(raw, &probe)
			if probe.Method == "" {
				c.wakePending(resp.ID, &resp)
				continue
			}
		}

		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			logrus.Warnf("[Federation] 无法解析消息: %v", err)
			continue
		}

		params := json.RawMessage(nil)
		if req.Params != nil {
			b, _ := json.Marshal(req.Params)
			params = b
		}

		if req.ID == 0 {
			// 通知
			if c.notifyHandler != nil {
				c.notifyHandler(req.Method, params)
			}
			continue
		}

		// 请求：处理后回写
		go func(req Request, params json.RawMessage) {
			result, rpcErr := c.handleSafe(req.Method, params)
			resp := &Response{JSONRPC: "2.0", ID: req.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				resp.Result = result
			}
			if err := c.writeMsg(resp); err != nil {
				logrus.Warnf("[Federation] 回写响应失败: %v", err)
			}
		}(req, params)
	}
}

// handleSafe 调用 handler 并兜底 panic。
func (c *wsConn) handleSafe(method string, params json.RawMessage) (result any, rpcErr *RPCError) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[Federation] 处理 %s 时 panic: %v", method, r)
			rpcErr = &RPCError{Code: -32603, Message: fmt.Sprintf("内部错误: %v", r)}
		}
	}()
	return c.handler(method, params)
}

// Call 发送请求并等待对端响应。
func (c *wsConn) Call(ctx context.Context, method string, params any) (*Response, error) {
	c.pendingMu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.writeMsg(&Request{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("联邦连接已断开")
	}
}

// Notify 发送通知（不等待响应）。
func (c *wsConn) Notify(method string, params any) error {
	return c.writeMsg(&Request{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *wsConn) writeMsg(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(v)
}

func (c *wsConn) wakePending(id int64, resp *Response) {
	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	delete(c.pending, id)
	c.pendingMu.Unlock()
	if ok {
		ch <- resp
	}
}

// failPending 唤醒所有等待者并置错误（响应为 nil 由调用方判断）。
func (c *wsConn) failPending(err error) {
	c.pendingMu.Lock()
	ids := make([]int64, 0, len(c.pending))
	for id := range c.pending {
		ids = append(ids, id)
	}
	c.pendingMu.Unlock()
	for _, id := range ids {
		c.wakePending(id, &Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		})
	}
}

// close 幂等关闭底层连接与信号通道。
func (c *wsConn) close() {
	c.writeMu.Lock()
	select {
	case <-c.closed:
	default:
		close(c.closed)
		_ = c.ws.Close()
	}
	c.writeMu.Unlock()
}

func (c *wsConn) done() <-chan struct{} {
	return c.closed
}
