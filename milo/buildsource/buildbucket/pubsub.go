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
	"go.chromium.org/luci/buildbucket"
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
func generateSummary(c context.Context, hostname string, build buildbucket.Build) *model.BuildSummary {
	bs := &model.BuildSummary{}
	bs.BuildKey = MakeBuildKey(c, hostname, build.ID)
	bs.BuilderID = fmt.Sprintf("buildbucket/%s/%s", build.Bucket, build.Builder)

	// TODO(hinoka,iannucci) - make this link point to the /p/$project/build/b$buildId
	// endpoint.
	if sid := build.Tags.Get("swarming_task_id"); sid != "" {
		// HACK: this is an ugly cross-buildsource import. Should go away with
		// a proper link though, as in the above TODO.
		bs.SelfLink = fmt.Sprintf("%s/%s", swarming.URLBase, sid)
	}
	bs.Created = build.CreationTime
	bs.Summary.Start = build.StartTime
	bs.Summary.End = build.CompletionTime

	var ok bool
	bs.Summary.Status, ok = map[buildbucket.Status]model.Status{
		buildbucket.StatusScheduled: model.NotRun,
		buildbucket.StatusStarted:   model.Running,
		buildbucket.StatusSuccess:   model.Success,
		buildbucket.StatusFailure:   model.Failure,
		buildbucket.StatusTimeout:   model.Expired,

		// TODO(hinoka,nodir,iannucci): This isn't exactly true.
		buildbucket.StatusCancelled: model.Expired,
	}[build.Status]
	if !ok {
		bs.Summary.Status = model.InfraFailure
	}

	// HACK(iannucci,nodir) - The logdog annotation stream URL shouldn't be
	// extracted from the swarming tags!!! This is a leaky abstraction!
	bs.AnnotationURL = build.Tags.Get("swarming_tag:log_location")

	bs.Version = build.UpdateTime.UnixNano()

	return bs
}

// attachRevisionData attaches the pseudo-manifest REVISION data to this
// BuildSummary given the pubsub event data.
//
// This mutates `bs`'s Manifests field.
func attachRevisionData(c context.Context, project string, build buildbucket.Build, bs *model.BuildSummary) error {
	// TODO(iannucci,nodir): support manifests/got_revision

	var consoles []*common.Console

	for _, bset := range build.BuildSets {
		bs.BuildSet = append(bs.BuildSet, bset.String())

		switch x := bset.(type) {
		case *buildbucket.GitilesCommit:
			revision, err := hex.DecodeString(x.Revision)
			if err != nil {
				logging.WithError(err).Warningf(c, "failed to decode revision: %v", x.Revision)
			} else if len(revision) != sha1.Size {
				logging.Warningf(c, "wrong revision size %d v %d: %v", len(revision), sha1.Size, x.Revision)
			} else {
				if consoles == nil {
					if consoles, err = common.GetAllConsoles(c, bs.BuilderID); err != nil {
						return errors.Annotate(err, "failed to GetAllConsoles").Tag(transient.Tag).Err()
					}
				}
				// HACK(iannucci): Until we have real manifest support, console definitions
				// will specify their manifest as "REVISION", and we'll do lookups with null
				// URL fields.
				for _, con := range consoles {
					bs.AddManifestKey(project, con.ID, "REVISION", "", revision)

					url := fmt.Sprintf("https://%s/%s", x.Host, x.Project)
					bs.AddManifestKey(project, con.ID, "BUILD_SET/GitilesCommit", url, revision)
				}
			}

		case *buildbucket.GerritChange:
			// this is a known type, but we don't index it specially.

		default:
			logging.Warningf(c, "unknown buildset: %v - %s", bset, bset.String())
		}
	}

	return nil
}

// pubSubHandlerImpl takes the http.Request, expects to find
// a common.PubSubSubscription JSON object in the Body, containing a bbPSEvent,
// and handles the contents with generateSummary and attachRevisionData.
func pubSubHandlerImpl(c context.Context, r *http.Request) error {
	// This is the default action. The code below will modify the values of some
	// or all of these parameters.
	isLUCI, bucket, status, action := false, "UNKNOWN", "UNKNOWN", "Rejected"

	defer func() {
		// closure for late binding
		buildCounter.Add(c, 1, bucket, isLUCI, status, action)
	}()

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

	event := struct {
		Build    bucketApi.ApiCommonBuildMessage `json:"build"`
		Hostname string                          `json:"hostname"`
	}{}
	if err := json.Unmarshal(bData, &event); err != nil {
		return errors.Annotate(err, "could not parse pubsub message data").Err()
	}

	build := buildbucket.Build{}
	if err := build.ParseMessage(&event.Build); err != nil {
		return errors.Annotate(err, "could not parse buildbucket.Build").Err()
	}

	// TODO(iannucci,nodir): ew.
	project := build.Tags.Get("swarming_tag:luci_project")
	isLUCI = strings.HasPrefix(project, "luci.")
	bucket = build.Bucket
	status = build.Status.String()

	logging.Debugf(c, "Received from %s: build %s/%s (%s)\n%v",
		event.Hostname, build.Bucket, build.Builder,
		build.Status.String(), build)

	if !isLUCI || build.Builder == "" {
		logging.Infof(c, "This is not an ingestable build, ignoring")
		return nil
	}

	bs := generateSummary(c, event.Hostname, build)

	if err := attachRevisionData(c, project, build, bs); err != nil {
		return err
	}

	err = datastore.RunInTransaction(c, func(c context.Context) error {
		curBS := &model.BuildSummary{BuildKey: bs.BuildKey}
		switch err := datastore.Get(c, curBS); err {
		case datastore.ErrNoSuchEntity:
			action = "Created"
		case nil:
			action = "Modified"
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
