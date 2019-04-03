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
	"strings"
	"time"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/buildbucket/swarmbucket/v1"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

// GetBuilders returns all Swarmbucket builders, cached for current identity.
func GetBuilders(c context.Context) (*swarmbucket.LegacySwarmbucketApiGetBuildersResponseMessage, error) {
	host := common.GetSettings(c).GetBuildbucket().GetHost()
	if host == "" {
		return nil, errors.New("buildbucket host is missing in config")
	}
	return getBuilders(c, host)
}

func getBuilders(c context.Context, host string) (*swarmbucket.LegacySwarmbucketApiGetBuildersResponseMessage, error) {
	mc := memcache.NewItem(c, fmt.Sprintf("swarmbucket-builders-%q-%q", host, auth.CurrentIdentity(c)))
	switch err := memcache.Get(c, mc); {
	case err == memcache.ErrCacheMiss:
	case err != nil:
		logging.WithError(err).Warningf(c, "memcache.get failed while loading swarmbucket builders")
	default:
		var res swarmbucket.LegacySwarmbucketApiGetBuildersResponseMessage
		if err := json.Unmarshal(mc.Value(), &res); err != nil {
			logging.WithError(err).Warningf(c, "corrupted swarmbucket builders cache")
		} else {
			return &res, nil
		}
	}

	client, err := newSwarmbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	// TODO(hinoka): Retries for transient errors
	res, err := client.GetBuilders().Do()
	if err != nil {
		return nil, err
	}

	if data, err := json.Marshal(res); err == nil {
		mc.SetValue(data).SetExpiration(10 * time.Minute)
		if err := memcache.Set(c, mc); err != nil {
			logging.WithError(err).Warningf(c, "failed to cache swarmbucket builders")
		}
	}

	return res, nil
}

// CIService returns a *ui.CIService containing all known buckets and builders.
func CIService(c context.Context) (*ui.CIService, error) {
	bucketSettings := common.GetSettings(c).GetBuildbucket()
	host := bucketSettings.GetHost()
	if host == "" {
		return nil, errors.New("buildbucket host is missing in config")
	}
	result := &ui.CIService{
		Name: "LUCI",
		Host: ui.NewLink(bucketSettings.Name, "https://"+host,
			fmt.Sprintf("buildbucket settings for %s", bucketSettings.Name)),
	}

	r, err := getBuilders(c, host)
	if err != nil {
		return nil, err
	}

	result.BuilderGroups = make([]ui.BuilderGroup, len(r.Buckets))
	for i, bucket := range r.Buckets {
		// TODO(nodir): instead of assuming luci.<project>. bucket prefix,
		// expect project explicitly in bucket struct.
		if !strings.HasPrefix(bucket.Name, "luci.") {
			continue
		}
		// buildbucket guarantees that buckets that start with "luci.",
		// start with "luci.<project id>." prefix.
		project := strings.Split(bucket.Name, ".")[1]
		group := ui.BuilderGroup{Name: bucket.Name}
		group.Builders = make([]ui.Link, len(bucket.Builders))
		for j, builder := range bucket.Builders {
			group.Builders[j] = *ui.NewLink(
				builder.Name, fmt.Sprintf("/p/%s/builders/%s/%s", project, bucket.Name, builder.Name),
				fmt.Sprintf("buildbucket builder %s in bucket %s", builder.Name, bucket.Name))
		}
		result.BuilderGroups[i] = group
	}

	return result, nil
}
