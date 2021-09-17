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

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"

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

// backPageToken implements bidirectional page tokens with forward-only
// buildbucket page tokens by storing a map for pageToken -> prevPageToken in
// memcache.
// backPageToken returns a previous page token given thisPageToken, and caches
// thisPageToken as the previous pageToken of nextPageToken.
func backPageToken(c context.Context, bid *buildbucketpb.BuilderID, pageSize int, thisPageToken, nextPageToken string) string {
	cacheKey := func(pageToken string) string {
		key := fmt.Sprintf("%s:%d:%s", protoutil.FormatBuilderID(bid), pageSize, pageToken)
		blob := sha1.Sum([]byte(key))
		return base64.StdEncoding.EncodeToString(blob[:])
	}

	cache := caching.GlobalCache(c, "builder-page-tokens")
	if cache == nil {
		logging.Errorf(c, "global cache not available in context")
		return ""
	}

	prevPageToken := ""
	if thisPageToken != "" {
		bytes, err := cache.Get(c, cacheKey(thisPageToken))
		if err == nil {
			prevPageToken = string(bytes)
		}
	}
	if nextPageToken != "" {
		cacheValue := thisPageToken
		// TODO(weiweilin): this is required to let the page know there's
		// a previous page token (with value ""). We can remove the magic
		// once we switch to the chained page token implementation.
		if cacheValue == "" {
			cacheValue = "EMPTY"
		}
		if err := cache.Set(c, cacheKey(nextPageToken), []byte(cacheValue), 24*time.Hour); err != nil {
			logging.WithError(err).Errorf(c, "failed to cache the page token")
		}
	}
	return prevPageToken
}

// viewsForBuilder returns a list of links to views that reference the builder.
func viewsForBuilder(c context.Context, bid *buildbucketpb.BuilderID) ([]*ui.Link, error) {
	consoles, err := common.GetAllConsoles(c, common.LegacyBuilderIDString(bid))
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
	nowTS := timestamppb.New(clock.Now(c))

	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	if pageSize <= 0 {
		pageSize = 20
	}

	result := &ui.BuilderPage{}

	buildsClient, err := buildbucketBuildsClient(c, host, auth.AsUser)
	if err != nil {
		return nil, err
	}
	buildersClient, err := buildbucketBuildersClient(c, host, auth.AsUser)
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
		res, err := buildsClient.SearchBuilds(c, req)
		if err != nil {
			err = common.TagGRPC(c, err)
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
		work <- func() (err error) {
			req := &buildbucketpb.GetBuilderRequest{
				Id: bid,
			}
			result.Builder, err = buildersClient.GetBuilder(c, req)
			if err != nil {
				err = common.TagGRPC(c, err)
			}
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
