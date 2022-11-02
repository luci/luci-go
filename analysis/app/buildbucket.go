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

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/router"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	ctlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// userAgentTagKey is the key of the user agent tag.
	userAgentTagKey = "user_agent"
	// userAgentCQ is the value of the user agent tag, for builds started
	// by LUCI CV.
	userAgentCQ = "cq"
)

var (
	buildCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/buildbucket_builds",
		"The number of buildbucket builds received by LUCI Analysis from PubSub.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "ignored", "transient-failure" or "permanent-failure".
		field.String("status"))

	buildProcessingOutcomeCounter = metric.NewCounter(
		"analysis/ingestion/pubsub/buildbucket_build_processing_outcome",
		"The number of buildbucket builds processed by LUCI Analysis,"+
			" by processing outcome (e.g. success, permission denied).",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "permission_denied".
		field.String("status"))
)

// BuildbucketPubSubHandler accepts and process buildbucket Pub/Sub messages.
// LUCI Analysis ingests buildbucket builds upon completion, with the
// caveat that for builds related to CV runs, we also wait for the
// CV run to complete (via CV Pub/Sub).
func BuildbucketPubSubHandler(ctx *router.Context) {
	project := "unknown"
	status := "unknown"
	defer func() {
		// Closure for late binding.
		buildCounter.Add(ctx.Context, 1, project, status)
	}()

	project, processed, err := bbPubSubHandlerImpl(ctx.Context, ctx.Request)
	if err != nil {
		errors.Log(ctx.Context, errors.Annotate(err, "handling buildbucket pubsub event").Err())
		status = processErr(ctx, err)
		return
	}
	if processed {
		status = "success"
		// Use subtly different "success" response codes to surface in
		// standard GAE logs whether an ingestion was ignored or not,
		// while still acknowledging the pub/sub.
		// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
		ctx.Writer.WriteHeader(http.StatusOK)
	} else {
		status = "ignored"
		ctx.Writer.WriteHeader(http.StatusNoContent) // 204
	}
}

func bbPubSubHandlerImpl(ctx context.Context, request *http.Request) (project string, processed bool, err error) {
	msg, err := parseBBMessage(ctx, request)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "failed to parse buildbucket pub/sub message").Err()
	}
	processed, err = processBBMessage(ctx, msg)
	if err != nil {
		return msg.Build.Project, false, errors.Annotate(err, "processing build").Err()
	}
	return msg.Build.Project, processed, nil
}

type buildBucketMessage struct {
	Build    bbv1.LegacyApiCommonBuildMessage
	Hostname string
}

func parseBBMessage(ctx context.Context, r *http.Request) (*buildBucketMessage, error) {
	var psMsg pubsubMessage
	if err := json.NewDecoder(r.Body).Decode(&psMsg); err != nil {
		return nil, errors.Annotate(err, "could not decode buildbucket pubsub message").Err()
	}

	var bbMsg buildBucketMessage
	if err := json.Unmarshal(psMsg.Message.Data, &bbMsg); err != nil {
		return nil, errors.Annotate(err, "could not parse buildbucket pubsub message data").Err()
	}
	return &bbMsg, nil
}

func processBBMessage(ctx context.Context, message *buildBucketMessage) (processed bool, err error) {
	if message.Build.Status != bbv1.StatusCompleted {
		// Received build that hasn't completed yet, ignore it.
		return false, nil
	}
	if message.Build.CreatedTs == 0 {
		return false, errors.New("build did not have created timestamp specified")
	}

	userAgents := extractTagValues(message.Build.Tags, userAgentTagKey)
	isPresubmit := len(userAgents) == 1 && userAgents[0] == userAgentCQ

	project := message.Build.Project
	result := &ctlpb.BuildResult{
		CreationTime: timestamppb.New(bbv1.ParseTimestamp(message.Build.CreatedTs)),
		Id:           message.Build.Id,
		Host:         message.Hostname,
		Project:      project,
	}

	id := control.BuildID(message.Hostname, message.Build.Id)

	buildReadMask := &field_mask.FieldMask{
		Paths: []string{"infra.resultdb"},
	}
	build, err := retrieveBuild(ctx, message.Hostname, message.Build.Id, buildReadMask)
	code := status.Code(err)
	if code == codes.NotFound {
		// Build not found, handle gracefully.
		logging.Warningf(ctx, "Buildbucket build %s/%d for project %s not found (or LUCI Analysis does not have access to read it).",
			message.Hostname, message.Build.Id, project)
		buildProcessingOutcomeCounter.Add(ctx, 1, "permission_denied")
		return false, nil
	}
	if err != nil {
		return false, transient.Tag.Apply(errors.Annotate(err, "retrieving buildbucket build").Err())
	}

	invocation := build.GetInfra().GetResultdb()
	if invocation != nil {
		wantInvocation := pbutil.InvocationName(fmt.Sprintf("build-%v", message.Build.Id))
		if invocation.Invocation != wantInvocation {
			logging.Warningf(ctx, "Build %v has unexpected ResultDB invocation (got %v, want %v)", id, invocation.Invocation, wantInvocation)
		}
	}

	hasInvocation := invocation != nil

	if err := JoinBuildResult(ctx, id, project, isPresubmit, hasInvocation, result); err != nil {
		return false, errors.Annotate(err, "joining build result").Err()
	}
	buildProcessingOutcomeCounter.Add(ctx, 1, "success")
	return true, nil
}

func extractTagValues(tags []string, key string) []string {
	var values []string
	for _, tag := range tags {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) < 2 {
			// Invalid tag.
			continue
		}
		if tagParts[0] == key {
			values = append(values, tagParts[1])
		}
	}
	return values
}

func retrieveBuild(ctx context.Context, bbHost string, id int64, readMask *field_mask.FieldMask) (*bbpb.Build, error) {
	bc, err := buildbucket.NewClient(ctx, bbHost)
	if err != nil {
		return nil, err
	}
	request := &bbpb.GetBuildRequest{
		Id: id,
		Mask: &bbpb.BuildMask{
			Fields: readMask,
		},
	}
	b, err := bc.GetBuild(ctx, request)
	switch {
	case err != nil:
		return nil, err
	}
	return b, nil
}
