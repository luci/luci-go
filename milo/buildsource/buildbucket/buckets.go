// Copyright 2017 The LUCI Authors.
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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/buildbucket/bbperms"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

const (
	// Keep the builders in cache for 10 mins to speed up repeated page loads and
	// reduce stress on buildbucket side.
	// But this also means newly added/removed builders would take 10 mins to
	// propagate.
	// Cache duration can be adjusted if needed.
	cacheDuration = 10 * time.Minute

	// Refresh the builders cache if the cache TTL falls below this threshold.
	cacheRefreshThreshold = cacheDuration - time.Minute
)

var buildbucketBuildersCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(64),
	GlobalNamespace: "buildbucket-builders-v3",
	Marshal:         json.Marshal,
	Unmarshal: func(blob []byte) (interface{}, error) {
		res := make([]*buildbucketpb.BuilderID, 0)
		err := json.Unmarshal(blob, &res)
		return res, err
	},
}

// getAllBuilders returns all cached buildbucket builders. If the cache expired,
// refresh it with Milo's credential.
func getAllBuilders(c context.Context, host string, opt ...layered.Option) ([]*buildbucketpb.BuilderID, error) {
	builders, err := buildbucketBuildersCache.GetOrCreate(c, host, func() (v interface{}, exp time.Duration, err error) {
		start := time.Now()

		buildersClient, err := ProdBuildersClientFactory(c, host, auth.AsSelf)
		if err != nil {
			return nil, 0, err
		}

		// Get all the Builder IDs from buildbucket.
		bids := make([]*buildbucketpb.BuilderID, 0)
		req := &buildbucketpb.ListBuildersRequest{PageSize: 1000}
		for {
			r, err := buildersClient.ListBuilders(c, req)
			if err != nil {
				return nil, 0, err
			}

			for _, builder := range r.Builders {
				bids = append(bids, builder.Id)
			}

			if r.NextPageToken == "" {
				break
			}
			req.PageToken = r.NextPageToken
		}

		logging.Infof(c, "listing all builders from buildbucket took %v", time.Since(start))

		return bids, cacheDuration, nil
	})
	if err != nil {
		return nil, err
	}

	return builders.([]*buildbucketpb.BuilderID), nil
}

// filterVisibleBuilders returns a list of builders that are visible to the
// current user.
func filterVisibleBuilders(c context.Context, builders []*buildbucketpb.BuilderID) ([]*buildbucketpb.BuilderID, error) {
	filteredBuilders := make([]*buildbucketpb.BuilderID, 0)

	bucketPermissions := make(map[string]bool)

	for _, builder := range builders {
		realm := realms.Join(builder.Project, builder.Bucket)

		allowed, ok := bucketPermissions[realm]
		if !ok {
			var err error
			allowed, err = auth.HasPermission(c, bbperms.BuildersList, realm, nil)
			if err != nil {
				return nil, err
			}
			bucketPermissions[realm] = allowed
		}

		if !allowed {
			continue
		}

		filteredBuilders = append(filteredBuilders, builder)
	}

	return filteredBuilders, nil
}

// UpdateBuilders updates the builders cache if the cache TTL falls below
// cacheRefreshThreshold.
func UpdateBuilders(c context.Context) error {
	bucketSettings := common.GetSettings(c).GetBuildbucket()
	host := bucketSettings.GetHost()
	if host == "" {
		return errors.New("buildbucket host is missing in config")
	}
	_, err := getAllBuilders(c, host, layered.WithMinTTL(cacheRefreshThreshold))
	return err
}

// CIService returns a *ui.CIService containing all known buckets and builders.
func CIService(c context.Context) (*ui.CIService, error) {
	bucketSettings := common.GetSettings(c).GetBuildbucket()
	host := bucketSettings.GetHost()
	if host == "" {
		return nil, errors.New("buildbucket host is missing in config")
	}
	result := &ui.CIService{
		Host: ui.NewLink(bucketSettings.Name, "https://"+host,
			fmt.Sprintf("buildbucket settings for %s", bucketSettings.Name)),
	}

	builders, err := getAllBuilders(c, host)
	if err != nil {
		return nil, err
	}

	builders, err = filterVisibleBuilders(c, builders)
	if err != nil {
		return nil, err
	}

	builderGroups := make(map[string]*ui.BuilderGroup)

	for _, builder := range builders {
		bucketID := builder.Project + "/" + builder.Bucket
		group, ok := builderGroups[bucketID]
		if !ok {
			group = &ui.BuilderGroup{Name: bucketID}
			builderGroups[bucketID] = group
		}

		group.Builders = append(group.Builders, *ui.NewLink(
			builder.Builder, fmt.Sprintf("/p/%s/builders/%s/%s", builder.Project, builder.Bucket, builder.Builder),
			fmt.Sprintf("buildbucket builder %s in bucket %s", builder.Builder, bucketID)))
	}

	result.BuilderGroups = make([]ui.BuilderGroup, 0, len(builderGroups))
	for _, builderGroup := range builderGroups {
		builderGroup.Sort()
		result.BuilderGroups = append(result.BuilderGroups, *builderGroup)
	}
	result.Sort()
	return result, nil
}
