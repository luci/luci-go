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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/nthsection"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"

	// Add support for datastore transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

const (
	taskClass = "test-failure-bisection"
	queue     = "test-failure-bisection"
)

var taskClassRef = tq.RegisterTaskClass(tq.TaskClass{
	ID:        taskClass,
	Prototype: (*tpb.TestFailureBisectionTask)(nil),
	Queue:     queue,
	Kind:      tq.Transactional,
})

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass(srv *server.Server, luciAnalysisProjectFunc func(luciProject string) string) error {
	ctx := srv.Context
	client, err := lucianalysis.NewClient(ctx, srv.Options.CloudProject, luciAnalysisProjectFunc)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		client.Close()
	})
	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*tpb.TestFailureBisectionTask)
		analysisID := task.GetAnalysisId()
		ctx = loggingutil.SetAnalysisID(ctx, analysisID)
		logging.Infof(ctx, "Processing test failure bisection task with id = %d", analysisID)
		err := Run(ctx, analysisID, client)
		if err != nil {
			err = errors.Fmt("run bisection: %w", err)
			logging.Errorf(ctx, err.Error())
			// Return nil so the task will not be retried.
			// We intentionally disable retrying because bisector does not support retrying at the moment.
			return nil
		}
		return nil
	}
	taskClassRef.AttachHandler(handler)
	return nil
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
func Run(ctx context.Context, analysisID int64, luciAnalysis analysis.AnalysisClient) (reterr error) {
	// Retrieves analysis from datastore.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Fmt("get test failure analysis: %w", err)
	}

	defer func() {
		if reterr != nil {
			// If there is an error, mark the analysis as failing with error.
			err := testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			if err != nil {
				// Just log the error if there is something wrong.
				err = errors.Fmt("update status: %w", err)
				logging.Errorf(ctx, err.Error())
			}
		}
	}()

	// Checks if test failure analysis is enabled.
	enabled, err := IsEnabled(ctx, tfa.Project)
	if err != nil {
		return errors.Fmt("is enabled: %w", err)
	}
	if !enabled {
		logging.Infof(ctx, "Bisection is not enabled")
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_DISABLED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Fmt("update status disabled: %w", err)
		}
		return nil
	}

	if tfa.Project != "chromium" {
		// We don't support other projects for now, so mark the analysis as unsupported.
		logging.Infof(ctx, "Unsupported project: %s", tfa.Project)
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_UNSUPPORTED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Fmt("update status unsupported: %w", err)
		}
		return
	}

	// Update the analysis status.
	err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
	if err != nil {
		return errors.Fmt("update status: %w", err)
	}

	// Run nthsection analysis
	return nthsection.Analyze(ctx, tfa, luciAnalysis)
}

func IsEnabled(ctx context.Context, project string) (bool, error) {
	cfg, err := config.Project(ctx, project)
	if err != nil {
		return false, err
	}
	return cfg.TestAnalysisConfig.GetBisectorEnabled(), nil
}
