package inbound

import (
	"context"
	"net"
	"os"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/mux"
	"github.com/kumakuma10/sing-box/common/tls"
	"github.com/kumakuma10/sing-box/common/uot"
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
	users     map[string]option.VLESSUser
	service   *vless.Service[string]
	tlsConfig tls.ServerConfig
	transport adapter.V2RayServerTransport
}

func NewVLESS(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.VLESSInboundOptions) (*VLESS, error) {
	inbound := &VLESS{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeVLESS,
			network:       []string{N.NetworkTCP},
			ctx:           ctx,
			router:        uot.NewRouter(router, logger),
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		ctx: ctx,
	}
	var err error
	inbound.router, err = mux.NewRouterWithOptions(inbound.router, logger, common.PtrValueOrDefault(options.Multiplex))
	if err != nil {
		return nil, err
	}
	service := vless.NewService[string](logger, adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound))
	users := make(map[string]option.VLESSUser)
	userNameList := make([]string, 0, len(inbound.users))
	userUUIDList := make([]string, 0, len(inbound.users))
	userFlowList := make([]string, 0, len(inbound.users))
	for _, user := range inbound.users {
		name := user.Name
		if name == "" {
			name = user.Name
		}
		userNameList = append(userNameList, name)
		userUUIDList = append(userUUIDList, user.UUID)
		userFlowList = append(userFlowList, user.Flow)
		users[name] = user

	}
	service.UpdateUsers(userNameList, userUUIDList, userFlowList)
	inbound.service = service
	inbound.users = users
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

func (h *VLESS) updateUsers() error {
	userNameList := make([]string, 0, len(h.users))
	userUUIDList := make([]string, 0, len(h.users))
	userFlowList := make([]string, 0, len(h.users))
	for _, user := range h.users {
		name := user.Name
		if name == "" {
			name = user.Name
		}
		userNameList = append(userNameList, name)
		userUUIDList = append(userUUIDList, user.UUID)
		userFlowList = append(userFlowList, user.Flow)
	}
	h.service.UpdateUsers(userNameList, userUUIDList, userFlowList)
	return nil

}

func (h *VLESS) AddUsers(users []option.VLESSUser) error {
	for _, u := range users {
		h.users[u.Name] = u
	}
	return h.updateUsers()
}

func (h *VLESS) DelUsers(name []string) error {
	for _, n := range name {
		delete(h.users, n)
	}
	return h.updateUsers()
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

func (h *VLESS) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
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

var _ adapter.V2RayServerTransportHandler = (*vlessTransportHandler)(nil)

type vlessTransportHandler VLESS

func (t *vlessTransportHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return (*VLESS)(t).newTransportConnection(ctx, conn, adapter.InboundContext{
		Source:      metadata.Source,
		Destination: metadata.Destination,
	})
}
