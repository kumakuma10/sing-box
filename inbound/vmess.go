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
	vmess "github.com/sagernet/sing-vmess"
	"github.com/sagernet/sing-vmess/packetaddr"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
)

var (
	_ adapter.Inbound           = (*VMess)(nil)
	_ adapter.InjectableInbound = (*VMess)(nil)
)

type VMess struct {
	myInboundAdapter
	ctx       context.Context
	service   *vmess.Service[string]
	users     map[string]option.VMessUser
	tlsConfig tls.ServerConfig
	transport adapter.V2RayServerTransport
}

func NewVMess(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VMessInboundOptions) (*VMess, error) {
	inbound := &VMess{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeVMess,
			network:       []string{N.NetworkTCP},
			ctx:           ctx,
			router:        router,
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		ctx: ctx,
	}
	var serviceOptions []vmess.ServiceOption
	if timeFunc := ntp.TimeFuncFromContext(ctx); timeFunc != nil {
		serviceOptions = append(serviceOptions, vmess.ServiceWithTimeFunc(timeFunc))
	}
	if options.Transport != nil && options.Transport.Type != "" {
		serviceOptions = append(serviceOptions, vmess.ServiceWithDisableHeaderProtection())
	}
	service := vmess.NewService[string](adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound), serviceOptions...)
	inbound.service = service
	users := make(map[string]option.VMessUser)
	for _, user := range options.Users {
		name := user.Name
		if user.Name == "" {
			name = user.UUID

		}
		users[name] = user
	}
	inbound.users = users
	err := service.UpdateUsers(common.Map(options.Users, func(it option.VMessUser) string {
		if it.Name == "" {
			return it.UUID
		}
		return it.Name

	}), common.Map(options.Users, func(it option.VMessUser) string {
		return it.UUID
	}), common.Map(options.Users, func(it option.VMessUser) int {
		return it.AlterId
	}))
	if err != nil {
		return nil, err
	}
	if options.TLS != nil {
		inbound.tlsConfig, err = tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
		if err != nil {
			return nil, err
		}
	}
	if options.Transport != nil {
		inbound.transport, err = v2ray.NewServerTransport(ctx, common.PtrValueOrDefault(options.Transport), inbound.tlsConfig, (*vmessTransportHandler)(inbound))
		if err != nil {
			return nil, E.Cause(err, "create server transport: ", options.Transport.Type)
		}
	}
	inbound.connHandler = inbound
	return inbound, nil
}

func (h *VMess) Start() error {
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

func (h *VMess) updateUsers() error {
	userNames := make([]string, 0, len(h.users))
	userIds := make([]string, 0, len(h.users))
	userAlterIds := make([]int, 0, len(h.users))
	for _, u := range h.users {
		userNames = append(userNames, u.Name)
		userIds = append(userIds, u.UUID)
		userAlterIds = append(userAlterIds, u.AlterId)
	}
	return h.service.UpdateUsers(userNames, userIds, userAlterIds)
}

func (h *VMess) AddUsers(users []option.VMessUser) error {
	for _, u := range users {
		h.users[u.Name] = u

	}
	return h.updateUsers()
}

func (h *VMess) DelUsers(name []string) error {
	for _, n := range name {
		delete(h.users, n)
	}
	return h.updateUsers()
}

func (h *VMess) Close() error {
	return common.Close(
		h.service,
		&h.myInboundAdapter,
		h.tlsConfig,
		h.transport,
	)
}

func (h *VMess) newTransportConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	h.injectTCP(conn, metadata)
	return nil
}

func (h *VMess) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	var err error
	if h.tlsConfig != nil && h.transport == nil {
		conn, err = tls.ServerHandshake(ctx, conn, h.tlsConfig)
		if err != nil {
			return err
		}
	}
	return h.service.NewConnection(adapter.WithContext(log.ContextWithNewID(ctx), &metadata), conn, adapter.UpstreamMetadata(metadata))
}

func (h *VMess) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	return os.ErrInvalid
}

func (h *VMess) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	user, loaded := auth.UserFromContext[string](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	if _, exist := h.users[user]; !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *VMess) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	user, loaded := auth.UserFromContext[string](ctx)
	if !loaded {
		return os.ErrInvalid
	}
	if _, exist := h.users[user]; !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	if metadata.Destination.Fqdn == packetaddr.SeqPacketMagicAddress {
		metadata.Destination = M.Socksaddr{}
		conn = packetaddr.NewConn(conn.(vmess.PacketConn), metadata.Destination)
		h.logger.InfoContext(ctx, "[", user, "] inbound packet addr connection")
	} else {
		h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	}
	return h.router.RoutePacketConnection(ctx, conn, metadata)
}

var _ adapter.V2RayServerTransportHandler = (*vmessTransportHandler)(nil)

type vmessTransportHandler VMess

func (t *vmessTransportHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return (*VMess)(t).newTransportConnection(ctx, conn, adapter.InboundContext{
		Source:      metadata.Source,
		Destination: metadata.Destination,
	})
}
