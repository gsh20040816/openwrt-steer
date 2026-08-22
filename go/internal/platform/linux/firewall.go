// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"fmt"
	"strings"
)

// RenderFirewall only captures DNS emitted by the local host.  It does not
// alter forwarding or resolver configuration and deliberately leaves local
// destinations alone.
func RenderFirewall(plan Plan) string {
	lines := []string{
		"table inet steer {",
		"\tchain dns_output {",
		"\t\ttype nat hook output priority mangle - 2; policy accept;",
		fmt.Sprintf("\t\tmeta mark 0x%x counter return", plan.Resources.AutoRedirectOutputMark),
		fmt.Sprintf("\t\toifname \"%s\" return", plan.Resources.TunInterface),
		"\t\tfib daddr type { local, broadcast, anycast, multicast } return",
		fmt.Sprintf("\t\tmeta nfproto ipv4 meta l4proto { tcp, udp } th dport 53 counter dnat ip to 127.0.0.1:%d", plan.Resources.DNSPort),
		fmt.Sprintf("\t\tmeta nfproto ipv6 meta l4proto { tcp, udp } th dport 53 counter dnat ip6 to [::1]:%d", plan.Resources.DNSPort),
		"\t}",
		"\tchain dns_postrouting {",
		"\t\ttype nat hook postrouting priority srcnat - 2; policy accept;",
		fmt.Sprintf("\t\tmeta nfproto ipv4 meta l4proto { tcp, udp } ip daddr 127.0.0.1 th dport %d counter snat ip to 127.0.0.1", plan.Resources.DNSPort),
		fmt.Sprintf("\t\tmeta nfproto ipv6 meta l4proto { tcp, udp } ip6 daddr ::1 th dport %d counter snat ip6 to ::1", plan.Resources.DNSPort),
		"\t}",
		"}",
		"",
	}
	return strings.Join(lines, "\n")
}
