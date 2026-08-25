// SPDX-License-Identifier: GPL-3.0-or-later

package intent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MigrateJSON8 converts the only supported legacy canonical JSON schema to
// schema 9. The removed per-profile strategy cannot be represented by
// sing-box 1.14 without a deprecated DNS rule action. Client A/AAAA queries
// stay transparent; bootstrap strategy remains the internal resolver policy.
func MigrateJSON8(content []byte) (Intent, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var legacy schema8Intent
	if err := decoder.Decode(&legacy); err != nil {
		return Intent{}, fmt.Errorf("decode schema 8 canonical intent JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing value")
		}
		return Intent{}, fmt.Errorf("decode schema 8 canonical intent JSON: %w", err)
	}
	if legacy.Main.SchemaVersion != 8 {
		return Intent{}, fmt.Errorf("canonical intent schema is %d, want 8", legacy.Main.SchemaVersion)
	}
	profiles := make([]DNSProfile, 0, len(legacy.DNSProfiles))
	for _, profile := range legacy.DNSProfiles {
		profiles = append(profiles, DNSProfile{
			ID: profile.ID, Enabled: profile.Enabled, Name: profile.Name,
			Protocol: profile.Protocol, Server: profile.Server, ServerPort: profile.ServerPort,
			TLSServerName: profile.TLSServerName, Path: profile.Path, Insecure: profile.Insecure,
		})
	}
	legacy.Main.SchemaVersion = SchemaVersion
	return Intent{
		Main: legacy.Main, Bootstrap: legacy.Bootstrap, Nodes: legacy.Nodes,
		Subscriptions: legacy.Subscriptions, Routes: legacy.Routes, DNSProfiles: profiles,
		LocalProxies: legacy.LocalProxies, Rules: legacy.Rules,
	}, nil
}

type schema8Intent struct {
	Main          Main                `json:"main"`
	Bootstrap     Bootstrap           `json:"bootstrap"`
	Nodes         []Node              `json:"nodes"`
	Subscriptions []Subscription      `json:"subscriptions"`
	Routes        []Route             `json:"routes"`
	DNSProfiles   []schema8DNSProfile `json:"dns_profiles"`
	LocalProxies  []LocalProxy        `json:"local_proxies"`
	Rules         []Rule              `json:"rules"`
}

type schema8DNSProfile struct {
	ID            string `json:"id"`
	Enabled       bool   `json:"enabled"`
	Name          string `json:"name,omitempty"`
	Protocol      string `json:"protocol"`
	Server        string `json:"server"`
	ServerPort    int    `json:"server_port"`
	TLSServerName string `json:"tls_server_name,omitempty"`
	Path          string `json:"path,omitempty"`
	Insecure      bool   `json:"insecure,omitempty"`
	Strategy      string `json:"strategy"`
}
