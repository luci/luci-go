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

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/pubsub"

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

func BuildbucketPubSubHandler(ctx context.Context, message pubsub.Message, bbMessage *buildbucketpb.BuildsV2PubSub) error {
	project := bbMessage.Build.GetBuilder().GetProject()
	status := "unknown"
	defer func() {
		// Closure for late binding.
		buildCounter.Add(ctx, 1, project, status)
	}()

	err := processBBV2Message(ctx, bbMessage)
	status = errStatus(err)
	return err
}

func processBBV2Message(ctx context.Context, message *buildbucketpb.BuildsV2PubSub) error {
	if message.Build.Status&buildbucketpb.Status_ENDED_MASK != buildbucketpb.Status_ENDED_MASK {
		// Received build that hasn't completed yet, ignore it.
		return pubsub.Ignore.Apply(errors.Reason("build did not complete yet, ignoring").Err())
	}
	if message.Build.Infra.GetBuildbucket().GetHostname() == "" {
		// Invalid build. Ignore.
		logging.Warningf(ctx, "Build %v did not specify buildbucket hostname, ignoring.", message.Build.Id)
		return pubsub.Ignore.Apply(errors.Reason("build %v did not specify hostname, ignoring", message.Build.Id).Err())
	}

	project := message.Build.Builder.Project
	task := &taskspb.JoinBuild{
		Project: project,
		Id:      message.Build.Id,
		Host:    message.Build.Infra.Buildbucket.Hostname,
	}
	err := buildjoiner.Schedule(ctx, task)
	if err != nil {
		return err
	}
	return nil
}
