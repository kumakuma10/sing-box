//go:build with_quic

package inbound

import (
	"context"
	"net"
	"time"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/tls"
	"github.com/kumakuma10/sing-box/common/uot"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	"github.com/sagernet/sing-quic/tuic"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"

	"github.com/gofrs/uuid/v5"
)

var _ adapter.Inbound = (*TUIC)(nil)

type TUIC struct {
	myInboundAdapter
	tlsConfig tls.ServerConfig
	server    *tuic.Service[string]
	users     map[string]option.TUICUser
}

func NewTUIC(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.TUICInboundOptions) (*TUIC, error) {
	options.UDPFragmentDefault = true
	if options.TLS == nil || !options.TLS.Enabled {
		return nil, C.ErrTLSRequired
	}
	tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
	if err != nil {
		return nil, err
	}
	inbound := &TUIC{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeTUIC,
			network:       []string{N.NetworkUDP},
			ctx:           ctx,
			router:        uot.NewRouter(router, logger),
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		tlsConfig: tlsConfig,
	}
	service, err := tuic.NewService[string](tuic.ServiceOptions{
		Context:           ctx,
		Logger:            logger,
		TLSConfig:         tlsConfig,
		CongestionControl: options.CongestionControl,
		AuthTimeout:       time.Duration(options.AuthTimeout),
		ZeroRTTHandshake:  options.ZeroRTTHandshake,
		Heartbeat:         time.Duration(options.Heartbeat),
		Handler:           adapter.NewUpstreamHandler(adapter.InboundContext{}, inbound.newConnection, inbound.newPacketConnection, nil),
	})
	if err != nil {
		return nil, err
	}
	users := make(map[string]option.TUICUser)
	var userList []string
	var userUUIDList [][16]byte
	var userPasswordList []string
	for _, user := range options.Users {
		if user.UUID == "" {
			return nil, E.New("missing uuid for user ", user.UUID)
		}
		if user.Name == "" {
			user.Name = user.UUID
		}
		userUUID, err := uuid.FromString(user.UUID)
		if err != nil {
			return nil, E.Cause(err, "invalid uuid for user ", user.UUID)
		}
		users[user.Name] = user
		userList = append(userList, user.Name)
		userUUIDList = append(userUUIDList, userUUID)
		userPasswordList = append(userPasswordList, user.Password)
	}
	service.UpdateUsers(userList, userUUIDList, userPasswordList)
	inbound.server = service
	inbound.users = users
	return inbound, nil
}

func (h *TUIC) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
	ctx = log.ContextWithNewID(ctx)
	metadata = h.createMetadata(conn, metadata)
	user, _ := auth.UserFromContext[string](ctx)
	if _, exist := h.users[user]; !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound connection to ", metadata.Destination)
	return h.router.RouteConnection(ctx, conn, metadata)
}

func (h *TUIC) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
	ctx = log.ContextWithNewID(ctx)
	metadata = h.createPacketMetadata(conn, metadata)
	user, _ := auth.UserFromContext[string](ctx)
	if _, exist := h.users[user]; !exist {
		return E.New("user not exist")
	}
	metadata.User = user
	h.logger.InfoContext(ctx, "[", user, "] inbound packet connection to ", metadata.Destination)
	return h.router.RoutePacketConnection(ctx, conn, metadata)
}

func (h *TUIC) Start() error {
	if h.tlsConfig != nil {
		err := h.tlsConfig.Start()
		if err != nil {
			return err
		}
	}
	packetConn, err := h.myInboundAdapter.ListenUDP()
	if err != nil {
		return err
	}
	return h.server.Start(packetConn)
}

func (h *TUIC) Close() error {
	return common.Close(
		&h.myInboundAdapter,
		h.tlsConfig,
		common.PtrOrNil(h.server),
	)
}

// v2bx

func (h *TUIC) updateUsers() error {
	userNames := make([]string, 0, len(h.users))
	userUUIDs := make([][16]byte, 0, len(h.users))
	userPasswords := make([]string, 0, len(h.users))
	for n, u := range h.users {
		userUUID, err := uuid.FromString(u.UUID)
		if err != nil {
			return E.Cause(err, "invalid uuid for user ", u.UUID)
		}
		userNames = append(userNames, n)
		userUUIDs = append(userUUIDs, userUUID)
		userPasswords = append(userPasswords, u.Password)
	}
	h.server.UpdateUsers(userNames, userUUIDs, userPasswords)
	return nil
}

func (h *TUIC) AddUsers(users []option.TUICUser) error {
	for _, u := range users {
		h.users[u.Name] = u
	}

	return h.updateUsers()

}

func (h *TUIC) DelUsers(names []string) error {
	for _, n := range names {
		delete(h.users, n)
	}

	return h.updateUsers()
}
