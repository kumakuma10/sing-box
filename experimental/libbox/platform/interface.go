package platform

import (
	"context"
	"net/netip"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/process"
	"github.com/kumakuma10/sing-box/option"
	"github.com/sagernet/sing-tun"
	"github.com/sagernet/sing/common/control"
	"github.com/sagernet/sing/common/logger"
)

type Interface interface {
	Initialize(ctx context.Context, router adapter.Router) error
	UsePlatformAutoDetectInterfaceControl() bool
	AutoDetectInterfaceControl() control.Func
	OpenTun(options *tun.Options, platformOptions option.TunPlatformOptions) (tun.Tun, error)
	UsePlatformDefaultInterfaceMonitor() bool
	CreateDefaultInterfaceMonitor(logger logger.Logger) tun.DefaultInterfaceMonitor
	UsePlatformInterfaceGetter() bool
	Interfaces() ([]NetworkInterface, error)
	UnderNetworkExtension() bool
	ClearDNSCache()
	ReadWIFIState() adapter.WIFIState
	process.Searcher
}

type NetworkInterface struct {
	Index     int
	MTU       int
	Name      string
	Addresses []netip.Prefix
}
