// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"net"
	"net/netip"
	"reflect"
	"testing"
)

func TestActiveLANPrefixesUsesOnlyUpLANInterfaces(t *testing.T) {
	result := activeLANPrefixes([]networkSnapshot{
		{Name: "en0", Flags: net.FlagUp | net.FlagBroadcast | net.FlagMulticast, Prefixes: []netip.Prefix{
			netip.MustParsePrefix("192.168.50.23/24"), netip.MustParsePrefix("fd12:3456:789a::42/64"),
		}},
		{Name: "en7", Flags: net.FlagUp | net.FlagBroadcast, Prefixes: []netip.Prefix{netip.MustParsePrefix("100.64.12.4/24")}},
		{Name: "utun7", Flags: net.FlagUp | net.FlagPointToPoint, Prefixes: []netip.Prefix{netip.MustParsePrefix("10.0.0.2/24")}},
		{Name: "awdl0", Flags: net.FlagUp | net.FlagMulticast, Prefixes: []netip.Prefix{netip.MustParsePrefix("fd00::1/64")}},
		{Name: "en8", Flags: net.FlagBroadcast, Prefixes: []netip.Prefix{netip.MustParsePrefix("172.16.4.2/24")}},
		{Name: "en9", Flags: net.FlagUp | net.FlagBroadcast, Prefixes: []netip.Prefix{netip.MustParsePrefix("8.8.8.8/24")}},
	})
	want := []string{"100.64.12.0/24", "192.168.50.0/24", "fd12:3456:789a::/64"}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("active LAN prefixes = %#v, want %#v", result, want)
	}
}

func TestActiveLANPrefixIsRemovedFromBroadExclusion(t *testing.T) {
	excluded, err := excludeActiveLANPrefixes(
		[]string{"10.0.0.0/8", "192.168.0.0/16", "fc00::/7", "fe80::/10"},
		[]string{"192.168.50.0/24", "fd12:3456:789a::/64"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, active := range []string{"192.168.50.0/24", "fd12:3456:789a::/64"} {
		prefix := netip.MustParsePrefix(active)
		for _, raw := range excluded {
			if netip.MustParsePrefix(raw).Contains(prefix.Addr()) {
				t.Fatalf("active prefix %s is still covered by exclusion %s", active, raw)
			}
		}
	}
	if !contains(excluded, "10.0.0.0/8") || !contains(excluded, "fe80::/10") {
		t.Fatalf("unrelated private and link-local exclusions were lost: %#v", excluded)
	}
}

func TestNormalizeLANPrefixesRejectsPublicAndHostRoutes(t *testing.T) {
	if _, err := normalizeLANPrefixes([]string{"8.8.8.0/24"}); err == nil {
		t.Fatal("public prefix was accepted as an active LAN")
	}
	result := activeLANPrefixes([]networkSnapshot{{
		Name: "en0", Flags: net.FlagUp | net.FlagBroadcast,
		Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.1.4/32")},
	}})
	if len(result) != 0 {
		t.Fatalf("host-only address was treated as a LAN subnet: %#v", result)
	}
}
