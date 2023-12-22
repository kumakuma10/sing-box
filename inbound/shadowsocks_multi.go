package inbound

import (
	"context"
	"net"
	"os"
	"time"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/mux"
	"github.com/kumakuma10/sing-box/common/uot"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	"github.com/sagernet/sing-shadowsocks"
	"github.com/sagernet/sing-shadowsocks/shadowaead"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
)

var (
	_ adapter.Inbound           = (*ShadowsocksMulti)(nil)
	_ adapter.InjectableInbound = (*ShadowsocksMulti)(nil)
)

type ShadowsocksMulti struct {
	myInboundAdapter
	service shadowsocks.MultiService[string]
	users   map[string]string // [name]pass
}

func newShadowsocksMulti(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.ShadowsocksInboundOptions) (*ShadowsocksMulti, error) {
	inbound := &ShadowsocksMulti{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeShadowsocks,
			network:       options.Network.Build(),
			ctx:           ctx,
			router:        uot.NewRouter(router, logger),
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
	}
	inbound.connHandler = inbound
	inbound.packetHandler = inbound
	var err error
	inbound.router, err = mux.NewRouterWithOptions(inbound.router, logger, common.PtrValueOrDefault(options.Multiplex))
	if err != nil {
		return nil, err
	}
	var udpTimeout time.Duration
	if options.UDPTimeout != 0 {
		udpTimeout = time.Duration(options.UDPTimeout)
	} else {
		udpTimeout = C.UDPTimeout
	}
	var service shadowsocks.MultiService[string]
	if common.Contains(shadowaead_2022.List, options.Method) {
		service, err = shadowaead_2022.NewMultiServiceWithPassword[string](
			options.Method,
			options.Password,
			int64(udpTimeout.Seconds()),
			adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound),
			ntp.TimeFuncFromContext(ctx),
		)
	} else if common.Contains(shadowaead.List, options.Method) {
		service, err = shadowaead.NewMultiService[string](
			options.Method,
			int64(udpTimeout.Seconds()),
			adapter.NewUpstreamContextHandler(inbound.newConnection, inbound.newPacketConnection, inbound))
	} else {
		return nil, E.New("unsupported method: " + options.Method)
	}
	if err != nil {
		return nil, err
	}
	err = service.UpdateUsersWithPasswords(common.Map(options.Users, func(user option.ShadowsocksUser) string {
		if user.Name != "" {
			return user.Name
		}
		return user.Password

	}), common.Map(options.Users, func(user option.ShadowsocksUser) string {
		return user.Password
	}))
	if err != nil {
		return nil, err
	}
	users := make(map[string]string)
	for _, v := range options.Users {
		users[v.Name] = v.Password
	}
	inbound.service = service
	inbound.packetUpstream = service
	inbound.users = users
	return inbound, err
}

func (h *ShadowsocksMulti) updateUsers() error {
	userList := make([]string, 0, len(h.users))
	passwordList := make([]string, 0, len(h.users))
	for u, p := range h.users {
		userList = append(userList, u)
		passwordList = append(passwordList, p)
	}
	return h.service.UpdateUsersWithPasswords(userList, passwordList)
}

func (h *ShadowsocksMulti) AddUsers(users []option.ShadowsocksUser) error {
	for _, u := range users {
		h.users[u.Name] = u.Password
	}
	err := h.updateUsers()
	if err != nil {
		return err
	}
	return nil
}

func (h *ShadowsocksMulti) DelUsers(name []string) error {
	for _, u := range name {
		delete(h.users, u)
	}
	err := h.updateUsers()
	return err
}

func (h *ShadowsocksMulti) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	return h.service.NewConnection(adapter.WithContext(log.ContextWithNewID(ctx), &metadata), conn, adapter.UpstreamMetadata(metadata))
}

func (h *ShadowsocksMulti) NewPacket(ctx context.Context, conn N.PacketConn, buffer *buf.Buffer, metadata adapter.InboundContext) error {
	return h.service.NewPacket(adapter.WithContext(ctx, &metadata), conn, buffer, adapter.UpstreamMetadata(metadata))
}

func (h *ShadowsocksMulti) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	return os.ErrInvalid
}

func (h *ShadowsocksMulti) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	user, loaded := auth.UserFromContext[string](ctx)
	if !loaded || user == "" {
		return os.ErrInvalid
	}
	_, exist := h.users[user]
	if !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *ShadowsocksMulti) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	user, loaded := auth.UserFromContext[string](ctx)
	if !loaded || user == "" {
		return os.ErrInvalid
	}
	_, exist := h.users[user]
	if !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	ctx = log.ContextWithNewID(ctx)
	h.logger.InfoContext(ctx, "[", user, "] inbound packet connection from ", metadata.Source)
	h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	return h.router.RoutePacketConnection(ctx, conn, metadata)
}
