package main

import (
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/infra/conf"
)

func parseAddr(s string) *conf.Address {
	return &conf.Address{Address: net.ParseAddress(s)}
}
