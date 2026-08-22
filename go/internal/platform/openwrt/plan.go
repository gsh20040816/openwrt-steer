// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	model "github.com/gsh20040816/openwrt-steer/go/internal/intent"
)

const (
	TunInterface = "steer0"
	DNSPort      = 1053
	// MAC policy packets use a distinct mark so locally generated
	// auto-redirect traffic cannot enter the MAC policy table.
	MACMark                = 0x2026
	MACTable               = 2023
	MACPriority            = 8999
	TunTable               = 2022
	TunPriority            = 9000
	TunFallbackPriority    = 32768
	AutoRedirectInputMark  = 0x2023
	AutoRedirectOutputMark = 0x2024
	AutoRedirectResetMark  = 0x2025
	AutoRedirectNFQueue    = 100
)

var nonGlobalIPv4 = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
	"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
}

var nonGlobalIPv6 = []string{
	"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64",
	"2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fc00::/7", "fe80::/10", "ff00::/8",
}

type Plan struct {
	SchemaVersion int       `json:"schema_version"`
	Resources     Resources `json:"resources"`
}

type Resources struct {
	TunInterface           string       `json:"tun_interface"`
	TunAddresses           []string     `json:"tun_addresses"`
	DNSPort                int          `json:"dns_port"`
	TunTable               int          `json:"tun_table"`
	TunPriority            int          `json:"tun_priority"`
	TunFallbackPriority    int          `json:"tun_fallback_priority"`
	AutoRedirectInputMark  int          `json:"auto_redirect_input_mark"`
	AutoRedirectOutputMark int          `json:"auto_redirect_output_mark"`
	AutoRedirectResetMark  int          `json:"auto_redirect_reset_mark"`
	AutoRedirectNFQueue    int          `json:"auto_redirect_nfqueue"`
	MACMark                int          `json:"mac_mark,omitempty"`
	MACTable               int          `json:"mac_table,omitempty"`
	MACPriority            int          `json:"mac_priority,omitempty"`
	MACBindings            []MACBinding `json:"mac_bindings"`
}

type MACBinding struct {
	Address       string `json:"address"`
	TProxyPort    int    `json:"tproxy_port"`
	DNSPort       int    `json:"dns_port"`
	TProxyTag     string `json:"tproxy_tag"`
	DNSInboundTag string `json:"dns_inbound_tag"`
}

func NewPlan(value model.Intent) Plan {
	return Plan{SchemaVersion: 1, Resources: Resources{
		TunInterface: TunInterface,
		TunAddresses: []string{"198.18.0.1/30", "fdfe:dcba:9876::1/126"},
		DNSPort:      DNSPort, TunTable: TunTable, TunPriority: TunPriority,
		TunFallbackPriority: TunFallbackPriority, AutoRedirectInputMark: AutoRedirectInputMark,
		AutoRedirectOutputMark: AutoRedirectOutputMark, AutoRedirectResetMark: AutoRedirectResetMark,
		AutoRedirectNFQueue: AutoRedirectNFQueue, MACMark: MACMark, MACTable: MACTable,
		MACPriority: MACPriority, MACBindings: allocateMACBindings(value),
	}}
}

func (plan Plan) CompilerTarget() compiler.Target {
	inbounds := []any{
		map[string]any{
			"type": "tun", "tag": "steer-tun", "interface_name": plan.Resources.TunInterface,
			"address": plan.Resources.TunAddresses, "mtu": 9000, "auto_route": true,
			"strict_route": true, "auto_redirect": true, "stack": "system",
			"iproute2_table_index": plan.Resources.TunTable, "iproute2_rule_index": plan.Resources.TunPriority,
			"auto_redirect_iproute2_fallback_rule_index": plan.Resources.TunFallbackPriority,
			"auto_redirect_input_mark":                   plan.Resources.AutoRedirectInputMark,
			"auto_redirect_output_mark":                  plan.Resources.AutoRedirectOutputMark,
			"auto_redirect_reset_mark":                   plan.Resources.AutoRedirectResetMark,
			"auto_redirect_nfqueue":                      plan.Resources.AutoRedirectNFQueue,
			"route_exclude_address":                      append(append([]string{}, nonGlobalIPv4...), nonGlobalIPv6...),
		},
		map[string]any{
			"type": "direct", "tag": "steer-dns", "listen": "::", "listen_port": plan.Resources.DNSPort,
			"network": []string{"tcp", "udp"},
		},
	}
	target := compiler.Target{
		Inbounds: inbounds, DNSInboundTags: []string{"steer-dns"}, SniffInboundTags: []string{"steer-tun"},
		RequiredCapabilities: []string{"tun", "auto_route", "auto_redirect"},
	}
	for _, binding := range plan.Resources.MACBindings {
		target.Inbounds = append(target.Inbounds,
			map[string]any{"type": "tproxy", "tag": binding.TProxyTag, "listen": "::", "listen_port": binding.TProxyPort, "network": []string{"tcp", "udp"}},
			map[string]any{"type": "direct", "tag": binding.DNSInboundTag, "listen": "::", "listen_port": binding.DNSPort, "network": []string{"tcp", "udp"}},
		)
		target.DNSInboundTags = append(target.DNSInboundTags, binding.DNSInboundTag)
		target.SniffInboundTags = append(target.SniffInboundTags, binding.TProxyTag)
		target.MACBindings = append(target.MACBindings, compiler.MACBinding{
			Address: binding.Address, TProxyTag: binding.TProxyTag, DNSInboundTag: binding.DNSInboundTag,
		})
	}
	if len(plan.Resources.MACBindings) > 0 {
		target.RequiredCapabilities = append(target.RequiredCapabilities, "tproxy")
	}
	return target
}

func allocateMACBindings(value model.Intent) []MACBinding {
	seen := map[string]bool{}
	var addresses []string
	for _, rule := range value.Rules {
		if !rule.Enabled || rule.Default {
			continue
		}
		for _, address := range rule.SourceMACAddress {
			address = strings.ToLower(address)
			if !seen[address] {
				seen[address] = true
				addresses = append(addresses, address)
			}
		}
	}
	sort.Strings(addresses)
	occupied := map[int]bool{DNSPort: true}
	for _, proxy := range value.LocalProxies {
		if proxy.Enabled {
			occupied[proxy.ListenPort] = true
		}
	}
	nextPort := 20000
	allocate := func() int {
		for occupied[nextPort] {
			nextPort++
		}
		result := nextPort
		occupied[result] = true
		nextPort++
		return result
	}
	bindings := make([]MACBinding, 0, len(addresses))
	for index, address := range addresses {
		bindings = append(bindings, MACBinding{Address: address, TProxyPort: allocate(), DNSPort: allocate(),
			TProxyTag: fmt.Sprintf("steer-mac-tproxy-%d", index), DNSInboundTag: fmt.Sprintf("steer-mac-dns-%d", index)})
	}
	return bindings
}
