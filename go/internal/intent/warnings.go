// SPDX-License-Identifier: GPL-3.0-or-later
package intent

import "sort"

// ReachableWarnings removes runtime warnings for entities which cannot be
// reached by the candidate's traffic graph. Structural errors remain owned by
// ValidateWithOptions and are intentionally not processed here.
func ReachableWarnings(value Intent, warnings []Issue) []Issue {
	reachable := reachableRuntimeObjects(value)
	result := make([]Issue, 0, len(warnings))
	for _, warning := range warnings {
		if runtimeWarningObjectType(warning.ObjectType) && !reachable[warning.ObjectType+"\x00"+warning.ObjectID] {
			continue
		}
		result = append(result, warning)
	}
	return result
}

// GroupWarnings builds a stable, bounded overview contract. Count represents
// affected entities rather than the number of duplicate Issue records.
func GroupWarnings(warnings []Issue) []WarningGroup {
	type accumulator struct {
		group         WarningGroup
		objects       map[string]struct{}
		withoutObject int
	}
	groups := map[string]*accumulator{}
	for _, warning := range warnings {
		key := warning.Code + "\x00" + warning.ObjectType + "\x00" + warning.Option
		item := groups[key]
		if item == nil {
			summary, destination := warningGroupPresentation(warning)
			item = &accumulator{
				group: WarningGroup{
					Code: warning.Code, ObjectType: warning.ObjectType, Option: warning.Option,
					Summary: summary, Destination: destination,
				},
				objects: map[string]struct{}{},
			}
			groups[key] = item
		}
		if warning.ObjectID == "" {
			item.withoutObject++
		} else {
			item.objects[warning.ObjectID] = struct{}{}
		}
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]WarningGroup, 0, len(keys))
	for _, key := range keys {
		item := groups[key]
		item.group.Count = len(item.objects) + item.withoutObject
		result = append(result, item.group)
	}
	return result
}

func reachableRuntimeObjects(value Intent) map[string]bool {
	result := map[string]bool{}
	if !value.Main.Enabled {
		return result
	}
	mark := func(objectType, id string) {
		if id != "" {
			result[objectType+"\x00"+id] = true
		}
	}
	mark("steer", value.Main.ID)
	mark("bootstrap", value.Bootstrap.ID)

	routes := make(map[string]Route, len(value.Routes))
	nodes := make(map[string]Node, len(value.Nodes))
	for _, route := range value.Routes {
		routes[route.ID] = route
	}
	for _, node := range value.Nodes {
		nodes[node.ID] = node
	}
	seenRoutes := map[string]bool{}
	var visitRoute func(string)
	visitRoute = func(id string) {
		if seenRoutes[id] {
			return
		}
		seenRoutes[id] = true
		route, exists := routes[id]
		if !exists || !route.Enabled {
			return
		}
		mark("route", route.ID)
		if route.Kind != "single" {
			return
		}
		if node, exists := nodes[route.Node]; exists && node.Enabled {
			mark("node", node.ID)
		}
		if route.Detour != "" {
			visitRoute(route.Detour)
		}
	}

	for _, rule := range value.Rules {
		if !rule.Enabled {
			continue
		}
		mark("rule", rule.ID)
		visitRoute(rule.Route)
		for _, profile := range value.DNSProfiles {
			if profile.ID == rule.DNSProfile && profile.Enabled {
				mark("dns_profile", profile.ID)
				break
			}
		}
	}
	// Enabled local proxies are real runtime listeners even when no Rule limits
	// its inbound tag explicitly.
	for _, proxy := range value.LocalProxies {
		if proxy.Enabled {
			mark("local_proxy", proxy.ID)
		}
	}
	return result
}

func runtimeWarningObjectType(objectType string) bool {
	switch objectType {
	case "steer", "bootstrap", "node", "route", "dns_profile", "local_proxy", "rule":
		return true
	default:
		return false
	}
}

func warningGroupPresentation(warning Issue) (string, string) {
	destination := map[string]string{
		"node": "nodes", "route": "routes", "dns_profile": "dns", "local_proxy": "proxies",
		"rule": "rules", "subscription": "subscriptions", "steer": "general", "bootstrap": "general",
	}[warning.ObjectType]
	switch warning.Code {
	case "INSECURE_TLS":
		if warning.ObjectType == "dns_profile" {
			return "DNS certificate verification is disabled", destination
		}
		return "TLS certificate verification is disabled", destination
	case "SUBSCRIPTION_NODE_STALE":
		return "Subscription node is no longer advertised", destination
	case "DNS_REJECT_PROJECTION_SKIPPED":
		return "DNS reject conditions cannot be applied before resolution", destination
	case "DNS_PROJECTION_EMPTY":
		return "DNS continues matching later rules", destination
	default:
		return "Configuration warning", destination
	}
}
