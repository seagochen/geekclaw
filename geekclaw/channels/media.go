package channels

import (
	"context"

	"github.com/seagosoft/geekclaw/geekclaw/bus"
)

// MediaSender 是频道可选实现的接口，用于发送媒体附件（图片、文件、音频、视频）。
// Manager 通过类型断言发现实现此接口的频道，并将 OutboundMediaMessage 路由给它们。
type MediaSender interface {
	SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error
}
