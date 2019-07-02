// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/buildbucket/deprecated"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

// builderPageBuildFields is a field mask used by builder page in build search
// requests.
var builderPageBuildFields = &field_mask.FieldMask{
	Paths: []string{
		"builds.*.id",
		"builds.*.builder",
		"builds.*.number",
		"builds.*.status",
		"builds.*.create_time",
		"builds.*.end_time",
		"builds.*.start_time",
		"builds.*.input.gerrit_changes",
		"builds.*.input.gitiles_commit",
		"builds.*.output.gitiles_commit",
		"builds.*.summary_markdown",
		"next_page_token",
	},
}

// LegacyBuilderIDString returns a legacy string identifying the builder.
// It is used in the Milo datastore.
func LegacyBuilderIDString(bid *buildbucketpb.BuilderID) string {
	return fmt.Sprintf("buildbucket/luci.%s.%s/%s", bid.Project, bid.Bucket, bid.Builder)
}

// ensureDefined returns a grpcutil.NotFoundTag tagged error if the builder is
// not defined.
func ensureDefined(c context.Context, host string, bid *buildbucketpb.BuilderID) error {
	definedBuilders, err := getBuilders(c, host)
	if err != nil {
		return err
	}

	v1Bucket := deprecated.BucketNameToV1(bid.Project, bid.Bucket)
	for _, bucket := range definedBuilders.Buckets {
		if bucket.Name != v1Bucket {
			continue
		}
		for _, builder := range bucket.Builders {
			if builder.Name == bid.Builder {
				return nil
			}
		}
		break
	}

	return errors.Reason("builder %q not found", protoutil.FormatBuilderID(bid)).Tag(grpcutil.NotFoundTag).Err()
}

// backPageToken implements bidirectional page tokens with forward-only
// buildbucket page tokens by storing a map for pageToken -> prevPageToken in
// memcache.
// backPageToken returns a previous page token given thisPageToken, and caches
// thisPageToken as the previous pageToken of nextPageToken.
func backPageToken(c context.Context, bid *buildbucketpb.BuilderID, pageSize int, thisPageToken, nextPageToken string) string {
	memcacheKey := func(pageToken string) string {
		key := fmt.Sprintf("%s:%d:%s", protoutil.FormatBuilderID(bid), pageSize, pageToken)
		blob := sha1.Sum([]byte(key))
		encoded := base64.StdEncoding.EncodeToString(blob[:])
		return "pageTokens:buildbucket_builders:" + encoded
	}

	prevPageToken := ""
	if thisPageToken != "" {
		if item, err := memcache.GetKey(c, memcacheKey(thisPageToken)); err == nil {
			prevPageToken = string(item.Value())
		}
	}
	if nextPageToken != "" {
		item := memcache.NewItem(c, memcacheKey(nextPageToken))
		if thisPageToken == "" {
			// TODO(iannucci): remove the magic.
			item.SetValue([]byte("EMPTY"))
		} else {
			item.SetValue([]byte(thisPageToken))
		}
		item.SetExpiration(24 * time.Hour)
		memcache.Set(c, item)
	}
	return prevPageToken
}

// viewsForBuilder returns a list of links to views that reference the builder.
func viewsForBuilder(c context.Context, bid *buildbucketpb.BuilderID) ([]*ui.Link, error) {
	consoles, err := common.GetAllConsoles(c, LegacyBuilderIDString(bid))
	if err != nil {
		return nil, err
	}

	views := make([]*ui.Link, len(consoles))
	for i, c := range consoles {
		views[i] = ui.NewLink(
			fmt.Sprintf("%s / %s", c.ProjectID(), c.ID),
			fmt.Sprintf("/p/%s/g/%s", c.ProjectID(), c.ID),
			fmt.Sprintf("view %s in project %s", c.ID, c.ProjectID()))
	}
	sort.Slice(views, func(i, j int) bool {
		return views[i].Label < views[j].Label
	})
	return views, nil
}

// GetBuilderPage computes a builder page data.
func GetBuilderPage(c context.Context, bid *buildbucketpb.BuilderID, pageSize int, pageToken string) (*ui.BuilderPage, error) {
	nowTS, _ := ptypes.TimestampProto(clock.Now(c))

	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	if pageSize <= 0 {
		pageSize = 20
	}

	result := &ui.BuilderPage{
		BuilderName: bid.Builder,
	}

	client, err := buildbucketClient(c, host, auth.AsUser)
	if err != nil {
		return nil, err
	}

	fetch := func(status buildbucketpb.Status, pageSize int, pageToken string) (builds []*ui.Build, nextPageToken string, complete bool, err error) {
		req := &buildbucketpb.SearchBuildsRequest{
			Predicate: &buildbucketpb.BuildPredicate{
				Builder:             bid,
				Status:              status,
				IncludeExperimental: true,
			},
			PageToken: pageToken,
			PageSize:  int32(pageSize),
			Fields:    builderPageBuildFields,
		}

		start := clock.Now(c)
		res, err := client.SearchBuilds(c, req)
		if err != nil {
			return
		}

		logging.Infof(c, "Fetched %d %s builds in %s", len(res.Builds), status, clock.Since(c, start))

		nextPageToken = res.NextPageToken
		complete = nextPageToken == ""

		builds = make([]*ui.Build, len(res.Builds))
		for i, b := range res.Builds {
			builds[i] = &ui.Build{
				Build: b,
				Now:   nowTS,
			}
		}
		return
	}

	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return ensureDefined(c, host, bid)
		}
		work <- func() (err error) {
			result.MachinePool, err = getPool(c, bid)
			return
		}

		work <- func() (err error) {
			result.Views, err = viewsForBuilder(c, bid)
			return
		}

		work <- func() (err error) {
			result.ScheduledBuilds, _, result.ScheduledBuildsComplete, err = fetch(buildbucketpb.Status_SCHEDULED, 100, "")
			return
		}
		work <- func() (err error) {
			result.StartedBuilds, _, result.StartedBuildsComplete, err = fetch(buildbucketpb.Status_STARTED, 100, "")
			return
		}
		work <- func() (err error) {
			result.EndedBuilds, result.NextPageToken, _, err = fetch(buildbucketpb.Status_ENDED_MASK, pageSize, pageToken)
			result.PrevPageToken = backPageToken(c, bid, pageSize, pageToken, result.NextPageToken) // Safe to do even with error.
			return
		}
	})
	if err != nil {
		return nil, err
	}

	// Handle inconsistencies.
	exclude := make(map[int64]struct{}, len(result.EndedBuilds)+len(result.StartedBuilds))
	addBuildIDs(exclude, result.EndedBuilds)
	result.StartedBuilds = excludeBuilds(result.StartedBuilds, exclude)
	addBuildIDs(exclude, result.StartedBuilds)
	result.ScheduledBuilds = excludeBuilds(result.ScheduledBuilds, exclude)

	return result, nil
}

func addBuildIDs(m map[int64]struct{}, builds []*ui.Build) {
	for _, b := range builds {
		m[b.Id] = struct{}{}
	}
}

func excludeBuilds(builds []*ui.Build, exclude map[int64]struct{}) []*ui.Build {
	ret := builds[:0]
	for _, b := range builds {
		if _, excluded := exclude[b.Id]; !excluded {
			ret = append(ret, b)
		}
	}
	return ret
}
