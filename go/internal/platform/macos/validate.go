// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	model "github.com/gsh20040816/steer/go/internal/intent"
)

// Validate combines the canonical contract with macOS platform limits.
// Source-MAC policy describes gateway traffic and has no reliable meaning for
// a local Darwin TUN, so it is rejected explicitly.
func Validate(value model.Intent) model.Validation {
	validation := model.Validate(value)
	for _, rule := range value.Rules {
		if !rule.Enabled {
			continue
		}
		if len(rule.SourceMACAddress) > 0 {
			validation.Errors = append(validation.Errors, model.Issue{
				Code: "PLATFORM_UNSUPPORTED_SOURCE_MAC", ObjectType: "rule", ObjectID: rule.ID,
				Option: "source_mac_address", Message: "macOS does not support source MAC rules",
			})
		}
	}
	validation.OK = len(validation.Errors) == 0
	return validation
}
