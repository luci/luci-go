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
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/buildbucket"
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
)

// BuildID implements buildsource.ID, and is the buildbucket notion of a build.
// It references a buildbucket build which may reference a swarming build.
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// Address is the Buildbucket's build address (required)
	Address string
}

// GetSwarmingID returns the swarming task ID of a buildbucket build.
func GetSwarmingID(c context.Context, buildAddress string) (*swarming.BuildID, error) {
	bs := &model.BuildSummary{BuildKey: MakeBuildKey(c, buildAddress)}
	switch err := datastore.Get(c, bs); err {
	case nil:
		for _, ctx := range bs.ContextURI {
			u, err := url.Parse(ctx)
			if err != nil {
				continue
			}
			if u.Scheme == "swarming" && len(u.Path) > 1 {
				toks := strings.Split(u.Path[1:], "/")
				if toks[0] == "task" {
					return &swarming.BuildID{Host: u.Host, TaskID: toks[1]}, nil
				}
			}
		}
		return nil, errors.New("no swarming task context")

	case datastore.ErrNoSuchEntity:
		// continue to the fallback code below.

	default:
		return nil, err
	}

	// DEPRECATED(2017-12-01) {{
	// This makes an RPC to buildbucket to obtain the swarming task ID.
	// Now that we include this data in the BuildSummary.ContextUI we should never
	// need to do this extra RPC. However, we have this codepath in place for old
	// builds.
	//
	// After the deprecation date, this code can be removed; the only effect will
	// be that buildbucket builds before 2017-11-03 will not render.
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}

	var msg *bbapi.ApiCommonBuildMessage
	if id, err := strconv.ParseInt(buildAddress, 10, 64); err == nil {
		resp, err := client.Get(id).Context(c).Do()
		switch {
		case err != nil:
			return nil, err
		case resp.Error.Reason == bbapi.ReasonNotFound:
			return nil, errors.Reason("build %d not found", id).Tag(common.CodeNotFound).Err()
		}
		msg = resp.Build
	} else {
		msgs, err := client.Search().
			Context(c).
			Tag(strpair.Format(buildbucket.TagBuildAddress, buildAddress)).
			Fetch(1, nil)
		switch {
		case err != nil:
			return nil, err
		case len(msgs) == 0:
			return nil, errors.Reason("build %q not found", buildAddress).Tag(common.CodeNotFound).Err()
		}
		msg = msgs[0]
	}
	tags := strpair.ParseMap(msg.Tags)
	shost := tags.Get("swarming_hostname")
	sid := tags.Get("swarming_task_id")
	if shost == "" || sid == "" {
		return nil, errors.New("not a valid LUCI build")
	}
	return &swarming.BuildID{Host: shost, TaskID: sid}, nil
	// }}
}

// Get returns a resp.MiloBuild based off of the buildbucket ID given by
// finding the coorisponding swarming build.
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	sID, err := GetSwarmingID(c, b.Address)
	if err != nil {
		return nil, err
	}
	return sID.Get(c)
}

func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	return "", false, errors.New("buildbucket builds do not implement GetLog")
}
