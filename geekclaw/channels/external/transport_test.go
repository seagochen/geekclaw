package external

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransport_Call(t *testing.T) {
	// 模拟信道进程：向 "fromChannel" 写入响应，
	// transport 将其读取为信道的 stdout。
	fromChannel, toTransport := io.Pipe()
	fromTransport, toChannel := io.Pipe()

	transport := NewTransport(fromChannel, toChannel)

	// 启动读取循环
	go func() {
		_ = transport.ReadLoop()
	}()

	// 模拟信道响应请求 ID 1
	go func() {
		// 从 transport 读取请求
		buf := make([]byte, 4096)
		n, _ := fromTransport.Read(buf)
		var req Request
		_ = json.Unmarshal(buf[:n], &req)

		// 发送响应
		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"ok": true},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = toTransport.Write(data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := transport.Call(ctx, "channel.start", nil)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(result, &parsed))
	assert.Equal(t, true, parsed["ok"])

	toTransport.Close()
	fromTransport.Close()
}

func TestTransport_Notifications(t *testing.T) {
	// 模拟信道发送通知
	notifJSON := `{"jsonrpc":"2.0","method":"channel.message","params":{"sender_id":"alice","chat_id":"#test","content":"hello"}}` + "\n"

	reader := strings.NewReader(notifJSON)
	writer := &bytes.Buffer{} // 未使用

	transport := NewTransport(reader, writer)

	go func() {
		_ = transport.ReadLoop()
	}()

	select {
	case notif := <-transport.Notifications():
		assert.Equal(t, "channel.message", notif.Method)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

func TestTransport_Notify(t *testing.T) {
	var buf bytes.Buffer
	reader := strings.NewReader("") // 空，无输入

	transport := NewTransport(reader, &buf)

	err := transport.Notify("channel.log", &LogParams{
		Level:   "info",
		Message: "test message",
	})
	require.NoError(t, err)

	var notif Notification
	require.NoError(t, json.Unmarshal(buf.Bytes(), &notif))
	assert.Equal(t, "2.0", notif.JSONRPC)
	assert.Equal(t, "channel.log", notif.Method)
}

func TestTransport_CallTimeout(t *testing.T) {
	// 永不发送任何数据的 Reader
	reader := &blockingReader{}
	writer := &bytes.Buffer{}

	transport := NewTransport(reader, writer)
	go func() {
		_ = transport.ReadLoop()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := transport.Call(ctx, "channel.start", nil)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// blockingReader 永不返回任何数据。
type blockingReader struct{}

func (r *blockingReader) Read(p []byte) (n int, err error) {
	select {} // 永久阻塞
}

func TestTransport_CallError(t *testing.T) {
	fromChannel, toTransport := io.Pipe()
	fromTransport, toChannel := io.Pipe()

	transport := NewTransport(fromChannel, toChannel)

	go func() {
		_ = transport.ReadLoop()
	}()

	go func() {
		buf := make([]byte, 4096)
		n, _ := fromTransport.Read(buf)
		var req Request
		_ = json.Unmarshal(buf[:n], &req)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32001, Message: "rate limited"},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		_, _ = toTransport.Write(data)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := transport.Call(ctx, "channel.send", &SendParams{
		ChatID:  "#test",
		Content: "hello",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")

	toTransport.Close()
	fromTransport.Close()
}
