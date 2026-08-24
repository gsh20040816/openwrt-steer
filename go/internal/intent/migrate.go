// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MigrateJSON7 converts the only supported legacy canonical JSON schema to
// schema 8. The removed per-profile cache flags never had runtime semantics;
// all other fields are preserved exactly through typed decoding.
func MigrateJSON7(content []byte) (Intent, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var legacy schema7Intent
	if err := decoder.Decode(&legacy); err != nil {
		return Intent{}, fmt.Errorf("decode schema 7 canonical intent JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing value")
		}
		return Intent{}, fmt.Errorf("decode schema 7 canonical intent JSON: %w", err)
	}
	if legacy.Main.SchemaVersion != 7 {
		return Intent{}, fmt.Errorf("canonical intent schema is %d, want 7", legacy.Main.SchemaVersion)
	}
	profiles := make([]DNSProfile, 0, len(legacy.DNSProfiles))
	for _, profile := range legacy.DNSProfiles {
		profiles = append(profiles, DNSProfile{
			ID: profile.ID, Enabled: profile.Enabled, Name: profile.Name,
			Protocol: profile.Protocol, Server: profile.Server, ServerPort: profile.ServerPort,
			TLSServerName: profile.TLSServerName, Path: profile.Path, Insecure: profile.Insecure,
			Strategy: profile.Strategy,
		})
	}
	legacy.Main.SchemaVersion = SchemaVersion
	return Intent{
		Main: legacy.Main, Bootstrap: legacy.Bootstrap, Nodes: legacy.Nodes,
		Subscriptions: legacy.Subscriptions, Routes: legacy.Routes, DNSProfiles: profiles,
		LocalProxies: legacy.LocalProxies, Rules: legacy.Rules,
	}, nil
}

type schema7Intent struct {
	Main          Main                `json:"main"`
	Bootstrap     Bootstrap           `json:"bootstrap"`
	Nodes         []Node              `json:"nodes"`
	Subscriptions []Subscription      `json:"subscriptions"`
	Routes        []Route             `json:"routes"`
	DNSProfiles   []schema7DNSProfile `json:"dns_profiles"`
	LocalProxies  []LocalProxy        `json:"local_proxies"`
	Rules         []Rule              `json:"rules"`
}

type schema7DNSProfile struct {
	ID              string `json:"id"`
	Enabled         bool   `json:"enabled"`
	Name            string `json:"name,omitempty"`
	Protocol        string `json:"protocol"`
	Server          string `json:"server"`
	ServerPort      int    `json:"server_port"`
	TLSServerName   string `json:"tls_server_name,omitempty"`
	Path            string `json:"path,omitempty"`
	Insecure        bool   `json:"insecure,omitempty"`
	Strategy        string `json:"strategy"`
	CachePersist    bool   `json:"cache_persist,omitempty"`
	OptimisticCache bool   `json:"optimistic_cache,omitempty"`
}
