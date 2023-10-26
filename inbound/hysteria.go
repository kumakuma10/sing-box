//go:build with_quic

package inbound

import (
	"context"
	"net"

	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/common/humanize"
	"github.com/kumakuma10/sing-box/common/tls"
	C "github.com/kumakuma10/sing-box/constant"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	"github.com/sagernet/sing-quic/hysteria"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/auth"
	E "github.com/sagernet/sing/common/exceptions"
	N "github.com/sagernet/sing/common/network"
)

var _ adapter.Inbound = (*Hysteria)(nil)

type Hysteria struct {
	myInboundAdapter
	tlsConfig tls.ServerConfig
	service   *hysteria.Service[string]
	users     map[string]string
}

func NewHysteria(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.HysteriaInboundOptions) (*Hysteria, error) {
	options.UDPFragmentDefault = true
	if options.TLS == nil || !options.TLS.Enabled {
		return nil, C.ErrTLSRequired
	}
	tlsConfig, err := tls.NewServer(ctx, logger, common.PtrValueOrDefault(options.TLS))
	if err != nil {
		return nil, err
	}
	inbound := &Hysteria{
		myInboundAdapter: myInboundAdapter{
			protocol:      C.TypeHysteria,
			network:       []string{N.NetworkUDP},
			ctx:           ctx,
			router:        router,
			logger:        logger,
			tag:           tag,
			listenOptions: options.ListenOptions,
		},
		tlsConfig: tlsConfig,
	}
	var sendBps, receiveBps uint64
	if len(options.Up) > 0 {
		sendBps, err = humanize.ParseBytes(options.Up)
		if err != nil {
			return nil, E.Cause(err, "invalid up speed format: ", options.Up)
		}
	} else {
		sendBps = uint64(options.UpMbps) * hysteria.MbpsToBps
	}
	if len(options.Down) > 0 {
		receiveBps, err = humanize.ParseBytes(options.Down)
		if receiveBps == 0 {
			return nil, E.New("invalid down speed format: ", options.Down)
		}
	} else {
		receiveBps = uint64(options.DownMbps) * hysteria.MbpsToBps
	}
	service, err := hysteria.NewService[string](hysteria.ServiceOptions{
		Context:       ctx,
		Logger:        logger,
		SendBPS:       sendBps,
		ReceiveBPS:    receiveBps,
		XPlusPassword: options.Obfs,
		TLSConfig:     tlsConfig,
		Handler:       adapter.NewUpstreamHandler(adapter.InboundContext{}, inbound.newConnection, inbound.newPacketConnection, nil),

		// Legacy options

		ConnReceiveWindow:   options.ReceiveWindowConn,
		StreamReceiveWindow: options.ReceiveWindowClient,
		MaxIncomingStreams:  int64(options.MaxConnClient),
		DisableMTUDiscovery: options.DisableMTUDiscovery,
	})
	if err != nil {
		return nil, err
	}
	// userList := make([]int, 0, len(options.Users))
	users := make(map[string]string)
	userNameList := make([]string, 0, len(options.Users))
	userPasswordList := make([]string, 0, len(options.Users))
	for _, user := range options.Users {
		var password string
		if user.AuthString != "" {
			password = user.AuthString
		} else {
			password = string(user.Auth)
		}
		var name string
		if user.Name == "" {
			name = password
		} else {
			name = user.Name
		}
		userNameList = append(userNameList, name)
		userPasswordList = append(userPasswordList, password)
		users[name] = password
	}
	service.UpdateUsers(userNameList, userPasswordList)
	inbound.service = service
	inbound.users = users
	return inbound, nil
}

func (h *Hysteria) newConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) error {
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

func (h *Hysteria) newPacketConnection(ctx context.Context, conn N.PacketConn, metadata adapter.InboundContext) error {
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

func (h *Hysteria) updateUsers() {
	userNames := make([]string, 0, len(h.users))
	userPasswords := make([]string, 0, len(h.users))
	for u, p := range h.users {
		userNames = append(userNames, u)
		userPasswords = append(userPasswords, p)
	}
	h.service.UpdateUsers(userNames, userPasswords)
}

func (h *Hysteria) AddUsers(users []option.HysteriaUser) error {
	for _, u := range users {
		if u.AuthString != "" {
			h.users[u.Name] = u.AuthString
		} else {
			h.users[u.Name] = string(u.Auth)
		}
	}

	h.updateUsers()
	return nil
}

func (h *Hysteria) DelUsers(names []string) error {
	for _, n := range names {
		delete(h.users, n)
	}

	h.updateUsers()
	return nil
}

func (h *Hysteria) Start() error {
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
	return h.service.Start(packetConn)
}

func (h *Hysteria) Close() error {
	return common.Close(
		&h.myInboundAdapter,
		h.tlsConfig,
		common.PtrOrNil(h.service),
	)
}
