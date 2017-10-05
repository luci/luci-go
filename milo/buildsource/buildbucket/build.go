package buildbucket

import (
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/net/context"

	bucketApi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
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
func getBucketBuild(client *bucketApi.Service, id int64) (
	*bucketApi.ApiCommonBuildMessage, error) {
	response, err := client.Get(id).Do()
	if err != nil {
		return nil, errors.Annotate(err, "fetching build %d", id).Err()
	}

	if response.Error != nil {
		if response.Error.Reason == "BUILD_NOT_FOUND" {
			return nil, errors.Annotate(
				err, "message: %s", response.Error.Message).Tag(common.CodeNotFound).Err()
		}
		return nil, fmt.Errorf(
			"reason: %s, message: %s", response.Error.Reason, response.Error.Message)
	}

	switch response.HTTPStatusCode {
	case http.StatusForbidden:
		return nil, errors.New("access forbidden", common.CodeNoAccess)
	case http.StatusNotFound:
		// TODO(hinoka): It is unclear under what conditions err == nil and
		// response.Error is also nil, but we still have an error here, but we still
		// want to prepare for that possibility.  This might not actually be
		// needed however.
		return nil, errors.New("build not found", common.CodeNotFound)
	}
	return response.Build, nil
}

// swarmingRefFromTags resolves the swarming hostname and task ID from a
// set of buildbucket tags.
func swarmingRefFromTags(tags []string) (*swarming.BuildID, error) {
	parsedTags := ParseTags(tags)
	var ok bool
	var host, task string
	if host, ok = parsedTags["swarming_hostname"]; !ok {
		return nil, errors.New("no swarming hostname tag")
	}
	if task, ok = parsedTags["swarming_task_id"]; !ok {
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
	build, err := getBucketBuild(client, id)
	if err != nil {
		return nil, err
	}
	return swarmingRefFromTags(build.Tags)
}

// getRespBuild fetches the full build state from Swarming and LogDog if
// available, otherwise returns an empty "pending build".
func getRespBuild(
	c context.Context, build *bucketApi.ApiCommonBuildMessage) (*resp.MiloBuild, error) {

	// Hasn't started yet, so definitely no build ready yet, return a pending
	// build.
	if build.Status == "SCHEDULED" {
		b := &resp.MiloBuild{
			Summary: resp.BuildComponent{
				Status: model.NotRun,
			},
		}
		return b, nil
	}

	sID, err := swarmingRefFromTags(build.Tags)
	if err != nil {
		return nil, err
	}

	swarmingSvc, err := swarming.NewProdService(c, sID.Host)
	if err != nil {
		return nil, err
	}
	bl := swarming.BuildLoader{}
	return bl.SwarmingBuildImpl(c, swarmingSvc, sID.TaskID)
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
	build, err := getBucketBuild(client, id)
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
	sID, err := GetSwarmingID(c, id)
	if err != nil {
		return "", false, err
	}
	// Defer implementation over to swarming's BuildID.GetLog
	return sID.GetLog(c, logname)
}
