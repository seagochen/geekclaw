package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
	"github.com/seagosoft/geekclaw/geekclaw/channels"
	"github.com/seagosoft/geekclaw/geekclaw/config"
)

// fakeProcess 模拟外部信道进程。
// 它读取 JSON-RPC 请求并发送响应。
type fakeProcess struct {
	toProcess   *io.PipeWriter // 写入进程 stdin
	fromProcess *io.PipeReader // 从进程 stdout 读取

	internalReader *io.PipeReader // 进程从此读取（其 stdin）
	internalWriter *io.PipeWriter // 进程向此写入（其 stdout）

	capabilities []string
	mu           sync.Mutex
	sendCalls    []SendParams
}

func newFakeProcess(capabilities []string) *fakeProcess {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	fp := &fakeProcess{
		toProcess:      stdinW,
		fromProcess:    stdoutR,
		internalReader: stdinR,
		internalWriter: stdoutW,
		capabilities:   capabilities,
	}

	go fp.loop()
	return fp
}

func (fp *fakeProcess) loop() {
	buf := make([]byte, 65536)
	for {
		n, err := fp.internalReader.Read(buf)
		if err != nil {
			return
		}

		lines := strings.Split(strings.TrimSpace(string(buf[:n])), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}

			var req Request
			if err := json.Unmarshal([]byte(line), &req); err != nil {
				continue
			}

			var result any
			switch req.Method {
			case MethodInitialize:
				result = InitializeResult{
					Capabilities:     fp.capabilities,
					MaxMessageLength: 400,
				}
			case MethodStart:
				result = map[string]any{}
			case MethodStop:
				result = map[string]any{}
			case MethodSend:
				raw, _ := json.Marshal(req.Params)
				var sp SendParams
				_ = json.Unmarshal(raw, &sp)
				fp.mu.Lock()
				fp.sendCalls = append(fp.sendCalls, sp)
				fp.mu.Unlock()
				result = map[string]any{}
			case MethodStartTyping:
				result = TypingResult{StopID: "typing-123"}
			case MethodStopTyping:
				result = map[string]any{}
			case MethodEditMessage:
				result = map[string]any{}
			case MethodReact:
				result = ReactResult{UndoID: "reaction-456"}
			case MethodUndoReact:
				result = map[string]any{}
			case MethodSendPlaceholder:
				result = PlaceholderResult{MessageID: "placeholder-789"}
			default:
				result = map[string]any{}
			}

			resp := Response{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			fp.internalWriter.Write(data)
		}
	}
}

func (fp *fakeProcess) close() {
	fp.toProcess.Close()
	fp.fromProcess.Close()
	fp.internalReader.Close()
	fp.internalWriter.Close()
}

func TestExternalChannel_ClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantType error
	}{
		{"rate_limit", &RPCError{Code: -32001, Message: "slow down"}, channels.ErrRateLimit},
		{"temporary", &RPCError{Code: -32002, Message: "timeout"}, channels.ErrTemporary},
		{"not_running", &RPCError{Code: -32003, Message: "offline"}, channels.ErrNotRunning},
		{"send_failed", &RPCError{Code: -32099, Message: "bad chat"}, channels.ErrSendFailed},
		{"generic", fmt.Errorf("something broke"), channels.ErrTemporary},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.ErrorIs(t, result, tt.wantType)
		})
	}
}

func TestExternalChannel_InitAndSend(t *testing.T) {
	fp := newFakeProcess([]string{"typing", "edit", "reaction", "placeholder"})
	defer fp.close()

	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	ch, err := NewExternalChannel("test_irc", config.ExternalChannelConfig{
		Enabled: true,
		Command: "fake", // won't actually exec
	}, messageBus)
	require.NoError(t, err)

	// 绕过 exec.Command，直接连接 transport
	transport := NewTransport(fp.fromProcess, fp.toProcess)
	ch.transport = transport
	ch.SetRunning(true)

	// 启动读取循环
	go func() {
		_ = transport.ReadLoop()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试初始化
	raw, err := transport.Call(ctx, MethodInitialize, &InitializeParams{
		Name:   "test_irc",
		Config: map[string]any{"server": "irc.test.com:6697"},
	})
	require.NoError(t, err)

	var initResult InitializeResult
	require.NoError(t, json.Unmarshal(raw, &initResult))
	assert.Equal(t, 400, initResult.MaxMessageLength)
	assert.Contains(t, initResult.Capabilities, "typing")

	// 测试发送
	err = ch.Send(ctx, bus.OutboundMessage{
		ChatID:  "#test",
		Content: "hello from geekclaw",
	})
	require.NoError(t, err)

	// 验证假进程已收到发送请求
	time.Sleep(50 * time.Millisecond)
	fp.mu.Lock()
	require.Len(t, fp.sendCalls, 1)
	assert.Equal(t, "#test", fp.sendCalls[0].ChatID)
	assert.Equal(t, "hello from geekclaw", fp.sendCalls[0].Content)
	fp.mu.Unlock()
}

func TestExternalChannel_TypingCapability(t *testing.T) {
	fp := newFakeProcess([]string{"typing"})
	defer fp.close()

	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	ch, err := NewExternalChannel("test", config.ExternalChannelConfig{
		Enabled: true,
		Command: "fake",
	}, messageBus)
	require.NoError(t, err)

	transport := NewTransport(fp.fromProcess, fp.toProcess)
	ch.transport = transport
	ch.capabilities["typing"] = true
	ch.SetRunning(true)

	go func() {
		_ = transport.ReadLoop()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stop, err := ch.StartTyping(ctx, "#test")
	require.NoError(t, err)
	assert.NotNil(t, stop)

	// Stop 不应引发 panic
	stop()
}

func TestExternalChannel_NoCapability(t *testing.T) {
	// 信道不具备任何能力——所有可选方法应为空操作
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	ch, err := NewExternalChannel("test", config.ExternalChannelConfig{
		Enabled: true,
		Command: "fake",
	}, messageBus)
	require.NoError(t, err)
	// capabilities 映射默认为空

	ctx := context.Background()

	// Typing——应返回空操作的 stop 函数
	stop, err := ch.StartTyping(ctx, "#test")
	assert.NoError(t, err)
	stop() // should not panic

	// Edit——应为空操作
	assert.NoError(t, ch.EditMessage(ctx, "#test", "msg1", "new content"))

	// React——应返回空操作的 undo 函数
	undo, err := ch.ReactToMessage(ctx, "#test", "msg1")
	assert.NoError(t, err)
	undo() // should not panic

	// Placeholder——应返回空字符串
	id, err := ch.SendPlaceholder(ctx, "#test")
	assert.NoError(t, err)
	assert.Empty(t, id)

	// Media——应为空操作
	assert.NoError(t, ch.SendMedia(ctx, bus.OutboundMediaMessage{
		ChatID: "#test",
		Parts:  []bus.MediaPart{{Type: "image", Ref: "data:image/png;base64,xxx"}},
	}))
}

func TestNewExternalChannel_Validation(t *testing.T) {
	messageBus := bus.NewMessageBus()
	defer messageBus.Close()

	// 缺少命令
	_, err := NewExternalChannel("test", config.ExternalChannelConfig{
		Enabled: true,
		Command: "",
	}, messageBus)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}
