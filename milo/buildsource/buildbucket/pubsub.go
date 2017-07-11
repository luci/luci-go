// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	bucketApi "github.com/luci/luci-go/common/api/buildbucket/buildbucket/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/milo/api/resp"
	"github.com/luci/luci-go/milo/buildsource/swarming"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/milo/common/model"
	"github.com/luci/luci-go/server/router"
)

var (
	buildCounter = metric.NewCounter(
		"luci/milo/buildbucket_pubsub/builds",
		"The number of buildbucket builds received by Milo from PubSub",
		nil,
		field.String("bucket"),
		// True for luci build, False for non-luci (ie buildbot) build.
		field.Bool("luci"),
		// Status can be "COMPLETED", "SCHEDULED", or "STARTED"
		field.String("status"),
		// Action can be one of 3 options.  "New", "Replaced", "Rejected".
		field.String("action"))
)

type psMsg struct {
	Build    bucketApi.ApiCommonBuildMessage
	Hostname string
}

var (
	errNoLogLocation = errors.New("log_location tag not found")
	errNoProject     = errors.New("project tag not found")
)

type parameters struct {
	builderName string `json:"builder_name"`
	properties  string `json:"properties"`
}

func isLUCI(build *bucketApi.ApiCommonBuildMessage) bool {
	// All luci buckets are assumed to be prefixed with luci.
	return strings.HasPrefix(build.Bucket, "luci.")
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	err := pubSubHandlerImpl(ctx.Context, ctx.Request)
	if err != nil {
		logging.WithError(err).Errorf(ctx.Context, "error while updating buildbucket")
	}
	if transient.Tag.In(err) {
		// Transient errors are 500 so that PubSub retries them.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	} else {
		// No errors or non-transient errors are 200s so that PubSub does not retry
		// them.
		ctx.Writer.WriteHeader(http.StatusOK)
	}

}

func maybeGetBuild(
	c context.Context, build *bucketApi.ApiCommonBuildMessage) (*resp.MiloBuild, error) {

	// Hasn't started yet, so definitely no buildinfo ready yet.
	if build.Status == "SCHEDULED" {
		return nil, nil
	}
	tags := ParseTags(build.Tags)
	var host, task string
	var ok bool
	if host, ok = tags["swarming_hostname"]; !ok {
		return nil, errors.New("no swarming hostname tag")
	}
	if task, ok = tags["swarming_task_id"]; !ok {
		return nil, errors.New("no swarming task id")
	}
	swarmingSvc, err := swarming.NewProdService(c, host)
	if err != nil {
		return nil, err
	}
	bl := swarming.BuildLoader{}
	// The linkBase is not necessary for the summary.
	return bl.SwarmingBuildImpl(c, swarmingSvc, "", task)
}

// processBuild queries swarming and logdog for annotation data, then adds or
// updates a buildEntry in datastore.
func processBuild(
	c context.Context, host string, build *bucketApi.ApiCommonBuildMessage) (
	*buildEntry, error) {

	now := clock.Now(c).UTC()
	entry := buildEntry{key: buildEntryKey(host, build.Id)}

	err := datastore.Get(c)
	switch err {
	case datastore.ErrNoSuchEntity:
		logging.Infof(c, "%s does not exist, will create", entry.key)
		entry.created = now
	case nil:
		// continue
	default:
		return nil, err
	}

	// If the build is running, try to get the annotation data.
	respBuild, err := maybeGetBuild(c, build)
	if err != nil {
		return nil, err
	}
	entry.respBuild = respBuild

	entry.modified = now
	entry.buildbucketData, err = json.Marshal(build)
	if err != nil {
		return nil, err
	}

	err = datastore.Put(c, &entry)
	return &entry, err
}

// saveBuildSummary creates or updates a build summary based off a buildbucket
// build entry.
func saveBuildSummary(
	c context.Context, key *datastore.Key, builderName string,
	entry *buildEntry) error {

	build, err := entry.getBuild()
	if err != nil {
		return err
	}
	status, err := parseStatus(build)
	if err != nil {
		return err
	}
	// TODO(hinoka): Console related items.
	bs := model.BuildSummary{
		BuildKey:  key,
		BuilderID: fmt.Sprintf("buildbucket/%s/%s", build.Bucket, builderName),
		Created:   parseTimestamp(build.CreatedTs),
		Summary: model.Summary{
			Status: status,
			Start:  parseTimestamp(build.StartedTs),
		},
	}
	if entry.respBuild != nil {
		// Add info from the respBuild into the build summary if we have the data.
		entry.respBuild.SummarizeTo(&bs)
	}
	logging.Debugf(c, "Created build summary: %#v", bs)
	// Make datastore flakes transient errors
	return transient.Tag.Apply(datastore.Put(c, &bs))
}

func handlePubSubBuild(c context.Context, data *psMsg) error {
	host := data.Hostname
	build := &data.Build
	p := parameters{}
	err := json.Unmarshal([]byte(build.ParametersJson), &p)
	if err != nil {
		logging.WithError(err).Errorf(c, "could not unmarshal build parameters")
		buildCounter.Add(c, 1, build.Bucket, isLUCI(build), build.Status, "Rejected")
		// Permanent error, since this is probably a type of build we do not recognize.
		return err
	}
	logging.Debugf(c, "Received from %s: build %s/%s (%s)\n%s",
		host, build.Bucket, p.builderName, build.Status, build)
	if !isLUCI(build) {
		logging.Infof(c, "This is not a luci build, ignoring")
		buildCounter.Add(c, 1, build.Bucket, isLUCI(build), build.Status, "Rejected")
		return nil
	}

	buildEntry, err := processBuild(c, host, build)
	if err != nil {
		logging.WithError(err).Errorf(c, "failed to update build")
		buildCounter.Add(c, 1, build.Bucket, isLUCI(build), build.Status, "Rejected")
		// Probably a datastore or network flake, make this into a transient error
		return transient.Tag.Apply(err)
	}
	action := "Created"
	if buildEntry.created != buildEntry.modified {
		action = "Modified"
	}
	buildCounter.Add(c, 1, build.Bucket, isLUCI(build), build.Status, action)

	return saveBuildSummary(
		c, datastore.MakeKey(c, "buildEntry", buildEntry.key), p.builderName, buildEntry)
}

// This returns 500 (Internal Server Error) if it encounters a transient error,
// and returns 200 (OK) if everything is OK, or if it encounters a permanent error.
func pubSubHandlerImpl(c context.Context, r *http.Request) error {
	var data psMsg

	msg := common.PubSubSubscription{}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&msg); err != nil {
		logging.WithError(err).Errorf(c, "could not decode message:\n%s", r.Body)
		// This might be a transient error, e.g. when the json format changes
		// and Milo isn't updated yet.
		return transient.Tag.Apply(err)
	}
	bData, err := msg.GetData()
	if err != nil {
		logging.WithError(err).Errorf(c, "could not parse pubsub message string")
		return err
	}
	if err := json.Unmarshal(bData, &data); err != nil {
		logging.WithError(err).Errorf(c, "could not parse pubsub message data")
		return err
	}

	return handlePubSubBuild(c, &data)
}
