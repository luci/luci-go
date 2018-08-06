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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl"
	"go.chromium.org/luci/cipd/common"
)

func prefixListingPage(c *router.Context, pfx string) error {
	pfx, err := common.ValidatePackagePrefix(strings.Trim(pfx, "/"))
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "bad prefix - %s", err)
	}

	var listing *api.ListPrefixResponse
	var meta *metadataBlock

	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error {
			var err error
			listing, err = impl.PublicRepo.ListPrefix(c.Context, &api.ListPrefixRequest{
				Prefix: pfx,
			})
			return err
		}
		tasks <- func() error {
			var err error
			meta, err = fetchMetadata(c.Context, pfx)
			return err
		}
	})
	if err != nil {
		return err
	}

	templates.MustRender(c.Context, c.Writer, "pages/index.html", map[string]interface{}{
		"Breadcrumbs": breadcrumbs(pfx),
		"Prefixes":    prefixesListing(pfx, listing.Prefixes),
		"Packages":    packagesListing(pfx, listing.Packages, ""),
		"Metadata":    meta,
		"Instances":   nil,
	})
	return nil
}
