// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import (
	"fmt"
	"strings"
)

// RenderFirewall captures traditional DNS from the host and from forwarded
// namespaces such as Docker and VMs. The main TUN auto_redirect path has no
// interface allow-list, so public forwarded traffic follows the same rules as
// host traffic. Resolver configuration files are deliberately left untouched.
func RenderFirewall(plan Plan) string {
	lines := []string{
		"table inet steer {",
		"\tchain dns_prerouting {",
		"\t\ttype nat hook prerouting priority dstnat; policy accept;",
		fmt.Sprintf("\t\tiifname \"%s\" return", plan.Resources.TunInterface),
		fmt.Sprintf("\t\tmeta mark 0x%x counter return", plan.Resources.AutoRedirectOutputMark),
		fmt.Sprintf("\t\tmeta l4proto { tcp, udp } th dport 53 counter redirect to :%d", plan.Resources.DNSPort),
		"\t}",
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
