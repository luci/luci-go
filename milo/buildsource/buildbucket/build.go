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
	"strconv"

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
// It references a buildbucket build which may reference a swarming build.
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// ID is the Buildbucket's build ID (required)
	ID string
}

// getBucketBuild fetches the buildbucket build given the ID.
func fetchBuild(client *bbapi.Service, id int64) (*buildbucket.Build, error) {
	response, err := client.Get(id).Do()

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

	build := &buildbucket.Build{}
	err = build.ParseMessage(response.Build)
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
func GetSwarmingID(c context.Context, id int64) (*swarming.BuildID, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	// Fetch the Swarming task ID from Buildbucket.
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	build, err := fetchBuild(client, id)
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
	id, err := strconv.ParseInt(b.ID, 10, 64)
	if err != nil {
		return nil, errors.Annotate(
			err, "%s is not a valid number", b.ID).Tag(common.CodeParameterError).Err()
	}
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}
	// Fetch the Swarming task ID from Buildbucket.
	client, err := newBuildbucketClient(c, host)
	if err != nil {
		return nil, err
	}
	build, err := fetchBuild(client, id)
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

func (b *BuildID) GetLog(c context.Context, logname string) (
	text string, closed bool, err error) {

	id, err := strconv.ParseInt(b.ID, 10, 64)
	if err != nil {
		return "", false, errors.Annotate(err, "%s is not a valid number", b.ID).Err()
	}
	// TODO(nodir,hinoka): use annotations directly without fetching swarming task
	sID, err := GetSwarmingID(c, id)
	if err != nil {
		return "", false, err
	}
	// Defer implementation over to swarming's BuildID.GetLog
	return sID.GetLog(c, logname)
}
