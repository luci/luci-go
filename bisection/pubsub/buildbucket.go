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
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailuredetection"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/metrics"
	"go.chromium.org/luci/bisection/rerun"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/loggingutil"
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
		"The number of rerun build result, by project, status and type.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the rerun build.
		// The possible values are "SUCCESS", "FAILURE", "INFRA_FAILURE", "CANCELED".
		field.String("status"),
		// The type of the analysis that rerun belongs to.
		// The possible values are "compile", "test".
		field.String("type"),
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

func BuildbucketPubSubHandler(c context.Context, msg pubsub.Message, bbmsg *buildbucketpb.BuildsV2PubSub) error {
	if v := msg.Attributes["version"]; v != "v2" {
		return pubsub.Ignore.Apply(errors.Reason("got version %q, expected v2", v).Err())
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
	compileBuilder, err := config.GetCompileBuilder(c, project)
	if err != nil {
		// If there are no configs for the project, just ignore.
		if !errors.Is(err, config.ErrNotFoundProjectConfig) {
			return errors.Annotate(err, "get compile builder").Err()
		}
	} else {
		if bucket == compileBuilder.Bucket && builder == compileBuilder.Builder {
			logging.Infof(c, "Received pubsub for luci bisection compile rerun build %d status %s", bbid, buildbucketpb.Status_name[int32(status)])
			bbCounter.Add(c, 1, project, string(OutcomeTypeUpdateRerun))

			// We only update the rerun counter after the build finished.
			// Status_ENDED_MASK is a union of all terminal statuses.
			if status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK {
				rerunCounter.Add(c, 1, project, status.String(), string(metrics.AnalysisTypeCompile))
			}

			if bbmsg.Build.Status != buildbucketpb.Status_SCHEDULED {
				return rerun.UpdateCompileRerunStatus(c, bbid)
			}
			return nil
		}
	}

	// Handle test rerun build.
	testBuilder, err := config.GetTestBuilder(c, project)
	if err != nil {
		// If there are no configs for the project, just ignore.
		if !errors.Is(err, config.ErrNotFoundProjectConfig) {
			return errors.Annotate(err, "get test builder").Err()
		}
	} else {
		if bucket == testBuilder.Bucket && builder == testBuilder.Builder {
			logging.Infof(c, "Test bisection: received pubsub for rerun build %d status %s", bbid, buildbucketpb.Status_name[int32(status)])
			bbCounter.Add(c, 1, project, string(OutcomeTypeUpdateRerun))

			// We only update the rerun counter after the build finished.
			// Status_ENDED_MASK is a union of all terminal statuses.
			if status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK {
				rerunCounter.Add(c, 1, project, status.String(), string(metrics.AnalysisTypeTest))
			}

			if bbmsg.Build.Status != buildbucketpb.Status_SCHEDULED {
				return rerun.UpdateTestRerunStatus(c, bbmsg.GetBuild())
			}
			return nil
		}
	}

	// For now, we only handle chromium/ci builds
	// TODO (nqmtuan): Move this into config
	if !(project == "chromium" && bucket == "ci") {
		logging.Debugf(c, "Unsupported build for bucket (%q, %q). Exiting early...", project, bucket)
		bbCounter.Add(c, 1, project, string(OutcomeTypeUnsupported))
		return nil
	}

	excludedBgs, err := config.GetExcludedBuilderGroupsForCompile(c, project)
	if err != nil {
		return errors.Annotate(err, "get excluded builder groups for compile").Err()
	}
	var largeFields *buildbucketpb.Build
	if bbmsg.BuildLargeFieldsDropped {
		mask := &buildbucketpb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{"input.properties"},
			},
		}
		largeFields, err = buildbucket.GetBuild(c, bbmsg.Build.Id, mask)
		if err != nil {
			return errors.Annotate(err, "fetch large field").Err()
		}
	} else {
		// Pubsub message stores input properties in large fields.
		largeFieldsData, err := zlibDecompress(bbmsg.BuildLargeFields)
		if err != nil {
			return errors.Annotate(err, "decompress large field").Err()
		}
		largeFields = &buildbucketpb.Build{}
		if err = proto.Unmarshal(largeFieldsData, largeFields); err != nil {
			return errors.Annotate(err, "unmarshal large field").Err()
		}
	}

	builderGroup := util.GetBuilderGroup(largeFields)
	if builderGroup != "" {
		for _, excludedBg := range excludedBgs {
			if builderGroup == excludedBg {
				logging.Debugf(c, "Builder group is excluded %s. Exiting early...", builderGroup)
				bbCounter.Add(c, 1, project, string(OutcomeTypeUnsupported))
				return nil
			}
		}
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
	return nil
}

// zlibDecompress decompresses data using zlib.
func zlibDecompress(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}
