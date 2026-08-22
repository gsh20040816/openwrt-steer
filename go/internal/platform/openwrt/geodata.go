// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"

	"github.com/gsh20040816/openwrt-steer/go/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/go/internal/geodata"
)

// GeoOptions is kept as an OpenWrt-facing alias while Geo generation itself
// belongs to the cross-platform geodata package.
type GeoOptions = geodata.Options

func GeoCatalog(ctx context.Context, runner Runner, kind, seedDirectory, geoViewBinary string) ([]string, error) {
	return geodata.Catalog(ctx, runner, kind, seedDirectory, geoViewBinary)
}

func EnsureGeoRules(ctx context.Context, runner Runner, ruleSets []compiler.GeoRuleSet, options GeoOptions) error {
	return geodata.EnsureRules(ctx, runner, ruleSets, options)
}
