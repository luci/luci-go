// Copyright 2018 The LUCI Authors.
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/common"
)

func packagePage(c *router.Context, pkg string) error {
	// Validate the package name and grab a parent prefix to list siblings.
	pkg = strings.Trim(pkg, "/")
	if err := common.ValidatePackageName(pkg); err != nil {
		return status.Errorf(codes.InvalidArgument, "bad package name - %s", err)
	}
	pfx := ""
	if i := strings.LastIndex(pkg, "/"); i != -1 {
		pfx = pkg[:i]
	}

	// Cursor used for paginating instance listing.
	cursor := c.Request.URL.Query().Get("c")
	prevPageURL := ""
	nextPageURL := ""

	var instances *api.ListInstancesResponse
	var siblings *api.ListPrefixResponse
	var refs *api.ListRefsResponse

	err := parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error {
			var err error
			instances, err = impl.PublicRepo.ListInstances(c.Context, &api.ListInstancesRequest{
				Package:   pkg,
				PageSize:  12,
				PageToken: cursor,
			})
			if err != nil {
				return err
			}
			if instances.NextPageToken != "" {
				instancesListing.storePrevCursor(c.Context, pkg, instances.NextPageToken, cursor)
				nextPageURL = packagePageURL(pkg, instances.NextPageToken)
			}
			if cursor != "" {
				prevPageURL = packagePageURL(pkg, instancesListing.fetchPrevCursor(c.Context, pkg, cursor))
			}
			return nil
		}
		tasks <- func() error {
			var err error
			siblings, err = impl.PublicRepo.ListPrefix(c.Context, &api.ListPrefixRequest{
				Prefix: pfx,
			})
			return err
		}
		tasks <- func() error {
			var err error
			refs, err = impl.PublicRepo.ListRefs(c.Context, &api.ListRefsRequest{
				Package: pkg,
			})
			return err
		}
	})
	if err != nil {
		return err
	}

	// Mapping "instance ID" => list of refs pointing to it.
	refMap := make(map[string][]*api.Ref, len(refs.Refs))
	for _, ref := range refs.Refs {
		iid := common.ObjectRefToInstanceID(ref.Instance)
		refMap[iid] = append(refMap[iid], ref)
	}

	// Build instance listing, annotating instances with refs that point to them.
	type instanceRef struct {
		Title string
		Href  string
	}
	type instanceItem struct {
		Title string
		Href  string
		Refs  []instanceRef
		Age   string
	}

	now := clock.Now(c.Context).UTC()
	instListing := make([]instanceItem, len(instances.Instances))
	for i, inst := range instances.Instances {
		iid := common.ObjectRefToInstanceID(inst.Instance)
		instListing[i] = instanceItem{
			Title: iid,
			Href:  instancePageURL(pkg, iid),
			Age:   humanize.RelTime(google.TimeFromProto(inst.RegisteredTs), now, "", ""),
		}
		if refs := refMap[iid]; len(refs) != 0 {
			refList := make([]instanceRef, len(refs))
			for i, ref := range refs {
				refList[i] = instanceRef{
					Title: ref.Name,
					Href:  instancePageURL(pkg, ref.Name),
				}
			}
			instListing[i].Refs = refList
		}
	}

	templates.MustRender(c.Context, c.Writer, "pages/index.html", map[string]interface{}{
		"Package":     pkg,
		"Breadcrumbs": breadcrumbs(pfx),
		"Prefixes":    prefixesListing(pfx, siblings.Prefixes),
		"Packages":    packagesListing(pfx, siblings.Packages, pkg),
		"Instances":   instListing,
		"NextPageURL": nextPageURL,
		"PrevPageURL": prevPageURL,
	})
	return nil
}
