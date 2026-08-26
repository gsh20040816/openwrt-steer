// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/platform/openwrt"
)

func TestUIPendingApplyUsesRuntimeProjectionAndFailedApplyFact(t *testing.T) {
	saved := uiIntentState{
		Available: true, Enabled: true, Digest: "canonical-with-inventory-a", RuntimeDigest: "runtime-a",
		Validation: model.Validation{OK: true},
	}
	active := openwrt.Status{Generation: "generation-a", IntentDigest: "canonical-with-inventory-b", RuntimeDigest: "runtime-a"}
	if uiPendingApply(saved, active) {
		t.Fatal("subscription-only canonical inventory drift manufactured pending Apply")
	}
	active.RuntimeDigest = "runtime-old"
	if !uiPendingApply(saved, active) {
		t.Fatal("runtime projection drift did not create pending Apply")
	}
	active.RuntimeDigest = "runtime-a"
	active.LastApply = &coreapply.Record{Sequence: "1", Result: coreapply.Result{OK: false, RuntimeDigest: "runtime-a"}}
	if !uiPendingApply(saved, active) {
		t.Fatal("failed Apply for the current Saved runtime projection was hidden")
	}
}
