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
	"google.golang.org/protobuf/encoding/protojson"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
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
	rerunCounter = metric.NewCounter(
		"bisection/ingestion/rerun",
		"The number of rerun build result, by project and status",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the rerun build.
		// The possible values are "SUCCESS", "FAILURE", "INFRA_FAILURE", "CANCELED".
		field.String("status"),
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

// BuildbucketPubSubHandler handles pub/sub messages from buildbucket
func BuildbucketPubSubHandler(ctx *router.Context) {
	if err := buildbucketPubSubHandlerImpl(ctx.Request.Context(), ctx.Request); err != nil {
		logging.Errorf(ctx.Request.Context(), "Error processing buildbucket pubsub message: %s", err)
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
		bbmsg, err := parseBBV2Message(c, psMsg)
		if err != nil {
			return errors.Annotate(err, "unmarshal buildbucket v2 pub/sub message").Err()
		}

		bbid := bbmsg.GetBuild().GetId()
		project := bbmsg.GetBuild().GetBuilder().GetProject()
		bucket := bbmsg.GetBuild().GetBuilder().GetBucket()
		builder := bbmsg.GetBuild().GetBuilder().GetBuilder()
		status := bbmsg.GetBuild().GetStatus()

		c = loggingutil.SetAnalyzedBBID(c, bbid)
		logging.Debugf(c, "Received message for build id %d", bbid)

		// Special handling for pubsub message for compile failure for
		// LUCI Bisection.
		// This is only triggered for rerun builds.
		if project == "chromium" && bucket == "findit" && builder == "gofindit-culprit-verification" {
			logging.Infof(c, "Received pubsub for luci bisection compile rerun build %d status %s", bbid, buildbucketpb.Status_name[int32(status)])
			bbCounter.Add(c, 1, project, string(OutcomeTypeUpdateRerun))

			// We only update the rerun counter after the build finished.
			// Status_ENDED_MASK is a union of all terminal statuses.
			if status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK {
				rerunCounter.Add(c, 1, project, status.String())
			}

			if bbmsg.Build.Status != buildbucketpb.Status_SCHEDULED {
				return rerun.UpdateCompileRerunStatus(c, bbid)
			}
			return nil
		}

		// Handle test rerun build.
		// Hardcode the builder name for now.
		// TODO (nqmtuan): Move this to config when we support other projects.
		if project == "chromium" && bucket == "findit" && builder == "test-single-revision" {
			logging.Infof(c, "Test bisection: received pubsub for rerun build %d status %s", bbid, buildbucketpb.Status_name[int32(status)])

			// TODO (nqmtuan): Update tsmon counter.
			if bbmsg.Build.Status != buildbucketpb.Status_SCHEDULED {
				return rerun.UpdateTestRerunStatus(c, bbmsg.GetBuild())
			}
			return nil
		}

		// For now, we only handle chromium/ci builds
		// TODO (nqmtuan): Move this into config
		if !(project == "chromium" && bucket == "ci") {
			logging.Debugf(c, "Unsupported build for bucket (%q, %q). Exiting early...", project, bucket)
			bbCounter.Add(c, 1, project, string(OutcomeTypeUnsupported))
			return nil
		}

		// Just ignore non-successful and non-failed builds
		if status != buildbucketpb.Status_SUCCESS && status != buildbucketpb.Status_FAILURE {
			logging.Debugf(c, "Build status = %s. Exiting early...", status)
			bbCounter.Add(c, 1, project, string(OutcomeTypeIgnore))
			return nil
		}

		// If the build is succeeded -> some running analysis may not be necessary
		if bbmsg.Build.Status == buildbucketpb.Status_SUCCESS {
			bbCounter.Add(c, 1, project, string(OutcomeTypeUpdateSucceededBuild))
			err := compilefailuredetection.UpdateSucceededBuild(c, bbid)
			if err != nil {
				return errors.Annotate(err, "UpdateSucceededBuild").Err()
			}
			return nil
		}

		// Create a task for task queue
		err = tq.AddTask(c, &tq.Task{
			Title: fmt.Sprintf("failed_build_%d", bbid),
			Payload: &taskpb.FailedBuildIngestionTask{
				Bbid: bbid,
			},
		})

		if err != nil {
			logging.Errorf(c, "Failed creating task in task queue for build %d", bbid)
			return err
		}
		bbCounter.Add(c, 1, project, string(OutcomeTypeAnalyze))
	}
	return nil
}

func parseBBV2Message(ctx context.Context, pbMsg pubsubMessage) (*buildbucketpb.BuildsV2PubSub, error) {
	buildsV2Msg := &buildbucketpb.BuildsV2PubSub{}
	opts := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	if err := opts.Unmarshal(pbMsg.Message.Data, buildsV2Msg); err != nil {
		return nil, err
	}
	return buildsV2Msg, nil
}
