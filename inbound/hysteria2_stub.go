//go:build !with_quic

package inbound

import (
	"context"

	"github.com/kumakuma10/sing-box/adapter"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
)

type Hysteria2 struct {
	adapter.Inbound
}

func (h *Hysteria2) AddUsers(_ []option.Hysteria2User) error {
	return C.ErrQUICNotIncluded
}

func (h *Hysteria2) DelUsers(_ []string) error {
	return C.ErrQUICNotIncluded
}

func NewHysteria2(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.Hysteria2InboundOptions) (adapter.Inbound, error) {
	return nil, C.ErrQUICNotIncluded
}
