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
	"time"

	"github.com/dustin/go-humanize"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

// Timezone for displaying absolute times.
var hardcodedTZ = time.FixedZone("UTC-07", -7*60*60)

// assetState is an overall asset state to display in the UI.
type assetState string

const (
	stateUnknown  assetState = "UNKNOWN"    // should be unreachable
	stateUpToDate assetState = "UP-TO-DATE" // matches intent
	stateLocked   assetState = "LOCKED"     // has outstanding locks
	stateDisabled assetState = "DISABLED"   // disabled in the config
	stateUpdating assetState = "UPDATING"   // has an active actuation right now
	stateBroken   assetState = "BROKEN"     // broken configuration
	stateFailed   assetState = "FAILED"     // the last actuation failed
)

// deriveState derives an overall state based on the asset proto.
func deriveState(a *modelpb.Asset) assetState {
	switch a.LastDecision.GetDecision() {
	case modelpb.ActuationDecision_SKIP_UPTODATE:
		return stateUpToDate

	case modelpb.ActuationDecision_SKIP_DISABLED:
		return stateDisabled

	case modelpb.ActuationDecision_SKIP_LOCKED:
		return stateLocked

	case modelpb.ActuationDecision_SKIP_BROKEN:
		return stateBroken

	case modelpb.ActuationDecision_ACTUATE_FORCE, modelpb.ActuationDecision_ACTUATE_STALE:
		switch a.LastActuation.GetState() {
		case modelpb.Actuation_EXECUTING:
			return stateUpdating
		case modelpb.Actuation_SUCCEEDED:
			return stateUpToDate
		case modelpb.Actuation_FAILED, modelpb.Actuation_EXPIRED:
			return stateFailed
		default:
			return stateUnknown // this should not be possible
		}

	default:
		return stateUnknown // this should not be possible
	}
}

// tableClass is the corresponding Bootstrap CSS table class.
func (s assetState) tableClass() string {
	switch s {
	case stateUpToDate:
		return "table-light"
	case stateLocked:
		return "table-warning"
	case stateDisabled:
		return "table-secondary"
	case stateUpdating:
		return "table-info"
	case stateUnknown, stateBroken, stateFailed:
		return "table-danger"
	default:
		panic("impossible")
	}
}

// tableClass is the corresponding Bootstrap CSS badge class.
func (s assetState) badgeClass() string {
	switch s {
	case stateUpToDate:
		return "bg-success"
	case stateLocked:
		return "bg-warning text-dark"
	case stateDisabled:
		return "bg-secondary"
	case stateUpdating:
		return "bg-info text-dark"
	case stateUnknown, stateBroken, stateFailed:
		return "bg-danger"
	default:
		panic("impossible")
	}
}

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

// linkHref is a potentially clickable link.
type linkHref struct {
	Text    string
	Href    string
	Tooltip string
}

// timestampHref a link that looks like a timestamp.
func timestampHref(ts *timestamppb.Timestamp, href string) linkHref {
	if ts == nil {
		return linkHref{Href: href}
	}
	t := ts.AsTime()
	return linkHref{
		Text:    humanize.RelTime(t, time.Now(), "ago", "from now"),
		Href:    href,
		Tooltip: t.In(hardcodedTZ).Format(time.RFC822),
	}
}

// buildbucketHref returns a link to the build running the actuation.
func buildbucketHref(act *modelpb.ActuatorInfo) string {
	if act.BuildbucketBuild == 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/build/%d", act.Buildbucket, act.BuildbucketBuild)
}

// commitHref returns a link to the commit matching the deployment.
func commitHref(dep *modelpb.Deployment) linkHref {
	if dep.GetId().GetRepoHost() == "" || dep.GetConfigRev() == "" {
		return linkHref{}
	}
	return linkHref{
		Text: dep.ConfigRev[:8],
		Href: fmt.Sprintf("https://%s.googlesource.com/%s/+/%s",
			dep.Id.RepoHost, dep.Id.RepoName, dep.ConfigRev),
	}
}

// assetOverview is a subset of asset data passed to the index HTML template.
type assetOverview struct {
	Ref assetRef // link to the asset page

	State      assetState // overall state
	TableClass string     // CSS class for the table row
	BadgeClass string     // CSS class for the state cell

	LastCheckIn   linkHref // when its state was reported last time
	LastActuation linkHref // when the last non-trivial actuation happened
	Revision      linkHref // last applied IaC revision
}

// deriveAssetOverview derives assetOverview from the asset proto.
func deriveAssetOverview(asset *modelpb.Asset) assetOverview {
	out := assetOverview{
		Ref:   assetRefFromID(asset.Id),
		State: deriveState(asset),
	}

	out.TableClass = out.State.tableClass()
	out.BadgeClass = out.State.badgeClass()

	// The last "check in" time always matches the last performed actuation
	// regardless of its outcome. Most of the time this outcome is SKIP_UPTODATE.
	if asset.LastActuation != nil {
		out.LastCheckIn = timestampHref(
			asset.LastActuation.Created,
			buildbucketHref(asset.LastActuation.Actuator),
		)
	}

	// Show the last non-trivial actuation (may still be executing).
	if asset.LastActuateActuation != nil {
		if asset.LastActuateActuation.Finished != nil {
			out.LastActuation = timestampHref(
				asset.LastActuateActuation.Finished,
				asset.LastActuateActuation.LogUrl,
			)
		} else {
			out.LastActuation = linkHref{
				Text: "now",
				Href: asset.LastActuateActuation.LogUrl,
			}
		}
	}

	// If have an actuation executing right now, show its IaC revision (as the
	// one being applied now), otherwise show the last successfully applied
	// revision.
	if asset.LastActuation.GetState() == modelpb.Actuation_EXECUTING {
		out.Revision = commitHref(asset.LastActuation.Deployment)
	} else if asset.AppliedState != nil {
		out.Revision = commitHref(asset.AppliedState.Deployment)
	}

	return out
}

////////////////////////////////////////////////////////////////////////////////

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
