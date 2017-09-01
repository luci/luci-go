package buildbucket

import (
	"fmt"
	"strconv"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	bucketApi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/api/resp"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
)

// BuildID is the buildbucket notion of a build.  It references a buildbucket
// build which may reference a swarming build.
type BuildID struct {
	// Project is the project which the build ID is supposed to reside in.
	Project string

	// ID is the Buildbucket's build ID (required)
	ID string
}

func getBuild(client *bucketApi.Service, id int64) (
	*bucketApi.ApiCommonBuildMessage, error) {
	response, err := client.Get(id).Do()
	if err != nil {
		return nil, errors.Annotate(err, "fetching build %d", id).Err()
	}
	if response.Error != nil {
		return nil, fmt.Errorf("reason: %s, message: %s", response.Error.Reason, response.Error.Message)
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

// GetSwarmingRef returns the swarming task ID of a buildbucket build.
func GetSwarmingID(c context.Context, id int64) (*swarming.BuildID, error) {
	build, err := getBucketBuild(c, id)
	if err != nil {
		return nil, err
	}
	return swarmingRefFromTags(build.Tags)
}

// getBucketBuild returns a buildbucket common build message given a build ID
// (assumed to be a build ID of the configured buildbucket instance).
// It checks datastore to see if the build exists first, before going to
// the buildbucket service to fetch the build.
func getBucketBuild(c context.Context, id int64) (*bucketApi.ApiCommonBuildMessage, error) {
	host, err := getHost(c)
	if err != nil {
		return nil, err
	}

	// We just want to get the swarming host and TaskID now.
	// Try getting it from datastore first.
	entry := buildEntry{Key: buildEntryKey(host, id)}
	var build *bucketApi.ApiCommonBuildMessage
	err = datastore.Get(c, &entry)
	switch err {
	case nil:
		build, err = entry.getBuild()
		if err != nil {
			// Not a critical error, just log it and fetch it from buildbucket instead.
			logging.WithError(err).Warningf(c, "couldn't load build from cache")
			build = nil
		}
		if build.Status == "SCHEDULED" {
			// This build doesn't contain any useful information, check to see if there
			// are any newer ones.
			build = nil
		}
	case datastore.ErrNoSuchEntity:
		// Not cached? Try to get it from buildbucket then.
	default:
		return nil, errors.Annotate(err, "fetching datastore entry").Err()
	}

	// We couldn't get it previously for some reason? Fetch it from buildbucket.
	// TODO(hinoka): Cache this as a buildEntry.
	if build == nil {
		client, err := newBuildbucketClient(c, host)
		if err != nil {
			return nil, err
		}
		build, err = getBuild(client, id)
		if err != nil {
			return nil, err
		}
	}

	return build, nil
}

// Get returns a resp.MiloBuild based off of the buildbucket ID given by
// finding the coorisponding swarming build.
func (b *BuildID) Get(c context.Context) (*resp.MiloBuild, error) {
	id, err := strconv.ParseInt(b.ID, 10, 64)
	if err != nil {
		return nil, errors.Annotate(err, "%s is not a valid number", b.ID).Err()
	}
	build, err := getBucketBuild(c, id)
	if err != nil {
		return nil, err
	}

	result, err := maybeGetBuild(c, build)
	if err != nil {
		return nil, err
	}
	if result.SourceStamp.Project != b.Project {
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
