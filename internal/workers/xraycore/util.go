package main

import (
	"encoding/json"

	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/infra/conf"
)

func parseAddr(s string) *conf.Address {
	return &conf.Address{Address: net.ParseAddress(s)}
}

func settingsRawMessage(v any) (*json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(b)
	return &raw, nil
}
