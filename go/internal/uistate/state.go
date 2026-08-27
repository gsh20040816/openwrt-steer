// SPDX-License-Identifier: GPL-3.0-or-later

// Package uistate builds the Saved lifecycle facts shared by native frontends.
// Draft ownership and Active status transport remain platform-specific.
package uistate

import (
	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

type Counts struct {
	Nodes         int `json:"nodes"`
	Subscriptions int `json:"subscriptions"`
	Routes        int `json:"routes"`
	DNSProfiles   int `json:"dns_profiles"`
	LocalProxies  int `json:"local_proxies"`
	Rules         int `json:"rules"`
}

type IntentState struct {
	Available     bool             `json:"available"`
	Enabled       bool             `json:"enabled"`
	Digest        string           `json:"digest,omitempty"`
	RuntimeDigest string           `json:"runtime_digest,omitempty"`
	Counts        Counts           `json:"counts"`
	Validation    model.Validation `json:"validation"`
}

func FromIntent(value model.Intent, validation model.Validation, options compiler.Options) IntentState {
	compiled := compiler.Compile(value, options)
	return IntentState{
		Available: true, Enabled: value.Main.Enabled,
		Digest: compiled.IntentDigest, RuntimeDigest: compiled.RuntimeDigest,
		Counts: Counts{
			Nodes: len(value.Nodes), Subscriptions: len(value.Subscriptions), Routes: len(value.Routes),
			DNSProfiles: len(value.DNSProfiles), LocalProxies: len(value.LocalProxies), Rules: len(value.Rules),
		},
		Validation: validation,
	}
}

func PendingApply(saved IntentState, activeGeneration, activeRuntimeDigest string, lastApply *coreapply.Record) bool {
	if !saved.Available || !saved.Validation.OK {
		return false
	}
	if saved.Enabled {
		if activeGeneration == "" || activeRuntimeDigest != saved.RuntimeDigest {
			return true
		}
	} else if activeGeneration != "" {
		return true
	}
	return lastApply != nil && !lastApply.Result.OK && lastApply.Result.RuntimeDigest != "" &&
		lastApply.Result.RuntimeDigest == saved.RuntimeDigest
}
