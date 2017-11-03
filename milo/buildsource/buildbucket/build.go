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
	"fmt"
	"net/http"

	"golang.org/x/net/context"

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
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// Address is the Buildbucket build address, see buildbucket.Build.Address.
	Address string
}

// getBucketBuild fetches the buildbucket build given the address.
func fetchBuild(c context.Context, client *bbapi.Service, address string) (*buildbucket.Build, error) {
	id, _, _, _, _, err := buildbucket.ParseAddress(address)
	if err != nil {
		return nil, errors.Annotate(
			err, "invalid build address %q", address).Tag(common.CodeParameterError).Err()
	}

	var msg *bbapi.ApiCommonBuildMessage
	if id != 0 {
		response, err := client.Get(id).Context(c).Do()
		switch {
		case err != nil:
			// Generic error.
			return nil, errors.Annotate(err, "fetching build %d", id).Err()
		case response.HTTPStatusCode == http.StatusForbidden:
			return nil, errors.New("access forbidden", common.CodeNoAccess)
		case response.HTTPStatusCode == http.StatusNotFound,
			response.Error != nil && response.Error.Reason == bbapi.ReasonNotFound:
			return nil, errors.New("build not found", common.CodeNotFound)
		case response.Error != nil:
			return nil, fmt.Errorf(
				"reason: %s, message: %s", response.Error.Reason, response.Error.Message)
		}
		msg = response.Build
	} else {
		msgs, err := client.Search().Context(c).Tag(strpair.Format(buildbucket.TagBuildAddress, address)).Fetch(1, nil)
		switch {
		case err != nil:
			// Generic error.
			return nil, errors.Annotate(err, "fetching build %d", id).Err()
		case len(msgs) == 0:
			return nil, errors.New("build not found", common.CodeNotFound)
		}
		msg = msgs[0]
	}
	build := &buildbucket.Build{}
	err = build.ParseMessage(msg)
	return build, err
}

// swarmingRefFromTags resolves the swarming hostname and task ID from a
// set of buildbucket tags.
func swarmingRefFromTags(tags strpair.Map) (*swarming.BuildID, error) {
	var host, task string
	if host = tags.Get("swarming_hostname"); host == "" {
		return nil, errors.New("no swarming hostname tag")
	}
	if task = tags.Get("swarming_task_id"); task == "" {
		return nil, errors.New("no swarming task id")
	}
	return &swarming.BuildID{Host: host, TaskID: task}, nil
}

// GetSwarmingID returns the swarming task ID of a buildbucket build.
func GetSwarmingID(c context.Context, buildAddress string) (*swarming.BuildID, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	// Fetch the Swarming task ID from Buildbucket.
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	build, err := fetchBuild(c, client, buildAddress)
	if err != nil {
		return nil, err
	}
	return swarmingRefFromTags(build.Tags)
}

// getRespBuild fetches the full build state from Swarming and LogDog if
// available, otherwise returns an empty "pending build".
func getRespBuild(c context.Context, build *buildbucket.Build) (*resp.MiloBuild, error) {
	// TODO(nodir,hinoka): squash getRespBuild with toMiloBuild.

	if build.Status == buildbucket.StatusScheduled {
		// Hasn't started yet, so definitely no build ready yet, return a pending
		// build.
		return &resp.MiloBuild{
			Summary: resp.BuildComponent{Status: model.NotRun},
		}, nil
	}

	// TODO(nodir,hinoka,iannucci): use annotations directly without fetching swarming task
	sID, err := swarmingRefFromTags(build.Tags)
	if err != nil {
		return nil, err
	}
	return sID.Get(c)
}

// Get returns a resp.MiloBuild based off of the buildbucket ID given by
// finding the coorisponding swarming build.
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	// Fetch the Swarming task ID from Buildbucket.
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	build, err := fetchBuild(c, client, b.Address)
	if err != nil {
		return nil, err
	}
	result, err := getRespBuild(c, build)
	if err != nil {
		return nil, err
	}

	if result.Trigger.Project != b.Project {
		return nil, errors.New("invalid project", common.CodeParameterError)
	}
	return result, nil
}

func (b *BuildID) GetLog(c context.Context, logname string) (text string, closed bool, err error) {
	return "", false, errors.New("buildbucket builds do not implement GetLog.")
}
