package external

import (
	"context"
	"fmt"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
	"github.com/seagosoft/geekclaw/geekclaw/plugin"
	"github.com/seagosoft/geekclaw/geekclaw/voice"
)

// ExternalTranscriber 将 geekclaw 的语音转录系统桥接到
// 通过 stdio 上的 JSON-RPC 通信的外部进程。实现 voice.Transcriber 接口。
type ExternalTranscriber struct {
	proc         *plugin.Process
	providerName string // 在初始化握手后设置
}

// 编译时接口检查。
var _ voice.Transcriber = (*ExternalTranscriber)(nil)

// NewExternalTranscriber 创建一个新的外部转录器插件。
func NewExternalTranscriber(name string, cfg PluginConfig) *ExternalTranscriber {
	return &ExternalTranscriber{
		proc: plugin.NewProcess(name, cfg),
	}
}

// Start 启动外部进程并执行初始化握手。
func (t *ExternalTranscriber) Start(ctx context.Context) error {
	raw, err := t.proc.Spawn(ctx, plugin.SpawnOpts{
		LogCategory: "voice",
		InitMethod:  MethodInitialize,
		InitParams:  &InitializeParams{Config: t.proc.PluginConfig().Config},
		StopMethod:  MethodStop,
		LogMethod:   MethodLog,
	})
	if err != nil {
		return err
	}

	initResult, err := ParseInitializeResult(raw)
	if err != nil {
		t.proc.Stop()
		return fmt.Errorf("parse initialize result: %w", err)
	}

	t.providerName = initResult.Name
	if t.providerName == "" {
		t.providerName = t.proc.Name()
	}

	logger.InfoCF("voice", "Transcriber plugin started", map[string]any{
		"plugin":        t.proc.Name(),
		"provider":      t.providerName,
		"audio_formats": initResult.AudioFormats,
		"command":       t.proc.PluginConfig().Command,
	})

	return nil
}

// Name 实现 voice.Transcriber 接口。
func (t *ExternalTranscriber) Name() string {
	if t.providerName != "" {
		return t.providerName
	}
	return "plugin:" + t.proc.Name()
}

// Transcribe 实现 voice.Transcriber 接口。
func (t *ExternalTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*voice.TranscriptionResponse, error) {
	raw, err := t.proc.Transport().Call(ctx, MethodTranscribe, &TranscribeParams{
		AudioFilePath: audioFilePath,
	})
	if err != nil {
		return nil, fmt.Errorf("transcriber plugin %q: %w", t.proc.Name(), err)
	}

	result, err := ParseTranscribeResult(raw)
	if err != nil {
		return nil, fmt.Errorf("parse transcribe result: %w", err)
	}

	return &voice.TranscriptionResponse{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
	}, nil
}

// Stop 优雅地关闭插件进程。
func (t *ExternalTranscriber) Stop() {
	t.proc.Stop()
}
