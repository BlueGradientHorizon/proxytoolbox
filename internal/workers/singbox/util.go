package main

import (
	"context"
	"crypto/tls"

	"github.com/bluegradienthorizon/proxytoolbox/measure"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/ntp"
)

func CreateTLSConfigProvider() measure.TLSConfigProvider {
	return func(ctx context.Context) *tls.Config {
		return &tls.Config{
			Time:    ntp.TimeFuncFromContext(ctx),
			RootCAs: adapter.RootPoolFromContext(ctx),
		}
	}
}
