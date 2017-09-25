// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	bucketApi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/router"
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
		// Action can be one of 3 options.
		//   * "Created" - This is the first time Milo heard about this build
		//   * "Modified" - Milo updated some information about this build vs. what
		//     it knew before.
		//   * "Rejected" - Milo was unable to accept this build.
		field.String("action"))
)

// bbPSEvent is the representation of a buidlbucket pubsub event.
type bbPSEvent struct {
	Build    bucketApi.ApiCommonBuildMessage
	Hostname string

	// DecodedParametersJson will store the build.ParametersJson object that we
	// care about from the buildbucket message.
	DecodedParametersJSON struct {
		BuilderName     string `json:"builder_name"`
		InputProperties struct {
			Revision string `json:"revision"`
		} `json:"properties"`
	} `json:"-"`

	// Action is the buildCounter "action". pubSubHandlerImpl will use this to
	// record a single entry in the buildCounter.
	Action string `json:"-"`

	Project string
}

// isLUCI is a hack; there's currently a convention that all 'luci' buckets are
// prefixed with the 'luci.' literal keyword.
//
// Currently this is only used for metrics and to avoid processing non-luci
// builds in Milo's pubsub handler.
//
// HACK(nodir,hinoka,iannucci): Clean this up.
func (b *bbPSEvent) isLUCI() bool {
	// All luci buckets are assumed to be prefixed with luci.
	return strings.HasPrefix(b.Build.Bucket, "luci.")
}

// readSwarmingTag loops over the buildbucket tags and finds a `swarming_tag`
// entry, with the matching swarmingTag name, returning the string data to the
// right of the colon.
func (b *bbPSEvent) readSwarmingTag(swarmingTag string) string {
	prefix := fmt.Sprintf("swarming_tag:%s:", swarmingTag)
	for _, t := range b.Build.Tags {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}

func (b *bbPSEvent) swarmingTaskID() string {
	const prefix = "swarming_task_id:"
	for _, t := range b.Build.Tags {
		if strings.HasPrefix(t, prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}

// PubSubHandler is a webhook that stores the builds coming in from pubsub.
func PubSubHandler(ctx *router.Context) {
	err := pubSubHandlerImpl(ctx.Context, ctx.Request)
	if err != nil {
		logging.Errorf(ctx.Context, "error while handling pubsub event")
		errors.Log(ctx.Context, err)
	}
	if transient.Tag.In(err) {
		// Transient errors are 500 so that PubSub retries them.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	// No errors or non-transient errors are 200s so that PubSub does not retry
	// them.
	ctx.Writer.WriteHeader(http.StatusOK)
}

// generateSummary takes a decoded buildbucket event and generates
// a model.BuildSummary from it.
//
// This is the portion of the summarization process which cannot fail (i.e. is
// pure-data).
func generateSummary(c context.Context, event *bbPSEvent) *model.BuildSummary {
	if !event.isLUCI() || event.Project == "" || event.DecodedParametersJSON.BuilderName == "" {
		return nil
	}

	bs := &model.BuildSummary{}
	bs.BuildKey = MakeBuildKey(c, event.Hostname, event.Build.Id)
	bs.BuilderID = fmt.Sprintf("buildbucket/%s/%s", event.Build.Bucket,
		event.DecodedParametersJSON.BuilderName)

	// TODO(hinoka,iannucci) - make this link point to the /p/$project/build/b$buildId
	// endpoint.
	if sid := event.swarmingTaskID(); sid != "" {
		// HACK: this is an ugly cross-buildsource import. Should go away with
		// a proper link though, as in the above TODO.
		bs.SelfLink = fmt.Sprintf("%s/%s", swarming.URLBase, sid)
	}
	bs.Created = parseTimestamp(event.Build.CreatedTs)
	bs.Summary.Start = parseTimestamp(event.Build.StartedTs)
	bs.Summary.End = parseTimestamp(event.Build.CompletedTs)
	switch event.Build.Status {
	case "SCHEDULED":
		bs.Summary.Status = model.NotRun

	case "STARTED":
		bs.Summary.Status = model.Running

	case "COMPLETED":
		switch event.Build.Result {
		case "SUCCESS":
			bs.Summary.Status = model.Success

		case "CANCELED":
			// TODO(hinoka,nodir,iannucci): This isn't exactly true.
			bs.Summary.Status = model.Expired

		case "FAILURE":
			switch event.Build.FailureReason {
			case "BUILD_FAILURE":
				bs.Summary.Status = model.Failure

			default:
				bs.Summary.Status = model.InfraFailure
			}
		}
	}

	// HACK(iannucci,nodir) - The logdog annotation stream URL shouldn't be
	// extracted from the swarming tags!!! This is a leaky abstraction!
	bs.AnnotationURL = event.readSwarmingTag("log_location")

	bs.Version = event.Build.UpdatedTs

	return bs
}

// attachRevisionData attaches the pseudo-manifest REVISION data to this
// BuildSummary given the pubsub event data.
//
// This mutates `bs`'s Manifests field.
func attachRevisionData(c context.Context, event bbPSEvent, bs *model.BuildSummary) error {
	// TODO(iannucci,nodir): support manifests/got_revision
	revisionHex := event.DecodedParametersJSON.InputProperties.Revision
	revision, err := hex.DecodeString(revisionHex)
	if err != nil {
		logging.WithError(err).Warningf(c, "failed to decode revision: %v", revisionHex)
	} else if len(revision) != sha1.Size {
		logging.Warningf(c, "wrong revision size %d v %d: %v", len(revision), sha1.Size, revisionHex)
	} else {
		consoles, err := common.GetAllConsoles(c, bs.BuilderID)
		if err != nil {
			return errors.Annotate(err, "failed to GetAllConsoles").Tag(transient.Tag).Err()
		}
		// HACK(iannucci): Until we have real manifest support, console definitions
		// will specify their manifest as "REVISION", and we'll do lookups with null
		// URL fields.
		for _, con := range consoles {
			bs.AddManifestKey(event.Project, con.ID, "REVISION", "", revision)
		}
	}

	return nil
}

// pubSubHandlerImpl takes the http.Request, expects to find
// a common.PubSubSubscription JSON object in the Body, containing a bbPSEvent,
// and handles the contents with generateSummary and attachRevisionData.
func pubSubHandlerImpl(c context.Context, r *http.Request) error {
	var event bbPSEvent

	// This is the default action. The code below will modify the values of some
	// or all of these parameters.
	event.Build.Bucket = "UNKNOWN"
	event.Build.Status = "UNKNOWN"
	event.Action = "Rejected"

	defer buildCounter.Add(
		c, 1, event.Build.Bucket, event.isLUCI(),
		event.Build.Status, event.Action,
	)

	msg := common.PubSubSubscription{}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		// This might be a transient error, e.g. when the json format changes
		// and Milo isn't updated yet.
		return errors.Annotate(err, "could not decode message").Tag(transient.Tag).Err()
	}
	bData, err := msg.GetData()
	if err != nil {
		return errors.Annotate(err, "could not parse pubsub message string").Err()
	}
	if err := json.Unmarshal(bData, &event); err != nil {
		return errors.Annotate(err, "could not parse pubsub message data").Err()
	}

	// HACK(iannucci,nodir) - The project shouldn't be extracted from the swarming
	// tags!!! This is a leaky abstraction!
	event.Project = event.readSwarmingTag("luci_project")

	err = json.Unmarshal([]byte(event.Build.ParametersJson), &event.DecodedParametersJSON)
	if err != nil {
		return errors.Annotate(err, "could not parse Build.ParametersJson").Err()
	}

	logging.Debugf(c, "Received from %s: build %s/%s (%s)\n%v",
		event.Hostname, event.Build.Bucket, event.DecodedParametersJSON.BuilderName,
		event.Build.Status, event.Build)

	bs := generateSummary(c, &event)
	if bs == nil {
		logging.Infof(c, "This is not an ingestable build, ignoring")
		return nil
	}

	if err := attachRevisionData(c, event, bs); err != nil {
		return err
	}

	err = datastore.RunInTransaction(c, func(c context.Context) error {
		curBS := &model.BuildSummary{BuildKey: bs.BuildKey}
		switch err := datastore.Get(c, curBS); err {
		case datastore.ErrNoSuchEntity:
			event.Action = "Created"
		case nil:
			event.Action = "Modified"
		default:
			return errors.Annotate(err, "reading current BuildSummary").Err()
		}

		if event.Build.UpdatedTs <= curBS.Version {
			// We've already ingested this (or newer) update.
			return nil
		}

		return datastore.Put(c, bs)
	}, nil)

	return transient.Tag.Apply(err)
}

// MakeBuildKey returns a new datastore Key for a buildbucket.Build.
//
// There's currently no model associated with this key, but it's used as
// a parent for a model.BuildSummary.
func MakeBuildKey(c context.Context, host string, buildID int64) *datastore.Key {
	return datastore.MakeKey(c,
		"buildbucket.Build", fmt.Sprintf("%s:%d", host, buildID))
}
