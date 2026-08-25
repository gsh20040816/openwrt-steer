// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"fmt"
	"strings"
)

var nftNonGlobalIPv4 = []string{
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
	"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
	"198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
}

var nftGlobalExceptionsIPv4 = []string{"192.0.0.9/32", "192.0.0.10/32"}

var nftNonGlobalIPv6 = []string{
	"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b:1::/48", "100::/64", "100:0:0:1::/64",
	"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20", "5f00::/16", "fc00::/7",
	"fe80::/10", "ff00::/8",
}

var nftGlobalExceptionsIPv6 = []string{
	"2001:1::1/128", "2001:1::2/128", "2001:1::3/128", "2001:3::/32",
	"2001:4:112::/48", "2001:20::/28", "2001:30::/28",
}

func RenderFirewall(plan Plan) (string, error) {
	var lines []string
	add := func(values ...string) { lines = append(lines, values...) }
	add("table inet steer {",
		renderSet("non_global_ipv4", "ipv4_addr", nftNonGlobalIPv4),
		renderSet("global_exceptions_ipv4", "ipv4_addr", nftGlobalExceptionsIPv4),
		renderSet("non_global_ipv6", "ipv6_addr", nftNonGlobalIPv6),
		renderSet("global_exceptions_ipv6", "ipv6_addr", nftGlobalExceptionsIPv6),
		"\tchain dns_prerouting {", "\t\ttype nat hook prerouting priority dstnat - 2; policy accept;")
	add("\t\tiifname \"steer0\" return",
		fmt.Sprintf("\t\tmeta mark 0x%x return", plan.Resources.AutoRedirectOutputMark),
	)
	add(
		fmt.Sprintf("\t\tmeta l4proto { tcp, udp } th dport 53 counter redirect to :%d", plan.Resources.DNSPort), "\t}",
		"\tchain dns_output {", "\t\ttype nat hook output priority mangle - 2; policy accept;",
		"\t\toifname \"steer0\" return",
		fmt.Sprintf("\t\tmeta mark 0x%x counter return", plan.Resources.AutoRedirectOutputMark),
		fmt.Sprintf("\t\tmeta nfproto ipv4 meta l4proto { tcp, udp } th dport 53 counter dnat ip to 127.0.0.1:%d", plan.Resources.DNSPort),
		fmt.Sprintf("\t\tmeta nfproto ipv6 meta l4proto { tcp, udp } th dport 53 counter dnat ip6 to [::1]:%d", plan.Resources.DNSPort), "\t}",
		"\tchain dns_postrouting {", "\t\ttype nat hook postrouting priority srcnat - 2; policy accept;",
		fmt.Sprintf("\t\tmeta nfproto ipv4 meta l4proto { tcp, udp } ip daddr 127.0.0.1 th dport %d counter snat ip to 127.0.0.1", plan.Resources.DNSPort),
		fmt.Sprintf("\t\tmeta nfproto ipv6 meta l4proto { tcp, udp } ip6 daddr ::1 th dport %d counter snat ip6 to ::1", plan.Resources.DNSPort), "\t}",
		"\tchain system_output {", "\t\ttype route hook output priority mangle - 2; policy accept;",
		fmt.Sprintf("\t\tmeta l4proto udp udp dport 123 counter meta mark set 0x%x", plan.Resources.AutoRedirectOutputMark), "\t}")
	add("}", "")
	return strings.Join(lines, "\n"), nil
}

func renderSet(name, kind string, values []string) string {
	return fmt.Sprintf("\tset %s {\n\t\ttype %s\n\t\tflags interval\n\t\tauto-merge\n\t\telements = { %s }\n\t}", name, kind, strings.Join(values, ", "))
}
