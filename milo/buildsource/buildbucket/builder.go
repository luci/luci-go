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
	"time"

	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/memcache"
	bb "go.chromium.org/luci/buildbucket"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

// BuilderID represents a buildbucket builder.  We wrap the underlying representation
// since we represent builder IDs slightly differently in Milo vs. Buildbucket.
// I.E. Builders can source from either BuildBot or Buildbucket.
type BuilderID struct {
	// BuilderID is the buildbucket v2 representation of the builder ID.  Note
	// that the v2 representation uses short bucket names.
	buildbucketpb.BuilderID
}

// NewBuilderID does what it says.
func NewBuilderID(v1Bucket, builder string) (bid BuilderID) {
	bid.Project, bid.Bucket = bb.BucketNameToV2(v1Bucket)
	bid.Builder = builder
	return
}

// V1Bucket returns the buildbucket v1 representation of the bucket name, which
// is what we use in Milo.
func (b BuilderID) V1Bucket() string {
	return fmt.Sprintf("luci.%s.%s", b.Project, b.Bucket)
}

// String returns the canonical format of BuilderID.
func (b BuilderID) String() string {
	return fmt.Sprintf("buildbucket/%s/%s", b.V1Bucket(), b.Builder)
}

// fetchBuilds fetches builds given a criteria.
// The returned builds are sorted by build creation descending.
// count defines maximum number of builds to fetch; if <0, defaults to 100.
func fetchBuilds(c context.Context, client buildbucketpb.BuildsClient, bid BuilderID,
	status buildbucketpb.Status, limit int32, cursor string) ([]*buildbucketpb.Build, string, error) {

	c, _ = context.WithTimeout(c, bbRPCTimeout)
	if limit < 0 {
		limit = 100
	}
	r := &buildbucketpb.SearchBuildsRequest{
		Predicate: &buildbucketpb.BuildPredicate{
			Builder:             &bid.BuilderID,
			Status:              status,
			IncludeExperimental: true,
		},
		PageSize:  limit,
		PageToken: cursor,
	}

	start := clock.Now(c)
	resp, err := client.SearchBuilds(c, r)
	builds := resp.GetBuilds()
	logging.Infof(c, "Fetched %d %s builds in %s", len(builds), status, clock.Since(c, start))
	return builds, resp.GetNextPageToken(), err
}

// ensureDefined returns common.CodeNotFound tagged error if a builder is not
// defined in its swarmbucket.
func ensureDefined(c context.Context, host string, bid BuilderID) error {
	client, err := newSwarmbucketClient(c, host)
	if err != nil {
		return err
	}
	getBuilders := client.GetBuilders()
	getBuilders.Bucket(bid.V1Bucket())
	getBuilders.Fields(googleapi.Field("buckets/(builders/name,name)"))
	res, err := getBuilders.Do()
	if err != nil {
		return err
	}

	for _, bucket := range res.Buckets {
		if bucket.Name != bid.V1Bucket() {
			continue // defensive programming; shouldn't happen in practice.
		}
		for _, builder := range bucket.Builders {
			if builder.Name == bid.Builder {
				return nil
			}
		}
	}
	return errors.Reason("builder %q not found", bid.Builder).Tag(common.CodeNotFound).Err()
}

func getHost(c context.Context) (string, error) {
	settings := common.GetSettings(c)
	if settings.Buildbucket == nil || settings.Buildbucket.Host == "" {
		return "", errors.New("missing buildbucket host in settings")
	}
	return settings.Buildbucket.Host, nil
}

// backCursor implements bidirectional cursors with forward-only datastore
// cursors by storing a map for cursor -> prevCursor in memcache.
// backCursor returns a previous cursor given thisCursor, and caches thisCursor
// to be the previous cursor of nextCursor.
func backCursor(c context.Context, bid BuilderID, limit int32, thisCursor, nextCursor string) string {
	memcacheKey := func(cursor string) string {
		key := fmt.Sprintf("%s:%d:%s", bid.String(), limit, cursor)
		blob := sha1.Sum([]byte(key))
		encoded := base64.StdEncoding.EncodeToString(blob[:])
		return "cursors:buildbucket_builders:" + encoded
	}

	prevCursor := ""
	if thisCursor != "" {
		if item, err := memcache.GetKey(c, memcacheKey(thisCursor)); err == nil {
			prevCursor = string(item.Value())
		}
	}
	if nextCursor != "" {
		item := memcache.NewItem(c, memcacheKey(nextCursor))
		if thisCursor == "" {
			item.SetValue([]byte("EMPTY"))
		} else {
			item.SetValue([]byte(thisCursor))
		}
		item.SetExpiration(24 * time.Hour)
		memcache.Set(c, item)
	}
	return prevCursor
}

// GetBuilder is used by buildsource.BuilderID.Get to obtain the resp.Builder.
func GetBuilder(c context.Context, bid BuilderID, limit int32, cursor string) (*ui.Builder, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	if limit < 0 {
		limit = 20
	}

	result := &ui.Builder{
		Name: bid.Builder,
	}
	client, err := buildbucketClient(c, host)
	if err != nil {
		return nil, err
	}

	fetch := func(statusFilter buildbucketpb.Status, limit int32, cursor string) (result []*buildbucketpb.Build, nextCursor string, err error) {
		result, nextCursor, err = fetchBuilds(c, client, bid, statusFilter, limit, cursor)
		if err != nil {
			logging.WithError(err).Errorf(c, "Could not fetch %s builds", statusFilter)
		}
		return
	}
	return result, parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			return ensureDefined(c, host, bid)
		}
		work <- func() (err error) {
			result.MachinePool, err = getPool(c, bid)
			return
		}
		work <- func() (err error) {
			result.PendingBuilds, _, err = fetch(buildbucketpb.Status_SCHEDULED, -1, "")
			return
		}
		work <- func() (err error) {
			result.CurrentBuilds, _, err = fetch(buildbucketpb.Status_STARTED, -1, "")
			return
		}
		work <- func() (err error) {
			result.FinishedBuilds, result.NextCursor, err = fetch(buildbucketpb.Status_ENDED_MASK, limit, cursor)
			result.PrevCursor = backCursor(c, bid, limit, cursor, result.NextCursor) // Safe to do even with error.
			return
		}
	})
}
