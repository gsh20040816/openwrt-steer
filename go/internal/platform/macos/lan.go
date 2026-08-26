// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"sort"
	"strings"
)

type networkSnapshot struct {
	Name     string
	Flags    net.Flags
	Prefixes []netip.Prefix
}

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// DiscoverActiveLANPrefixes returns only active unicast subnets currently
// attached to real LAN-capable interfaces. Point-to-point utun links,
// loopback, Apple peer-to-peer interfaces, link-local addresses and the Steer
// TUN itself are deliberately excluded.
func DiscoverActiveLANPrefixes() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list macOS network interfaces: %w", err)
	}
	snapshots := make([]networkSnapshot, 0, len(interfaces))
	for _, item := range interfaces {
		addresses, err := item.Addrs()
		if err != nil {
			return nil, fmt.Errorf("list addresses for macOS interface %s: %w", item.Name, err)
		}
		snapshot := networkSnapshot{Name: item.Name, Flags: item.Flags}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil {
				snapshot.Prefixes = append(snapshot.Prefixes, prefix.Masked())
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return activeLANPrefixes(snapshots), nil
}

func CurrentLANPrefixes(runDirectory string) ([]string, error) {
	paths := runtimePaths(runDirectory, "")
	current, err := paths.LoadCurrent()
	if err != nil {
		return nil, err
	}
	content, err := readJSON(filepath.Join(paths.GenerationsDirectory, current.Directory, "platform.json"))
	if err != nil {
		return nil, err
	}
	var plan Plan
	if err := unmarshalStrict(content, &plan); err != nil {
		return nil, fmt.Errorf("decode active macOS platform plan: %w", err)
	}
	if plan.SchemaVersion != 2 {
		return nil, fmt.Errorf("unsupported active macOS platform plan schema %d", plan.SchemaVersion)
	}
	return normalizeLANPrefixes(plan.Resources.ActiveLANPrefixes)
}

func activeLANPrefixes(snapshots []networkSnapshot) []string {
	known := map[string]bool{}
	result := []string{}
	for _, item := range snapshots {
		name := strings.ToLower(item.Name)
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 || item.Flags&net.FlagPointToPoint != 0 ||
			strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "awdl") || strings.HasPrefix(name, "llw") {
			continue
		}
		if item.Flags&(net.FlagBroadcast|net.FlagMulticast) == 0 {
			continue
		}
		for _, raw := range item.Prefixes {
			prefix := raw.Masked()
			address := prefix.Addr().Unmap()
			if !address.IsValid() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() ||
				(!address.IsPrivate() && !(address.Is4() && cgnatPrefix.Contains(address))) {
				continue
			}
			bits := prefix.Bits()
			if (address.Is4() && (bits < 8 || bits >= 32)) || (address.Is6() && (bits < 16 || bits >= 128)) {
				continue
			}
			value := prefix.String()
			if !known[value] {
				known[value] = true
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}

func normalizeLANPrefixes(values []string) ([]string, error) {
	known := map[string]bool{}
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("invalid active LAN prefix %q: %w", value, err)
		}
		prefix = prefix.Masked()
		address := prefix.Addr().Unmap()
		if !address.IsPrivate() && !(address.Is4() && cgnatPrefix.Contains(address)) {
			return nil, fmt.Errorf("active LAN prefix %q is not private unicast or CGNAT", value)
		}
		if (address.Is4() && (prefix.Bits() < 8 || prefix.Bits() >= 32)) ||
			(address.Is6() && (prefix.Bits() < 16 || prefix.Bits() >= 128)) {
			return nil, fmt.Errorf("active LAN prefix %q is not a usable subnet", value)
		}
		if !known[prefix.String()] {
			known[prefix.String()] = true
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if prefixes[i].Addr().BitLen() != prefixes[j].Addr().BitLen() {
			return prefixes[i].Addr().BitLen() < prefixes[j].Addr().BitLen()
		}
		if prefixes[i].Bits() != prefixes[j].Bits() {
			return prefixes[i].Bits() < prefixes[j].Bits()
		}
		return prefixes[i].Addr().Less(prefixes[j].Addr())
	})
	result := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		result = append(result, prefix.String())
	}
	return result, nil
}

func excludeActiveLANPrefixes(excluded, active []string) ([]string, error) {
	current := make([]netip.Prefix, 0, len(excluded))
	for _, value := range excluded {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, err
		}
		current = append(current, prefix.Masked())
	}
	for _, value := range active {
		remove, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, err
		}
		next := []netip.Prefix{}
		for _, base := range current {
			next = append(next, subtractPrefix(base, remove.Masked())...)
		}
		current = next
	}
	sort.Slice(current, func(i, j int) bool {
		if current[i].Addr().BitLen() != current[j].Addr().BitLen() {
			return current[i].Addr().BitLen() < current[j].Addr().BitLen()
		}
		if current[i].Addr() != current[j].Addr() {
			return current[i].Addr().Less(current[j].Addr())
		}
		return current[i].Bits() < current[j].Bits()
	})
	result := make([]string, 0, len(current))
	for _, prefix := range current {
		result = append(result, prefix.String())
	}
	return result, nil
}

func subtractPrefix(base, remove netip.Prefix) []netip.Prefix {
	base, remove = base.Masked(), remove.Masked()
	if base.Addr().BitLen() != remove.Addr().BitLen() || !base.Overlaps(remove) {
		return []netip.Prefix{base}
	}
	if remove.Bits() <= base.Bits() {
		return nil
	}
	current := base
	result := []netip.Prefix{}
	for current.Bits() < remove.Bits() {
		left, right := splitPrefix(current)
		if left.Contains(remove.Addr()) {
			result = append(result, right)
			current = left
		} else {
			result = append(result, left)
			current = right
		}
	}
	return result
}

func splitPrefix(prefix netip.Prefix) (netip.Prefix, netip.Prefix) {
	nextBits := prefix.Bits() + 1
	left := netip.PrefixFrom(prefix.Addr(), nextBits).Masked()
	bit := prefix.Bits()
	if prefix.Addr().Is4() {
		bytes := prefix.Addr().As4()
		bytes[bit/8] |= byte(1 << (7 - bit%8))
		return left, netip.PrefixFrom(netip.AddrFrom4(bytes), nextBits).Masked()
	}
	bytes := prefix.Addr().As16()
	bytes[bit/8] |= byte(1 << (7 - bit%8))
	return left, netip.PrefixFrom(netip.AddrFrom16(bytes), nextBits).Masked()
}
