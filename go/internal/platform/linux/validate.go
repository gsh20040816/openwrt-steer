// SPDX-License-Identifier: GPL-3.0-or-later

package linux

import model "github.com/gsh20040816/openwrt-steer/go/internal/intent"

// Validate combines the canonical contract with Linux workstation limits.
// Source-MAC policy is an OpenWrt gateway feature and is rejected explicitly.
func Validate(value model.Intent) model.Validation {
	validation := model.Validate(value)
	for _, rule := range value.Rules {
		if !rule.Enabled || len(rule.SourceMACAddress) == 0 {
			continue
		}
		validation.Errors = append(validation.Errors, model.Issue{
			Code: "PLATFORM_UNSUPPORTED_SOURCE_MAC", ObjectType: "rule", ObjectID: rule.ID,
			Option: "source_mac_address", Message: "Linux workstation mode does not support source MAC rules",
		})
	}
	validation.OK = len(validation.Errors) == 0
	return validation
}
