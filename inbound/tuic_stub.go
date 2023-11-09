//go:build !with_quic

package inbound

import (
	"context"

	"github.com/kumakuma10/sing-box/adapter"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
)

type TUIC struct {
	adapter.Inbound
}

func NewTUIC(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TUICInboundOptions) (adapter.Inbound, error) {
	return nil, C.ErrQUICNotIncluded
}

func (h *TUIC) AddUsers(_ []option.TUICUser) error {
	return C.ErrQUICNotIncluded
}

func (h *TUIC) DelUsers(_ []string) error {
	return C.ErrQUICNotIncluded
}
