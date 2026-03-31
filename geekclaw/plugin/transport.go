package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Transport 处理通过 io.Reader/Writer 对（通常是外部进程的 stdio 管道）
// 进行的 JSON-RPC 2.0 双向通信。
//
// 支持三种消息流：
//   - Go→Plugin 请求/响应（Call）
//   - Plugin→Go 通知（Notifications）
//   - Plugin→Go 反向请求/响应（RegisterService + handleReverseCall）
type Transport struct {
	writer  io.Writer
	scanner *bufio.Scanner

	nextID  atomic.Int64
	pending sync.Map // id -> chan *Response

	// 从外部进程接收的通知
	notifyCh chan *Notification

	// 反向调用：插件通过 host.* 方法调用 Go 服务
	services sync.Map // method -> ServiceHandler
	ctx      context.Context

	mu sync.Mutex // 序列化写入
}

// NewTransport 创建一个基于给定 reader（进程 stdout）和
// writer（进程 stdin）的 Transport。
func NewTransport(r io.Reader, w io.Writer) *Transport {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 最大 10MB 行

	return &Transport{
		writer:   w,
		scanner:  scanner,
		notifyCh: make(chan *Notification, 64),
		ctx:      context.Background(),
	}
}

// SetContext 设置用于反向调用 ServiceHandler 的上下文。
// 必须在 ReadLoop 启动之前调用。
func (t *Transport) SetContext(ctx context.Context) {
	t.ctx = ctx
}

// RegisterService 注册一个 Go 服务处理函数，供插件通过反向 RPC 调用。
// method 应使用 "host.<service>.<action>" 格式。
func (t *Transport) RegisterService(method string, handler ServiceHandler) {
	t.services.Store(method, handler)
}

// Notifications 返回入站通知的只读通道。
func (t *Transport) Notifications() <-chan *Notification {
	return t.notifyCh
}

// Call 发送 JSON-RPC 请求并等待响应。
func (t *Transport) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *Response, 1)
	t.pending.Store(id, ch)
	defer t.pending.Delete(id)

	if err := t.send(req); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		raw, err := json.Marshal(resp.Result)
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
		return raw, nil
	}
}

// Notify 发送 JSON-RPC 通知（不期望响应）。
func (t *Transport) Notify(method string, params any) error {
	n := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return t.send(n)
}

// ReadLoop 从外部进程读取消息并分发。
// 阻塞直到 reader 关闭或发生错误。
//
// 三路分发：
//   - id != nil && method == "": 响应（Go 发出的请求的回复）
//   - id != nil && method != "": 反向调用（插件调用 Go 服务）
//   - id == nil && method != "": 通知（插件发出的单向消息）
func (t *Transport) ReadLoop() error {
	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var peek struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &peek); err != nil {
			continue
		}

		if peek.ID != nil && peek.Method == "" {
			// 响应：Plugin 回复 Go 发出的请求
			resp := &Response{
				JSONRPC: "2.0",
				ID:      *peek.ID,
				Error:   peek.Error,
			}
			if peek.Result != nil {
				var result any
				_ = json.Unmarshal(peek.Result, &result)
				resp.Result = result
			}
			if ch, ok := t.pending.Load(*peek.ID); ok {
				ch.(chan *Response) <- resp
			}
		} else if peek.ID != nil && peek.Method != "" {
			// 反向调用：Plugin 调用 Go 服务
			id := *peek.ID
			method := peek.Method
			params := peek.Params
			go t.handleReverseCall(id, method, params)
		} else if peek.Method != "" {
			// 通知
			var notif Notification
			if err := json.Unmarshal(line, &notif); err != nil {
				continue
			}
			select {
			case t.notifyCh <- &notif:
			default:
				// 缓冲区满时丢弃
			}
		}
	}

	return t.scanner.Err()
}

// handleReverseCall 处理来自插件的反向 JSON-RPC 调用。
func (t *Transport) handleReverseCall(id int64, method string, rawParams json.RawMessage) {
	handler, ok := t.services.Load(method)
	if !ok {
		_ = t.send(Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &RPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", method)},
		})
		return
	}

	result, err := handler.(ServiceHandler)(t.ctx, rawParams)
	if err != nil {
		_ = t.send(Response{
			JSONRPC: "2.0",
			ID:      id,
			Error:   &RPCError{Code: -32000, Message: err.Error()},
		})
		return
	}

	_ = t.send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// send 序列化并写入单行 JSON。
func (t *Transport) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = t.writer.Write(data)
	return err
}
