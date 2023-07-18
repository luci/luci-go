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

// Package testfailuredetection analyses recent test failures with
// the changepoint analysis from LUCI analysis, and select test failures to bisect.
package testfailuredetection

import (
	"context"

	tpb "go.chromium.org/luci/bisection/task/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"
)

const (
	taskClass = "test-failure-detection"
	queue     = "test-failure-detection"
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.TestFailureDetectionTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(c context.Context, payload proto.Message) error {
			task := payload.(*tpb.TestFailureDetectionTask)
			logging.Infof(c, "Processing test failure detection task with project = %s", task.Project)
			return nil
		},
	})
}

// Schedule enqueues a task to find test failures to bisect.
func Schedule(ctx context.Context, task *tpb.TestFailureDetectionTask) error {
	return tq.AddTask(ctx, &tq.Task{Payload: task})
}
