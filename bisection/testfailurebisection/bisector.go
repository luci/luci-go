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

// Package testfailurebisection performs bisection for test failures.
package testfailurebisection

import (
	"context"

	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"
)

const (
	taskClass = "test-failure-bisection"
	queue     = "test-failure-bisection"
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.TestFailureBisectionTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*tpb.TestFailureBisectionTask)
			analysisID := task.GetAnalysisId()
			loggingutil.SetAnalysisID(ctx, analysisID)
			logging.Infof(ctx, "Processing test failure bisection task with id = %d", analysisID)
			err := Run(ctx, analysisID)
			if err != nil {
				err = errors.Annotate(err, "run bisection").Err()
				logging.Errorf(ctx, err.Error())
				// If the error is transient, return err to retry.
				if transient.Tag.In(err) {
					return err
				}
				return nil
			}
			return nil
		},
	})
}

// Run runs bisection for the given analysisID.
func Run(ctx context.Context, analysisID int64) error {
	// Retrieves analysis from datastore.
	return nil
}
