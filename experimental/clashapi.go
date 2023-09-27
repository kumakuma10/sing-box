package experimental

import (
	"context"
	"github.com/kumakuma10/sing-box/adapter"
	"github.com/kumakuma10/sing-box/log"
	"github.com/kumakuma10/sing-box/option"
	"github.com/sagernet/sing/common"
	"os"
)

type ClashServerConstructor = func(ctx context.Context, router adapter.Router, logFactory log.ObservableFactory, options option.ClashAPIOptions) (adapter.ClashServer, error)

var clashServerConstructor ClashServerConstructor

func RegisterClashServerConstructor(constructor ClashServerConstructor) {
	clashServerConstructor = constructor
}

func NewClashServer(ctx context.Context, router adapter.Router, logFactory log.ObservableFactory, options option.ClashAPIOptions) (adapter.ClashServer, error) {
	if clashServerConstructor == nil {
		return nil, os.ErrInvalid
	}
	return clashServerConstructor(ctx, router, logFactory, options)
}

func CalculateClashModeList(options option.Options) []string {
	var clashMode []string
	for _, dnsRule := range common.PtrValueOrDefault(options.DNS).Rules {
		if dnsRule.DefaultOptions.ClashMode != "" && !common.Contains(clashMode, dnsRule.DefaultOptions.ClashMode) {
			clashMode = append(clashMode, dnsRule.DefaultOptions.ClashMode)
		}
		for _, defaultRule := range dnsRule.LogicalOptions.Rules {
			if defaultRule.ClashMode != "" && !common.Contains(clashMode, defaultRule.ClashMode) {
				clashMode = append(clashMode, defaultRule.ClashMode)
			}
		}
	}
	for _, rule := range common.PtrValueOrDefault(options.Route).Rules {
		if rule.DefaultOptions.ClashMode != "" && !common.Contains(clashMode, rule.DefaultOptions.ClashMode) {
			clashMode = append(clashMode, rule.DefaultOptions.ClashMode)
		}
		for _, defaultRule := range rule.LogicalOptions.Rules {
			if defaultRule.ClashMode != "" && !common.Contains(clashMode, defaultRule.ClashMode) {
				clashMode = append(clashMode, defaultRule.ClashMode)
			}
		}
	}
	return clashMode
}
