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
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/deploy/api/rpcpb"
)

// assetRef is a reference to an asset in the UI.
type assetRef struct {
	Kind string // kind of the asset (as human readable string)
	Icon string // icon image file name
	Name string // display name of the asset e.g. GAE app ID
	Href string // link to the asset page
}

// assetRefFromID constructs assetRef based on the asset ID.
func assetRefFromID(assetID string) assetRef {
	href := fmt.Sprintf("/a/%s", assetID)
	switch {
	case strings.HasPrefix(assetID, "apps/"):
		return assetRef{
			Kind: "Appengine service",
			Icon: "app_engine.svg",
			Name: strings.TrimPrefix(assetID, "apps/"),
			Href: href,
		}
	default:
		return assetRef{
			Kind: "Unknown",
			Icon: "unknown.svg",
			Name: assetID,
			Href: href,
		}
	}
}

// assetPage renders the asset page.
func (ui *UI) assetPage(ctx *router.Context) error {
	asset, err := ui.assets.GetAsset(ctx.Context, &rpcpb.GetAssetRequest{
		AssetId: strings.TrimPrefix(ctx.Params.ByName("AssetID"), "/"),
	})
	if err != nil {
		return err
	}

	// TODO: this is temporary.
	dump, err := (prototext.MarshalOptions{Indent: "  "}).Marshal(asset)
	if err != nil {
		return err
	}

	ref := assetRefFromID(asset.Id)

	templates.MustRender(ctx.Context, ctx.Writer, "pages/asset.html", map[string]interface{}{
		"Breadcrumbs": assetBreadcrumbs(ref),
		"Ref":         ref,
		"Dump":        string(dump),
	})
	return nil
}
