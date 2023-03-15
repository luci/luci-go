// Copyright 2022 The LUCI Authors.
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

// Package pubsub handles pub/sub messages
package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.chromium.org/luci/bisection/compilefailuredetection"
	"go.chromium.org/luci/bisection/rerun"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/router"

	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/server/tq"
)

var (
	bbCounter = metric.NewCounter(
		"bisection/ingestion/buildbucket",
		"The number of Buildbucket pubsub received, by project and outcome.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The outcome action of the ingestion
		// "unsupported", "update_rerun", "update_succeeded_build", "ignore", "analyze"
		field.String("outcome"),
	)
)

// OutcomeType is used for sending metrics to tsmon
type OutcomeType string

const (
	OutcomeTypeUnsupported          OutcomeType = "unsupported"
	OutcomeTypeUpdateRerun          OutcomeType = "update_rerun"
	OutcomeTypeUpdateSucceededBuild OutcomeType = "update_succeeded_build"
	OutcomeTypeIgnore               OutcomeType = "ignore"
	OutcomeTypeAnalyze              OutcomeType = "analyze"
)

type pubsubMessage struct {
	Message struct {
		Data       []byte
		Attributes map[string]any
	}
}

type buildBucketMessage struct {
	Build    bbv1.LegacyApiCommonBuildMessage
	Hostname string
}

// BuildbucketPubSubHandler handles pub/sub messages from buildbucket
func BuildbucketPubSubHandler(ctx *router.Context) {
	if err := buildbucketPubSubHandlerImpl(ctx.Context, ctx.Request); err != nil {
		logging.Errorf(ctx.Context, "Error processing buildbucket pubsub message: %s", err)
		processError(ctx, err)
		return
	}
	// Just returns OK here so pubsub does not resend the message
	ctx.Writer.WriteHeader(http.StatusOK)
}

func processError(ctx *router.Context, err error) {
	if transient.Tag.In(err) {
		// Pubsub will retry this
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	} else {
		// Pubsub will not retry those errors
		ctx.Writer.WriteHeader(http.StatusAccepted)
	}
}

func buildbucketPubSubHandlerImpl(c context.Context, r *http.Request) error {
	var psMsg pubsubMessage
	if err := json.NewDecoder(r.Body).Decode(&psMsg); err != nil {
		return errors.Annotate(err, "could not decode message").Err()
	}

	// Handle the message from `builds_v2` pubsub topic.
	if v, ok := psMsg.Message.Attributes["version"].(string); ok && v == "v2" {
		logging.Debugf(c, "Got message from v2")
		return nil
	} else {
		// Handle v1 message.
		// TODO (nqmtuan): Remove this when we finish migration.
		bbmsg, err := parseBBMessage(psMsg)
		if err != nil {
			return err
		}
		c = loggingutil.SetAnalyzedBBID(c, bbmsg.Build.Id)
		logging.Debugf(c, "Received message for build id %d", bbmsg.Build.Id)

		// Special handling for pubsub message for LUCI Bisection
		if bbmsg.Build.Project == "chromium" && bbmsg.Build.Bucket == "luci.chromium.findit" {
			logging.Infof(c, "Received pubsub for luci bisection build %d", bbmsg.Build.Id)
			bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeUpdateRerun))
			if bbmsg.Build.Status == bbv1.StatusStarted {
				return rerun.UpdateRerunStartTime(c, bbmsg.Build.Id)
			}
			return nil
		}

		// For now, we only handle chromium/ci builds
		// TODO (nqmtuan): Move this into config
		if !(bbmsg.Build.Project == "chromium" && bbmsg.Build.Bucket == "luci.chromium.ci") {
			logging.Debugf(c, "Unsupported build for bucket (%q, %q). Exiting early...", bbmsg.Build.Project, bbmsg.Build.Bucket)
			bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeUnsupported))
			return nil
		}

		// Just ignore non-completed builds
		if bbmsg.Build.Status != bbv1.StatusCompleted {
			logging.Debugf(c, "Build status = %s. Exiting early...", bbmsg.Build.Status)
			bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeIgnore))
			return nil
		}

		// If the build is succeeded -> some running analysis may not be necessary
		if bbmsg.Build.Result == bbv1.ResultSuccess {
			bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeUpdateSucceededBuild))
			err := compilefailuredetection.UpdateSucceededBuild(c, bbmsg.Build.Id)
			if err != nil {
				return errors.Annotate(err, "UpdateSucceededBuild").Err()
			}
			return nil
		}

		// If the build is not succeed, and not failed either
		if bbmsg.Build.Result != bbv1.ResultFailure {
			logging.Debugf(c, "Result = %s. Exiting early...", bbmsg.Build.Result)
			bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeIgnore))
			return nil
		}

		// Create a task for task queue
		err = tq.AddTask(c, &tq.Task{
			Title: fmt.Sprintf("failed_build_%d", bbmsg.Build.Id),
			Payload: &taskpb.FailedBuildIngestionTask{
				Bbid: bbmsg.Build.Id,
			},
		})

		if err != nil {
			logging.Errorf(c, "Failed creating task in task queue for build %d", bbmsg.Build.Id)
			return err
		}
		bbCounter.Add(c, 1, bbmsg.Build.Project, string(OutcomeTypeAnalyze))

		return nil
	}
}

func parseBBMessage(psMsg pubsubMessage) (*buildBucketMessage, error) {
	var bbMsg buildBucketMessage
	if err := json.Unmarshal(psMsg.Message.Data, &bbMsg); err != nil {
		return nil, errors.Annotate(err, "could not parse buildbucket pubsub message data").Err()
	}
	return &bbMsg, nil
}
