// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"github.com/gsh20040816/steer/go/internal/geodata"
	model "github.com/gsh20040816/steer/go/internal/intent"
	platformvalidation "github.com/gsh20040816/steer/go/internal/validation"
)

func Validate(value model.Intent) model.Validation {
	return ValidateWithGeoDataDirectory(value, geodata.DefaultSeedDirectory)
}

func ValidateWithGeoDataDirectory(value model.Intent, seedDirectory string) model.Validation {
	return platformvalidation.Validate(value, platformvalidation.Options{
		ReservedListeners: []model.Listener{
			platformvalidation.ReservedListener("::", DNSPort, "OpenWrt DNS"),
		},
		IPv6WildcardDualStack: true,
		GeoDataDirectory:      seedDirectory,
	})
}
