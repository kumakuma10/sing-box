//go:build with_quic

package include

import (
	_ "github.com/kumakuma10/sing-box/transport/v2rayquic"
	_ "github.com/sagernet/sing-dns/quic"
)

const WithQUIC = true
