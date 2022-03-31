// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ui

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/deploy/api/rpcpb"
)

// assetOverview is a subset of asset data passed to the index HTML template.
type assetOverview struct {
	Ref assetRef
	// TODO: add more
}

// indexPage renders the index page.
func (ui *UI) indexPage(ctx *router.Context) error {
	listing, err := ui.assets.ListAssets(ctx.Context, &rpcpb.ListAssetsRequest{})
	if err != nil {
		return err
	}

	assets := make([]assetOverview, len(listing.Assets))
	for i, asset := range listing.Assets {
		assets[i] = assetOverview{
			Ref: assetRefFromID(asset.Id),
			// TODO: add more
		}
	}

	templates.MustRender(ctx.Context, ctx.Writer, "pages/index.html", map[string]interface{}{
		"Breadcrumbs": rootBreadcrumbs(),
		"Assets":      assets,
	})
	return nil
}
