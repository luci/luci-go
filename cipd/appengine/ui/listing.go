// Copyright 2023 The LUCI Authors.
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
	"strings"

	"github.com/dustin/go-humanize"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

const rootPackage = ""

func listingPage(c *router.Context, pkg string) error {
	pkg = strings.Trim(pkg, "/")
	if pkg != rootPackage {
		if err := common.ValidatePackageName(pkg); err != nil {
			return status.Errorf(codes.InvalidArgument, "%s", err)
		}
	}

	// Note PublicRepo is essentially CIPD server public API and it checks ACLs
	// in every call. Thus UI cannot be tricked to reveal anything that is not
	// already accessible via RPC API.
	ctx := c.Request.Context()
	svc := state(ctx).services.PublicRepo

	// Cursor used for paginating instance listing.
	cursor := c.Request.URL.Query().Get("c")
	prevPageURL := ""
	nextPageURL := ""

	eg, gctx := errgroup.WithContext(ctx)

	// Fetch ACLs of to this package to display them. Note this is useful to show
	// even if there's no such package yet.
	var meta *prefixMetadataBlock
	eg.Go(func() error {
		var err error
		meta, err = fetchPrefixMetadata(gctx, pkg)
		return err
	})

	// List the children of this prefix. If there are no children, this is a leaf
	// package and we need fetch siblings instead. Such leaf packages are shown
	// together with their siblings in the UI in a "highlighted" state. This also
	// detects non-existing packages or prefixes.
	var relatives *repopb.ListPrefixResponse
	var missing bool
	eg.Go(func() error {
		var err error
		relatives, err = svc.ListPrefix(gctx, &repopb.ListPrefixRequest{
			Prefix: pkg,
		})
		if err != nil || pkg == rootPackage || len(relatives.Packages) != 0 || len(relatives.Prefixes) != 0 {
			return err
		}
		// There are no child packages. Try to find siblings by listing the parent.
		parent := ""
		if i := strings.LastIndex(pkg, "/"); i != -1 {
			parent = pkg[:i]
		}
		relatives, err = svc.ListPrefix(gctx, &repopb.ListPrefixRequest{
			Prefix: parent,
		})
		if err != nil {
			return err
		}
		// Check if `pkg` actually exists (as a prefix or package). We should not
		// show siblings of a missing entity, it looks very confusing.
		isInList := func(s string, l []string) bool {
			for _, p := range l {
				if p == s {
					return true
				}
			}
			return false
		}
		missing = !isInList(pkg, relatives.Packages) && !isInList(pkg, relatives.Prefixes)
		return nil
	})

	// Fetch instance of this package, if it is a package. This will be empty if
	// it is not a package or it doesn't exist or not visible. Non-existing
	// entities are already checked by the prefix listing logic above.
	var instances []*repopb.Instance
	if pkg != rootPackage {
		eg.Go(func() error {
			resp, err := svc.ListInstances(gctx, &repopb.ListInstancesRequest{
				Package:   pkg,
				PageSize:  12,
				PageToken: cursor,
			})
			switch status.Code(err) {
			case codes.OK: // carry on
			case codes.NotFound, codes.PermissionDenied:
				return nil
			default:
				return err
			}
			if resp.NextPageToken != "" {
				instancesListing.storePrevCursor(gctx, pkg, resp.NextPageToken, cursor)
				nextPageURL = listingPageURL(pkg, resp.NextPageToken)
			}
			if cursor != "" {
				prevPageURL = listingPageURL(pkg, instancesListing.fetchPrevCursor(gctx, pkg, cursor))
			}
			instances = resp.Instances
			return nil
		})
	}

	// Fetch refs of this package, if it is a package.
	var refs []*repopb.Ref
	if pkg != rootPackage {
		eg.Go(func() error {
			resp, err := svc.ListRefs(gctx, &repopb.ListRefsRequest{
				Package: pkg,
			})
			switch status.Code(err) {
			case codes.OK: // carry on
			case codes.NotFound, codes.PermissionDenied:
				return nil
			default:
				return err
			}
			refs = resp.Refs
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	// Mapping "instance ID" => list of refs pointing to it.
	refMap := make(map[string][]*repopb.Ref, len(refs))
	for _, ref := range refs {
		iid := common.ObjectRefToInstanceID(ref.Instance)
		refMap[iid] = append(refMap[iid], ref)
	}

	// Build instance listing, annotating instances with refs that point to them.
	now := clock.Now(ctx).UTC()
	type instanceItem struct {
		ID          string
		TruncatedID string
		Href        string
		Refs        []refItem
		Age         string
	}
	instListing := make([]instanceItem, len(instances))
	for i, inst := range instances {
		iid := common.ObjectRefToInstanceID(inst.Instance)
		instListing[i] = instanceItem{
			ID:          iid,
			TruncatedID: iid[:30],
			Href:        instancePageURL(pkg, iid),
			Age:         humanize.RelTime(inst.RegisteredTs.AsTime(), now, "", ""),
			Refs:        refsListing(refMap[iid], pkg, now),
		}
	}

	templates.MustRender(ctx, c.Writer, "pages/index.html", map[string]any{
		"Package":     pkg,
		"Missing":     missing,
		"Breadcrumbs": breadcrumbs(pkg, "", len(instListing) != 0),
		"Listing":     prefixListing(pkg, relatives),
		"Metadata":    meta,
		"Instances":   instListing,
		"Refs":        refsListing(refs, pkg, now),
		"NextPageURL": nextPageURL,
		"PrevPageURL": prevPageURL,
	})
	return nil
}

type listingItem struct {
	Title   string
	Href    string
	Back    bool
	Active  bool
	Prefix  bool
	Package bool
}

func prefixListing(pkg string, relatives *repopb.ListPrefixResponse) []*listingItem {
	var listing []*listingItem

	title := func(pfx string) string {
		if pfx == rootPackage {
			return "[root]"
		}
		return pfx[strings.LastIndex(pfx, "/")+1:]
	}

	// The "go up" item is always first unless we already at root.
	if pkg != rootPackage {
		parent := ""
		if i := strings.LastIndex(pkg, "/"); i != -1 {
			parent = pkg[:i]
		}
		listing = append(listing, &listingItem{
			Back: true,
			Href: listingPageURL(parent, ""),
		})
	}

	// This will list either children or sibling of the current package (prefixes
	// and packages), depending on if it is a leaf or not.
	prefixes := make(map[string]*listingItem, len(relatives.Prefixes))
	for _, p := range relatives.Prefixes {
		item := &listingItem{
			Title:  title(p),
			Href:   listingPageURL(p, ""),
			Prefix: true,
		}
		listing = append(listing, item)
		prefixes[p] = item
	}
	for _, p := range relatives.Packages {
		if item := prefixes[p]; item != nil {
			item.Package = true
		} else {
			listing = append(listing, &listingItem{
				Title:   title(p),
				Href:    listingPageURL(p, ""),
				Active:  p == pkg, // can be true only when listing siblings
				Package: true,
			})
		}
	}
	return listing
}
