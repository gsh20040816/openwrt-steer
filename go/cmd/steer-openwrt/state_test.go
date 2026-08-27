// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/uistate"
)

func TestUIPendingApplyUsesRuntimeProjectionAndFailedApplyFact(t *testing.T) {
	saved := uistate.IntentState{
		Available: true, Enabled: true, Digest: "canonical-with-inventory-a", RuntimeDigest: "runtime-a",
		Validation: model.Validation{OK: true},
	}
	if uistate.PendingApply(saved, "generation-a", "runtime-a", nil) {
		t.Fatal("subscription-only canonical inventory drift manufactured pending Apply")
	}
	if !uistate.PendingApply(saved, "generation-a", "runtime-old", nil) {
		t.Fatal("runtime projection drift did not create pending Apply")
	}
	lastApply := &coreapply.Record{Sequence: "1", Result: coreapply.Result{OK: false, RuntimeDigest: "runtime-a"}}
	if !uistate.PendingApply(saved, "generation-a", "runtime-a", lastApply) {
		t.Fatal("failed Apply for the current Saved runtime projection was hidden")
	}
}
