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

// Package buildjoiner defines a task to join a completed build with
// the other context (presubmit run and/or ResultDB invocation)
// required for ingestion. The work occurs on a task queue instead of
// directly in the pub/sub handler to allow control over the rate of
// outbound requests to other services.
package buildjoiner

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/ingestion/join"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	joinBuildTaskClass = "join-build"
	joinBuildQueue     = "join-build"
)

func RegisterTaskClass() {
	// RegisterTaskClass registers the task class for tq dispatcher.
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        joinBuildTaskClass,
		Prototype: &taskspb.JoinBuild{},
		Queue:     joinBuildQueue,
		Kind:      tq.NonTransactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*taskspb.JoinBuild)
			if _, err := join.JoinBuild(ctx, task.Host, task.Project, task.Id); err != nil {
				return errors.Annotate(err, "join build %s/%v", task.Host, task.Id).Err()
			}
			return nil
		},
	})
}

// Schedule enqueues a task to join the given build.
func Schedule(ctx context.Context, task *taskspb.JoinBuild) error {
	return tq.AddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-%d", task.Project, task.Host, task.Id),
		Payload: task,
	})
}
