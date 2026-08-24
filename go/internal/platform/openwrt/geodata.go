// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"github.com/gsh20040816/steer/go/internal/compiler"
	"github.com/gsh20040816/steer/go/internal/geodata"
)

func GeoCatalog(seedDirectory, kind string) ([]string, error) {
	return geodata.Catalog(seedDirectory, kind)
}

func ValidateGeoRules(ruleSets []compiler.GeoRuleSet, seedDirectory string) error {
	return geodata.ValidateRequiredRules(ruleSets, seedDirectory)
}
