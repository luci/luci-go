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
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

func GetAllBuilders(c context.Context) (*ui.CIService, error) {
	settings := common.GetSettings(c)
	bucketSettings := settings.Buildbucket
	if bucketSettings == nil {
		return nil, errors.New("buildbucket settings missing in config")
	}
	result := &ui.CIService{
		Name: "Swarmbucket",
		Host: ui.NewLink(bucketSettings.Name, "https://"+bucketSettings.Host,
			fmt.Sprintf("buildbucket settings for %s", bucketSettings.Name)),
	}
	client, err := newSwarmbucketClient(c, bucketSettings.Host)
	if err != nil {
		return nil, err
	}
	// TODO(hinoka): Retries for transient errors
	r, err := client.GetBuilders().Do()
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
