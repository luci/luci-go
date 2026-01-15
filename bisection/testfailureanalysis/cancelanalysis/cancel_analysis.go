// Copyright 2026 The LUCI Authors.
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

// Package cancelanalysis handles cancellation of existing test failure analyses.
package cancelanalysis

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

const (
	taskClass = "cancel-test-analysis"
	queue     = "cancel-test-analysis"
)

// ScheduleTask schedules a task to cancel a test failure analysis.
func ScheduleTask(ctx context.Context, analysisID int64) error {
	return tq.AddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("cancel_test_analysis_%d", analysisID),
		Payload: &tpb.CancelTestAnalysisTask{
			AnalysisId: analysisID,
		},
	})
}

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.CancelTestAnalysisTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(c context.Context, payload proto.Message) error {
			task := payload.(*tpb.CancelTestAnalysisTask)
			logging.Infof(c, "Process CancelTestAnalysisTask with id = %d", task.GetAnalysisId())
			err := CancelAnalysis(c, task.GetAnalysisId())
			if err != nil {
				err := errors.Fmt("cancelTestAnalysis id=%d: %w", task.GetAnalysisId(), err)
				logging.Errorf(c, err.Error())
				// Tag non-transient errors as Fatal so TQ won't retry them
				if !transient.Tag.In(err) {
					err = tq.Fatal.Apply(err)
				}
				return err
			}
			return nil
		},
	})
}

// CancelAnalysis cancels all pending and running reruns for a test failure analysis.
func CancelAnalysis(c context.Context, analysisID int64) error {
	c = loggingutil.SetAnalysisID(c, analysisID)
	logging.Infof(c, "Cancel test analysis %d", analysisID)

	tfa, err := datastoreutil.GetTestFailureAnalysis(c, analysisID)
	if err != nil {
		return errors.Fmt("couldn't get analysis %d: %w", analysisID, err)
	}

	reruns, err := datastoreutil.GetInProgressReruns(c, tfa)
	if err != nil {
		return errors.Fmt("couldn't get reruns for analysis %d: %w", analysisID, err)
	}

	var errs []error
	for _, rerun := range reruns {
		bbid := rerun.LUCIBuild.BuildID
		_, err := buildbucket.CancelBuild(c, bbid, "analysis was canceled due to confirmed culprit")
		if err != nil {
			errs = append(errs, errors.Fmt("couldn't cancel build %d: %w", bbid, err))
		} else {
			err = updateCancelStatusForRerun(c, rerun)
			if err != nil {
				errs = append(errs, errors.Fmt("couldn't update rerun status %d: %w", rerun.LUCIBuild.BuildID, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}

	// Update status of analysis and nthsection analysis
	// Set status to FOUND if there's a verified culprit, otherwise keep current status
	// Update the analysis to an end runStatus when cancelling it
	analysisStatus := tfa.Status
	if tfa.VerifiedCulpritKey != nil {
		analysisStatus = pb.AnalysisStatus_FOUND
	}
	err = testfailureanalysis.UpdateAnalysisStatus(c, tfa, analysisStatus, pb.AnalysisRunStatus_CANCELED)

	if err != nil {
		return errors.Fmt("couldn't update status for analysis %d: %w", tfa.ID, err)
	}

	// Update status of nthsection analysis if it exists
	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(c, tfa)
	if err != nil {
		return errors.Fmt("couldn't get nth-section analysis: %w", err)
	}
	if nsa == nil {
		return nil
	}
	// If nthsection was running when canceled, mark as NOTFOUND since it didn't complete
	// Otherwise keep its current status
	nthStatus := nsa.Status
	if nsa.Status == pb.AnalysisStatus_RUNNING {
		nthStatus = pb.AnalysisStatus_NOTFOUND
	}

	err = testfailureanalysis.UpdateNthSectionAnalysisStatus(c, nsa, nthStatus, pb.AnalysisRunStatus_CANCELED)
	if err != nil {
		return errors.Fmt("couldn't update status for nthsection for analysis %d: %w", tfa.ID, err)
	}

	return nil
}

func updateCancelStatusForRerun(c context.Context, rerun *model.TestSingleRerun) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		// Update rerun
		err := datastore.Get(c, rerun)
		if err != nil {
			return err
		}
		now := clock.Now(c)
		rerun.EndTime = now
		rerun.Status = pb.RerunStatus_RERUN_STATUS_CANCELED

		// Update embedded LUCI build fields
		rerun.LUCIBuild.EndTime = now
		rerun.LUCIBuild.Status = bbpb.Status_CANCELED

		err = datastore.Put(c, rerun)
		if err != nil {
			return err
		}

		// Also if the rerun is for culprit verification, set the status of the suspect
		if rerun.CulpritKey != nil {
			suspect := &model.Suspect{
				Id:             rerun.CulpritKey.IntID(),
				ParentAnalysis: rerun.CulpritKey.Parent(),
			}
			err = datastore.Get(c, suspect)
			if err != nil {
				return errors.Fmt("couldn't get suspect for rerun: %w", err)
			}

			suspect.VerificationStatus = model.SuspectVerificationStatus_Canceled
			err = datastore.Put(c, suspect)

			if err != nil {
				return errors.Fmt("couldn't update suspect status: %w", err)
			}
		}
		return nil
	}, nil)
}
