package bus

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/seagosoft/geekclaw/geekclaw/logger"
)

// ErrBusClosed 在向已关闭的 MessageBus 发布消息时返回。
var ErrBusClosed = errors.New("message bus closed")

// defaultBusBufferSize 是消息总线通道的默认缓冲区大小。
const defaultBusBufferSize = 64

// MessageBus 是核心消息总线，负责在入站、出站和媒体通道之间路由消息。
type MessageBus struct {
	inbound       chan InboundMessage
	outbound      chan OutboundMessage
	outboundMedia chan OutboundMediaMessage
	done          chan struct{}
	closed        atomic.Bool
}

// NewMessageBus 创建并返回一个新的消息总线实例。
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:       make(chan InboundMessage, defaultBusBufferSize),
		outbound:      make(chan OutboundMessage, defaultBusBufferSize),
		outboundMedia: make(chan OutboundMediaMessage, defaultBusBufferSize),
		done:          make(chan struct{}),
	}
}

// PublishInbound 向入站通道发布消息。
func (mb *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mb.inbound <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeInbound 从入站通道消费一条消息。
func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg, ok := <-mb.inbound:
		return msg, ok
	case <-mb.done:
		return InboundMessage{}, false
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound 向出站通道发布消息。
func (mb *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mb.outbound <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeOutbound 从出站通道订阅并接收一条消息。
func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg, ok := <-mb.outbound:
		return msg, ok
	case <-mb.done:
		return OutboundMessage{}, false
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

// PublishOutboundMedia 向出站媒体通道发布媒体消息。
func (mb *MessageBus) PublishOutboundMedia(ctx context.Context, msg OutboundMediaMessage) error {
	if mb.closed.Load() {
		return ErrBusClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mb.outboundMedia <- msg:
		return nil
	case <-mb.done:
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SubscribeOutboundMedia 从出站媒体通道订阅并接收一条媒体消息。
func (mb *MessageBus) SubscribeOutboundMedia(ctx context.Context) (OutboundMediaMessage, bool) {
	select {
	case msg, ok := <-mb.outboundMedia:
		return msg, ok
	case <-mb.done:
		return OutboundMediaMessage{}, false
	case <-ctx.Done():
		return OutboundMediaMessage{}, false
	}
}

// Close 关闭消息总线，排空缓冲通道中的消息。
func (mb *MessageBus) Close() {
	if mb.closed.CompareAndSwap(false, true) {
		close(mb.done)

		// 排空缓冲通道，避免消息被静默丢弃。
		// 不关闭通道以避免并发发布者导致的 send-on-closed panic。
		drained := 0
		for {
			select {
			case <-mb.inbound:
				drained++
			default:
				goto doneInbound
			}
		}
	doneInbound:
		for {
			select {
			case <-mb.outbound:
				drained++
			default:
				goto doneOutbound
			}
		}
	doneOutbound:
		for {
			select {
			case <-mb.outboundMedia:
				drained++
			default:
				goto doneMedia
			}
		}
	doneMedia:
		if drained > 0 {
			logger.DebugCF("bus", "Drained buffered messages during close", map[string]any{
				"count": drained,
			})
		}
	}
}
