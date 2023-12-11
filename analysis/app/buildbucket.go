// Copyright 2023 The LUCI Authors.
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
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/analysis/internal/services/buildjoiner"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
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
)

// BuildbucketPubSubHandler accepts and process buildbucket v2 Pub/Sub messages.
// LUCI Analysis ingests buildbucket builds upon completion, with the
// caveat that for builds related to CV runs, we also wait for the
// CV run to complete (via CV Pub/Sub).
func BuildbucketPubSubHandler(ctx *router.Context) {
	project := "unknown"
	status := "unknown"
	defer func() {
		// Closure for late binding.
		buildCounter.Add(ctx.Request.Context(), 1, project, status)
	}()

	project, processed, err := bbPubSubHandlerImpl(ctx.Request.Context(), ctx.Request)
	if err != nil {
		errors.Log(ctx.Request.Context(), errors.Annotate(err, "handling buildbucket pubsub event").Err())
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
	var psMsg pubsubMessage
	if err := json.NewDecoder(request.Body).Decode(&psMsg); err != nil {
		return "unknown", false, errors.Annotate(err, "could not decode buildbucket pubsub message").Err()
	}
	// Handle message from the builds (v2) topic.
	msg, err := parseBBV2Message(ctx, psMsg)
	if err != nil {
		return "unknown", false, errors.Annotate(err, "unmarshal buildbucket v2 pub/sub message").Err()
	}
	processed, err = processBBV2Message(ctx, msg)
	if err != nil {
		return msg.Build.Builder.Project, false, errors.Annotate(err, "process buildbucket v2 build").Err()
	}
	return msg.Build.Builder.Project, processed, nil
}

func parseBBV2Message(ctx context.Context, pbMsg pubsubMessage) (*buildbucketpb.BuildsV2PubSub, error) {
	buildsV2Msg := &buildbucketpb.BuildsV2PubSub{}
	opts := protojson.UnmarshalOptions{AllowPartial: true, DiscardUnknown: true}
	if err := opts.Unmarshal(pbMsg.Message.Data, buildsV2Msg); err != nil {
		return nil, err
	}
	// Optional: implement decompression of large fields. As we don't need build.input,
	// build.output or build.steps here, we omit it.
	// See https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/luci_notify/notify/pubsub.go;l=442;drc=2ed735a67ecfe6a824076d231a4c7268b84e8e95;bpv=0
	// for an example.
	return buildsV2Msg, nil
}

func processBBV2Message(ctx context.Context, message *buildbucketpb.BuildsV2PubSub) (processed bool, err error) {
	if message.Build.Status&buildbucketpb.Status_ENDED_MASK != buildbucketpb.Status_ENDED_MASK {
		// Received build that hasn't completed yet, ignore it.
		return false, nil
	}
	if message.Build.Infra.GetBuildbucket().GetHostname() == "" {
		// Invalid build. Ignore.
		logging.Warningf(ctx, "Build %v did not specify buildbucket hostname, ignoring.", message.Build.Id)
		return false, nil
	}

	project := message.Build.Builder.Project
	task := &taskspb.JoinBuild{
		Project: project,
		Id:      message.Build.Id,
		Host:    message.Build.Infra.Buildbucket.Hostname,
	}
	err = buildjoiner.Schedule(ctx, task)
	if err != nil {
		return false, err
	}
	return true, nil
}
