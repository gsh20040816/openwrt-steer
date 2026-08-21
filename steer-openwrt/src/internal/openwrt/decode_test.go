// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"strings"
	"testing"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

const minimalConfig = `
config steer 'main'
	option schema_version '5'
	option enabled '1'
	option log_level 'warn'

config bootstrap 'bootstrap'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'

config route 'direct'
	option kind 'direct'

config dns_profile 'direct_dns'
	option protocol 'udp'
	option server '1.1.1.1'
	option server_port '53'
	option strategy 'prefer_ipv4'

config rule 'default'
	option default '1'
	option dns_profile 'direct_dns'
	option route 'direct'
`

func TestDecodeCanonicalIntent(t *testing.T) {
	intent, err := Decode(strings.NewReader(minimalConfig))
	if err != nil {
		t.Fatal(err)
	}
	if intent.Main.SchemaVersion != 5 || !intent.Main.Enabled || len(intent.Main.ProbeURLs) != 3 {
		t.Fatalf("unexpected main: %#v", intent.Main)
	}
	if validation := model.Validate(intent); !validation.OK {
		t.Fatalf("decoded intent is invalid: %#v", validation.Errors)
	}
}

func TestDecodeRejectsImplementationAndLegacyFields(t *testing.T) {
	for _, field := range []string{"router_proxy", "tproxy_port", "response_mode", "outbound"} {
		config := minimalConfig
		switch field {
		case "response_mode":
			config = strings.Replace(config, "option protocol 'udp'\n\toption server '1.1.1.1'", "option protocol 'udp'\n\toption response_mode 'fastest-ip'\n\toption server '1.1.1.1'", 1)
		case "outbound":
			config = strings.Replace(config, "option route 'direct'", "option outbound 'direct'", 1)
		default:
			config = strings.Replace(config, "option log_level 'warn'", "option log_level 'warn'\n\toption "+field+" '1'", 1)
		}
		if _, err := Decode(strings.NewReader(config)); err == nil {
			t.Fatalf("legacy field %s was accepted", field)
		}
	}
}

func TestDecodeRejectsWrongScalarShape(t *testing.T) {
	config := strings.Replace(minimalConfig, "option log_level 'warn'", "option managed_zone 'lan'\n\toption log_level 'warn'", 1)
	if _, err := Decode(strings.NewReader(config)); err == nil {
		t.Fatal("removed managed_zone option must be rejected")
	}
}
