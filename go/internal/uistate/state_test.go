// SPDX-License-Identifier: GPL-3.0-or-later

package uistate

import (
	"testing"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	"github.com/gsh20040816/steer/go/internal/compiler"
	model "github.com/gsh20040816/steer/go/internal/intent"
)

func TestPendingApplyUsesRuntimeProjectionAndFailedApplyFact(t *testing.T) {
	saved := IntentState{
		Available: true, Enabled: true, Digest: "canonical-with-inventory-a", RuntimeDigest: "runtime-a",
		Validation: model.Validation{OK: true},
	}
	if PendingApply(saved, "generation-a", "runtime-a", nil) {
		t.Fatal("subscription-only canonical inventory drift manufactured pending Apply")
	}
	if !PendingApply(saved, "generation-a", "runtime-old", nil) {
		t.Fatal("runtime projection drift did not create pending Apply")
	}
	failed := &coreapply.Record{Sequence: "1", Result: coreapply.Result{OK: false, RuntimeDigest: "runtime-a"}}
	if !PendingApply(saved, "generation-a", "runtime-a", failed) {
		t.Fatal("failed Apply for the current Saved runtime projection was hidden")
	}
	saved.Enabled = false
	if PendingApply(saved, "", "", nil) || !PendingApply(saved, "still-active", "runtime-a", nil) {
		t.Fatal("disabled Saved lifecycle did not follow Active generation truth")
	}
	saved.Validation.OK = false
	if PendingApply(saved, "still-active", "runtime-a", failed) {
		t.Fatal("invalid Saved configuration advertised an Apply action")
	}
}

func TestFromIntentCountsEveryOverviewCollection(t *testing.T) {
	value := model.Intent{
		Nodes: []model.Node{{ID: "a"}, {ID: "b"}}, Routes: []model.Route{{ID: "direct"}},
		DNSProfiles: []model.DNSProfile{{ID: "dns"}}, LocalProxies: []model.LocalProxy{{ID: "proxy"}},
		Rules: []model.Rule{{ID: "default"}}, Subscriptions: []model.Subscription{{ID: "feed"}},
	}
	state := FromIntent(value, model.Validation{OK: true}, compiler.Options{})
	if state.Counts != (Counts{Nodes: 2, Routes: 1, DNSProfiles: 1, LocalProxies: 1, Rules: 1, Subscriptions: 1}) {
		t.Fatalf("unexpected Overview counts: %#v", state.Counts)
	}
}
