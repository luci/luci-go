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

// Package task handle task scheduling for culprit verification.
package task

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/tq"

	tpb "go.chromium.org/luci/bisection/task/proto"
)

// CompileFailureTasks describes how to route compile failure culprit verification tasks.
var CompileFailureTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "culprit-verification",
	Prototype: (*tpb.CulpritVerificationTask)(nil),
	Queue:     "culprit-verification",
	Kind:      tq.NonTransactional,
})

// TestFailureTasks describes how to route test failure culprit verification tasks.
var TestFailureTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "test-failure-culprit-verification",
	Prototype: (*tpb.TestFailureCulpritVerificationTask)(nil),
	Queue:     "test-failure-culprit-verification",
	Kind:      tq.NonTransactional,
})

// RegisterTaskClass registers the task class for tq dispatcher
func RegisterTaskClass(compileHandler, testHandler func(ctx context.Context, payload proto.Message) error) {
	if compileHandler != nil {
		CompileFailureTasks.AttachHandler(compileHandler)
	}
	if testHandler != nil {
		TestFailureTasks.AttachHandler(testHandler)
	}
}

// ScheduleTestFailureTask schedules a task for test failure culprit verification.
func ScheduleTestFailureTask(ctx context.Context, analysisID int64) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: &tpb.TestFailureCulpritVerificationTask{
			AnalysisId: analysisID,
		},
		Title: fmt.Sprintf("test_failure_culprit_verification_%d", analysisID),
	})
}
