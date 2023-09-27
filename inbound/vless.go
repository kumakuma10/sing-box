package inbound

import (
	"context"
	"net"
	"os"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/tls"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	"github.com/kumakuma10/sing-box/transport/v2ray"
	"github.com/kumakuma10/sing-box/transport/vless"
	"github.com/sagernet/sing-vmess"
	"github.com/sagernet/sing-vmess/packetaddr"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	F "github.com/sagernet/sing/common/format"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

var (
	_ adapter.Inbound           = (*VLESS)(nil)
	_ adapter.InjectableInbound = (*VLESS)(nil)
)

type VLESS struct {
	myInboundAdapter
	ctx       context.Context
	users     []option.VLESSUser
	service   *vless.Service[int]
	tlsConfig tls.ServerConfig
	transport adapter.V2RayServerTransport
}

func NewVLESS(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions) (*VLESS, error) {
	inbound := &VLESS{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeVLESS,
			network:       []string{N.NetworkTCP},
			ctx:           ctx,
			router:        router,
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		ctx:   ctx,
		users: options.Users,
	}
	service := vless.NewService[int](logger, adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound))
	service.UpdateUsers(common.MapIndexed(inbound.users, func(index int, _ option.VLESSUser) int {
		return index
	}), common.Map(inbound.users, func(it option.VLESSUser) string {
		return it.UUID
	}), common.Map(inbound.users, func(it option.VLESSUser) string {
		return it.Flow
	}))
	inbound.service = service
	var err error
	if options.TLS != nil {
		inbound.tlsConfig, err = tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
	}
	if options.Transport != nil {
		inbound.transport, err = v2ray.NewServerTransport(ctx, common.PtrValueOrDefault(options.Transport), inbound.tlsConfig, (*vlessTransportHandler)(inbound))
		if err != nil {
			return nil, E.Cause(err, "create server transport: ", options.Transport.Type)
		}
	}
	inbound.connHandler = inbound
	return inbound, nil
}

func (h *VLESS) Start() error {
	err := common.Start(
		h.service,
		h.tlsConfig,
	)
	if err != nil {
		return err
	}
	if h.transport == nil {
		return h.myInboundAdapter.Start()
	}
	if common.Contains(h.transport.Network(), N.NetworkTCP) {
		tcpListener, err := h.myInboundAdapter.ListenTCP()
		if err != nil {
			return err
		}
		go func() {
			sErr := h.transport.Serve(tcpListener)
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	if common.Contains(h.transport.Network(), N.NetworkUDP) {
		udpConn, err := h.myInboundAdapter.ListenUDP()
		if err != nil {
			return err
		}
		go func() {
			sErr := h.transport.ServePacket(udpConn)
			if sErr != nil && !E.IsClosed(sErr) {
				h.logger.Error("transport serve error: ", sErr)
			}
		}()
	}
	return nil
}

func (h *VLESS) AddUsers(users []option.VLESSUser) error {
	if cap(h.users)-len(h.users) >= len(users) {
		h.users = append(h.users, users...)
	} else {
		tmp := make([]option.VLESSUser, 0, len(h.users)+len(users)+10)
		tmp = append(tmp, h.users...)
		tmp = append(tmp, users...)
		h.users = tmp
	}
	h.service.UpdateUsers(common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
		return index
	}), common.Map(h.users, func(it option.VLESSUser) string {
		return it.UUID
	}), common.Map(h.users, func(it option.VLESSUser) string {
		return it.Flow
	}))
	return nil
}

func (h *VLESS) DelUsers(name []string) error {
	is := make([]int, 0, len(name))
	ulen := len(name)
	for i := range h.users {
		for _, u := range name {
			if h.users[i].Name == u {
				is = append(is, i)
				ulen--
			}
			if ulen == 0 {
				break
			}
		}
	}
	ulen = len(h.users)
	for _, i := range is {
		h.users[i] = h.users[ulen-1]
		h.users[ulen-1] = option.VLESSUser{}
		h.users = h.users[:ulen-1]
		ulen--
	}
	h.service.UpdateUsers(common.MapIndexed(h.users, func(index int, it option.VLESSUser) int {
		return index
	}), common.Map(h.users, func(it option.VLESSUser) string {
		return it.UUID
	}), common.Map(h.users, func(it option.VLESSUser) string {
		return it.Flow
	}))
	return nil
}

func (h *VLESS) Close() error {
	return common.Close(
		h.service,
		&h.myInboundAdapter,
		h.tlsConfig,
		h.transport,
	)
}

func (h *VLESS) newTransportConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	h.injectTCP(conn, metadata)
	return nil
}

func (h *VLESS) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	var err error
	if h.tlsConfig != nil && h.transport == nil {
		conn, err = tls.ServerHandshake(ctx, conn, h.tlsConfig)
		if err != nil {
			return err
		}
	}
	return h.service.NewConnection(adapter.WithContext(log.ContextWithNewID(ctx), &metadata), conn, adapter.UpstreamMetadata(metadata))
}

func (h *VLESS) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	return os.ErrInvalid
}

func (h *VLESS) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	userIndex, loaded := auth.UserFromContext[int](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	user := h.users[userIndex].Name
	if user == "" {
		user = F.ToString(userIndex)
	} else {
		metadata.User = user
	}
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *VLESS) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	userIndex, loaded := auth.UserFromContext[int](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	user := h.users[userIndex].Name
	if user == "" {
		user = F.ToString(userIndex)
	} else {
		metadata.User = user
	}
	if metadata.Destination.Fqdn == packetaddr.SeqPacketMagicAddress {
		metadata.Destination = M.Socksaddr{}
		conn = packetaddr.NewConn(conn.(vmess.PacketConn), metadata.Destination)
		h.logger.InfoContext(ctx, "[", user, "] inbound packet addr connection")
	} else {
		h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	}
	return h.router.RoutePacketConnection(ctx, conn, metadata)
}

var _ adapter.V2RayServerTransportHandler = (*vlessTransportHandler)(nil)

type vlessTransportHandler VLESS

func (t *vlessTransportHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return (*VLESS)(t).newTransportConnection(ctx, conn, adapter.InboundContext{
		Source:      metadata.Source,
		Destination: metadata.Destination,
	})
}

func (t *vlessTransportHandler) FallbackConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return os.ErrInvalid
}
