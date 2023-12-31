//go:build !with_quic

package inbound

import (
	"context"

	"github.com/kumakuma10/sing-box/adapter"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
)

func NewHysteria(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HysteriaInboundOptions) (adapter.Inbound, error) {
	return nil, C.ErrQUICNotIncluded
}

type Hysteria struct {
	adapter.Inbound
}

func (h *Hysteria) AddUsers(_ []option.HysteriaUser) error {
	return C.ErrQUICNotIncluded
}

func (h *Hysteria) DelUsers(_ []string) error {
	return C.ErrQUICNotIncluded
}
