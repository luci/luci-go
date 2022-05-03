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
	"html"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

var (
	// Timezone for displaying absolute times.
	hardcodedTZ = time.FixedZone("UTC-07", -7*60*60)
	// For sorting, to make sure "missing" timestamps show up first.
	distantFuture = time.Date(2112, 1, 1, 1, 1, 1, 1, time.UTC)
)

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

// badgeClass is the corresponding Bootstrap CSS badge class.
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
	Target  string
}

// hrefTarget derives "target" attribute for an href.
func hrefTarget(href string) string {
	if !strings.HasPrefix(href, "/") {
		return "_blank"
	}
	return ""
}

// timestampHref a link that looks like a timestamp.
func timestampHref(ts *timestamppb.Timestamp, href, tooltipSfx string) linkHref {
	if ts == nil {
		return linkHref{
			Href:   href,
			Target: hrefTarget(href),
		}
	}
	t := ts.AsTime()

	tooltip := t.In(hardcodedTZ).Format(time.RFC822)
	if tooltipSfx != "" {
		tooltip += " " + tooltipSfx
	}

	return linkHref{
		Text:    humanize.RelTime(t, time.Now(), "ago", "from now"),
		Href:    href,
		Tooltip: tooltip,
		Target:  hrefTarget(href),
	}
}

// buildbucketHref returns a link to the build running the actuation.
func buildbucketHref(act *modelpb.ActuatorInfo) string {
	if act.BuildbucketBuild == 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/build/%d", act.Buildbucket, act.BuildbucketBuild)
}

// appengineServiceHref returns a link to a GAE service Cloud Console page.
func appengineServiceHref(project, service string) linkHref {
	return linkHref{
		Text: service,
		Href: "https://console.cloud.google.com/appengine/versions?" + (url.Values{
			"project":   {project},
			"serviceId": {service},
		}).Encode(),
		Target: "_blank",
	}
}

// appengineVersionHref returns a link to a GAE version Cloud Console logs page.
func appengineVersionHref(project, service, version string) linkHref {
	// That's how "TOOLS => Logs" link looks like on GAE Versions list page.
	return linkHref{
		Text: version,
		Href: "https://console.cloud.google.com/logs?" + (url.Values{
			"project":   {project},
			"serviceId": {service},
			"service":   {"appengine.googleapis.com"},
			"key1":      {service},
			"key2":      {version},
		}).Encode(),
		Target: "_blank",
	}
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
		Target: "_blank",
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
			"",
		)
	}

	// Show the last non-trivial actuation (may still be executing).
	if asset.LastActuateActuation != nil {
		if asset.LastActuateActuation.Finished != nil {
			out.LastActuation = timestampHref(
				asset.LastActuateActuation.Finished,
				asset.LastActuateActuation.LogUrl,
				"",
			)
		} else {
			out.LastActuation = linkHref{
				Text:   "now",
				Href:   asset.LastActuateActuation.LogUrl,
				Target: "_blank",
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

type versionState struct {
	Service         linkHref // name of the service e.g. "default"
	Version         linkHref // name of the version e.g. "1234-abcedf"
	Deployed        linkHref // when it was deployed
	TrafficIntended int32    // intended percent of traffic
	TrafficReported int32    // reported percent of traffic

	RowSpan int // helper to merge HTML table cells

	sortKey1 string // sorting helper, derived from service name
	sortKey2 int64  // sorting helper, derived from deployment timestamp
}

func versionsSummary(asset *modelpb.Asset, active bool) []versionState {
	type versionID struct {
		svc string
		ver string
	}
	versions := map[versionID]versionState{}

	serviceSortKey := func(svc string) string {
		// We want to show services like "default" and "default-go" on top, they
		// are known to be the most important.
		if strings.HasPrefix(svc, "default") {
			return "a " + svc
		}
		return "b " + svc
	}

	projectID := strings.TrimPrefix(asset.Id, "apps/")

	// Add all currently running versions to the table.
	for _, svc := range asset.GetReportedState().GetAppengine().GetServices() {
		for _, ver := range svc.Versions {
			// Visit only active or only inactive versions, based on `active` flag.
			traffic := (svc.TrafficAllocation[ver.Name] * 100) / 1000
			if active != (traffic != 0) {
				continue
			}

			// Trim giant email suffixes of service accounts. There are very few of
			// service accounts that should be deploying stuff, and they are easily
			// identified by their name alone.
			deployer := ver.GetCapturedState().CreatedBy
			if strings.HasSuffix(deployer, ".gserviceaccount.com") {
				deployer = strings.Split(deployer, "@")[0]
			}

			versions[versionID{svc.Name, ver.Name}] = versionState{
				Service:         appengineServiceHref(projectID, svc.Name),
				Version:         appengineVersionHref(projectID, svc.Name, ver.Name),
				Deployed:        timestampHref(ver.GetCapturedState().CreateTime, "", "<br>by "+html.EscapeString(deployer)),
				TrafficReported: traffic,
				sortKey1:        serviceSortKey(svc.Name),
				sortKey2:        ver.GetCapturedState().CreateTime.AsTime().Unix(),
			}
		}
	}

	// Add all versions that are declared in the configs. They are all considered
	// active (even when they get 0% intended traffic), since they are "actively"
	// declared in the configs.
	if active {
		for _, svc := range asset.GetIntendedState().GetAppengine().GetServices() {
			for _, ver := range svc.Versions {
				key := versionID{svc.Name, ver.Name}
				val, ok := versions[key]
				if !ok {
					val = versionState{
						Service:  appengineServiceHref(projectID, svc.Name),
						Version:  appengineVersionHref(projectID, svc.Name, ver.Name),
						sortKey1: serviceSortKey(svc.Name),
						sortKey2: distantFuture.Unix(), // will show up before any currently deployed version
					}
				}
				val.TrafficIntended = (svc.TrafficAllocation[ver.Name] * 100) / 1000
				versions[key] = val
			}
		}
	}

	// Sort by service, then by version deployment time (most recent on top).
	versionsList := make([]versionState, 0, len(versions))
	for _, v := range versions {
		versionsList = append(versionsList, v)
	}
	sort.Slice(versionsList, func(li, ri int) bool {
		l, r := versionsList[li], versionsList[ri]
		if l.sortKey1 == r.sortKey1 {
			return l.sortKey2 > r.sortKey2 // reversed intentionally
		}
		return l.sortKey1 < r.sortKey1
	})

	if len(versionsList) == 0 {
		return versionsList
	}

	// Populate RowSpan by "merging" rows with the same Service name.
	curIdx := 0
	for i := range versionsList {
		if versionsList[i].Service != versionsList[curIdx].Service {
			curIdx = i
		}
		versionsList[curIdx].RowSpan += 1
	}

	return versionsList
}

////////////////////////////////////////////////////////////////////////////////

// assetPage renders the asset page.
func (ui *UI) assetPage(ctx *router.Context, assetID string) error {
	const historyLimit = 10

	assetHistory, err := ui.assets.ListAssetHistory(ctx.Context, &rpcpb.ListAssetHistoryRequest{
		AssetId: assetID,
		Limit:   historyLimit,
	})
	if err != nil {
		return err
	}

	if assetHistory.Current != nil {
		// TODO: Show detailed UI.
	}

	history := make([]*historyOverview, len(assetHistory.History))
	for i, rec := range assetHistory.History {
		history[i] = deriveHistoryOverview(assetHistory.Asset, rec)
	}

	ref := assetRefFromID(assetHistory.Asset.Id)

	templates.MustRender(ctx.Context, ctx.Writer, "pages/asset.html", map[string]interface{}{
		"Breadcrumbs":       assetBreadcrumbs(ref),
		"Ref":               ref,
		"Overview":          deriveAssetOverview(assetHistory.Asset),
		"ActiveVersions":    versionsSummary(assetHistory.Asset, true),
		"InactiveVersions":  versionsSummary(assetHistory.Asset, false),
		"History":           history,
		"LikelyMoreHistory": len(history) == historyLimit,
		"HistoryHref":       fmt.Sprintf("/a/%s/history", assetID),
	})
	return nil
}
