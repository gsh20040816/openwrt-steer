// SPDX-License-Identifier: GPL-3.0-or-later

// Package macos contains the platform-owned configuration contract for the
// future NetworkExtension adapter. It intentionally has no Swift or Darwin
// dependency so the compiler boundary can be tested on Linux.
package macos

import (
	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

const (
	DNSPort  = 1053
	DNSPort6 = 1054
)

type Plan struct {
	SchemaVersion int       `json:"schema_version"`
	Resources     Resources `json:"resources"`
}

type Resources struct {
	TunAddresses []string `json:"tun_addresses"`
	DNSPort      int      `json:"dns_port"`
	DNSPort6     int      `json:"dns_port6"`
}

func NewPlan(_ model.Intent) Plan {
	return Plan{SchemaVersion: 1, Resources: Resources{
		TunAddresses: []string{"198.18.0.1/30", "fdfe:dcba:9876::1/126"},
		DNSPort:      DNSPort, DNSPort6: DNSPort6,
	}}
}

// CompilerTarget describes the sing-box-facing part of the macOS runtime.
// The NetworkExtension provider owns the utun interface and system routes;
// therefore this target deliberately omits auto_route, auto_redirect and
// interface_name. DNSProxyProvider forwards only captured DNS flows to the
// loopback DNS inbounds below, where hijack-dns hands them to sing-box's DNS
// router.
func (plan Plan) CompilerTarget() compiler.Target {
	inbounds := []any{
		map[string]any{
			"type": "tun", "tag": "steer-tun", "address": plan.Resources.TunAddresses, "mtu": 9000,
		},
		map[string]any{
			"type": "direct", "tag": "steer-dns4", "listen": "127.0.0.1", "listen_port": plan.Resources.DNSPort,
			"network": []string{"tcp", "udp"},
		},
		map[string]any{
			"type": "direct", "tag": "steer-dns6", "listen": "::1", "listen_port": plan.Resources.DNSPort6,
			"network": []string{"tcp", "udp"},
		},
	}
	return compiler.Target{
		Inbounds: inbounds,
		DNSCapture: compiler.DNSCapture{
			Mode:        compiler.DNSCaptureInboundHijack,
			InboundTags: []string{"steer-dns4", "steer-dns6"},
		},
		SniffInboundTags:     []string{"steer-tun"},
		RequiredCapabilities: []string{"tun"},
	}
}

func (plan Plan) CompilerOptions(stateDirectory string) compiler.Options {
	return compiler.Options{StateDirectory: stateDirectory, Target: plan.CompilerTarget()}
}
