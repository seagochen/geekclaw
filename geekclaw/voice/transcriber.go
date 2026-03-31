// Package voice 提供语音转录功能。
package voice

import (
	"context"
)

// Transcriber 定义语音转录器的接口。
type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
}

// TranscriptionResponse 表示语音转录的响应结果。
type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}
