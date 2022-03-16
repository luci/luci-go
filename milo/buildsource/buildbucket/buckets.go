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

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

var buildbucketBuildersCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(65536),
	GlobalNamespace: "buildbucket-builders-v2",
	Marshal:         json.Marshal,
	Unmarshal: func(blob []byte) (interface{}, error) {
		res := make([]*buildbucketpb.BuilderID, 0)
		err := json.Unmarshal(blob, &res)
		return res, err
	},
}

// getBuilders returns all buildbucket builders, cached for current identity.
func getBuilders(c context.Context, host string) ([]*buildbucketpb.BuilderID, error) {
	key := fmt.Sprintf("%q-%q", host, auth.CurrentIdentity(c))
	builderIds, err := buildbucketBuildersCache.GetOrCreate(c, key, func() (v interface{}, exp time.Duration, err error) {
		start := time.Now()

		authOpt := auth.AsSessionUser
		// Use NoAuth when user is not signed in so RPC calls won't return
		// ErrNotConfigured.
		if auth.CurrentIdentity(c) == identity.AnonymousIdentity {
			authOpt = auth.NoAuth
		}
		buildersClient, err := ProdBuildersClientFactory(c, host, authOpt)
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

		// Keep the builders in cache for 12 hours to speed up repeated page loads
		// and reduce stress on buildbucket side.
		// But this also means builder visibility ACL changes would take 12 hours to
		// propagate.
		// Cache duration can be adjusted if needed.
		return bids, 12 * time.Hour, nil
	})
	if err != nil {
		return nil, err
	}

	return builderIds.([]*buildbucketpb.BuilderID), nil
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

	builders, err := getBuilders(c, host)
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
