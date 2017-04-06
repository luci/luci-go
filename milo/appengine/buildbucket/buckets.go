// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/appengine/common"
)

func GetAllBuilders(c context.Context) (*resp.CIService, error) {
	settings := common.GetSettings(c)
	bucketSettings := settings.Buildbucket
	if bucketSettings == nil {
		return nil, errors.New("buildbucket settings missing in config")
	}
	result := &resp.CIService{
		Name: "Swarmbucket",
		Host: &resp.Link{
			Label: bucketSettings.Name,
			URL:   "https://" + bucketSettings.Host,
		},
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

	result.BuilderGroups = make([]resp.BuilderGroup, len(r.Buckets))
	for i, bucket := range r.Buckets {
		group := resp.BuilderGroup{Name: bucket.Name}
		group.Builders = make([]resp.Link, len(bucket.Builders))
		for j, builder := range bucket.Builders {
			group.Builders[j] = resp.Link{
				Label: builder.Name,
				URL:   fmt.Sprintf("/buildbucket/%s/%s", bucket.Name, builder.Name),
			}
		}
		result.BuilderGroups[i] = group
	}

	return result, nil
}
