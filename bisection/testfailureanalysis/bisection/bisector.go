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

// Package bisection performs bisection for test failures.
package bisection

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/internal/config"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/chromium"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"

	// Add support for datastore transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
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
		Kind:      tq.Transactional,
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

// Schedule enqueues a task to perform bisection.
func Schedule(ctx context.Context, analysisID int64) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: &tpb.TestFailureBisectionTask{
			AnalysisId: analysisID,
		},
		Title: fmt.Sprintf("analysisID-%d", analysisID),
	})
}

// Run runs bisection for the given analysisID.
func Run(ctx context.Context, analysisID int64) (reterr error) {
	// Retrieves analysis from datastore.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Annotate(err, "get test failure analysis").Err()
	}

	defer func() {
		if reterr != nil {
			// If there is an error, mark the analysis as failing with error.
			err := testfailureanalysis.UpdateStatus(ctx, tfa, bisectionpb.AnalysisStatus_ERROR, bisectionpb.AnalysisRunStatus_ENDED)
			if err != nil {
				// Just log the error if there is something wrong.
				err = errors.Annotate(err, "update status").Err()
				logging.Errorf(ctx, err.Error())
			}
		}
	}()

	// Checks if test failure analysis is enabled.
	enabled, err := isEnabled(ctx)
	if err != nil {
		return errors.Annotate(err, "is enabled").Err()
	}
	if !enabled {
		logging.Infof(ctx, "Bisection is not enabled")
		err = testfailureanalysis.UpdateStatus(ctx, tfa, bisectionpb.AnalysisStatus_DISABLED, bisectionpb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status disabled").Err()
		}
		return nil
	}

	// Update the analysis status.
	err = testfailureanalysis.UpdateStatus(ctx, tfa, bisectionpb.AnalysisStatus_RUNNING, bisectionpb.AnalysisRunStatus_STARTED)
	if err != nil {
		return errors.Annotate(err, "update status").Err()
	}

	primaryFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get primary test failure").Err()
	}

	// Trigger specific project bisector.
	switch primaryFailure.Project {
	case "chromium":
		return chromium.Run(ctx, tfa)
	default:
		// We don't support other projects for now, so mark the analysis as unsupported.
		logging.Infof(ctx, "Unsupported project: %s", primaryFailure.Project)
		err = testfailureanalysis.UpdateStatus(ctx, tfa, bisectionpb.AnalysisStatus_UNSUPPORTED, bisectionpb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status unsupported").Err()
		}
	}
	return nil
}

func isEnabled(ctx context.Context) (bool, error) {
	cfg, err := config.Get(ctx)
	if err != nil {
		return false, err
	}
	return cfg.TestAnalysisConfig.GetBisectorEnabled(), nil
}
