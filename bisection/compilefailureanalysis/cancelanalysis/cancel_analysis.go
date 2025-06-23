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

// Package cancelanalysis handles cancelation of existing analyses.
package cancelanalysis

import (
	"context"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailureanalysis/statusupdater"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

const (
	taskClass = "cancel-analysis"
	queue     = "cancel-analysis"
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*tpb.CancelAnalysisTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(c context.Context, payload proto.Message) error {
			task := payload.(*tpb.CancelAnalysisTask)
			logging.Infof(c, "Process CancelAnalysisTask with id = %d", task.GetAnalysisId())
			err := CancelAnalysis(c, task.GetAnalysisId())
			if err != nil {
				err := errors.Fmt("cancelAnalysis id=%d: %w", task.GetAnalysisId(), err)
				logging.Errorf(c, err.Error())
				// If the error is transient, return err to retry
				if transient.Tag.In(err) {
					return err
				}
				return nil
			}
			return nil
		},
	})
}

// CancelAnalysis cancels all pending and running reruns for an analysis.
func CancelAnalysis(c context.Context, analysisID int64) error {
	c, err := loggingutil.UpdateLoggingWithAnalysisID(c, analysisID)
	if err != nil {
		// not critical, just log
		err := errors.Fmt("failed UpdateLoggingWithAnalysisID %d: %w", analysisID, err)
		logging.Errorf(c, "%v", err)
	}
	logging.Infof(c, "Cancel analysis %d", analysisID)

	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, analysisID)
	if err != nil {
		return errors.Fmt("couldn't get analysis %d: %w", analysisID, err)
	}
	reruns, err := datastoreutil.GetRerunsForAnalysis(c, cfa)
	if err != nil {
		return errors.Fmt("couldn't get reruns for analysis %d: %w", analysisID, err)
	}

	var errs []error
	for _, rerun := range reruns {
		if rerun.Status == pb.RerunStatus_RERUN_STATUS_IN_PROGRESS {
			bbid := rerun.RerunBuild.IntID()
			_, err := buildbucket.CancelBuild(c, bbid, "analysis was canceled")
			if err != nil {
				errs = append(errs, errors.Fmt("couldn't cancel build %d: %w", bbid, err))
			} else {
				err = updateCancelStatusForRerun(c, rerun)
				if err != nil {
					errs = append(errs, errors.Fmt("couldn't update rerun status %d: %w", rerun.RerunBuild.IntID(), err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}

	// Update status of analysis and nthsection analysis
	newStatus := cfa.Status
	// Only updates status if is was running
	if cfa.Status == pb.AnalysisStatus_RUNNING {
		newStatus = pb.AnalysisStatus_NOTFOUND
	}
	err = statusupdater.UpdateStatus(c, cfa, newStatus, pb.AnalysisRunStatus_CANCELED)

	if err != nil {
		return errors.Fmt("couldn't update status for analysis %d: %w", cfa.Id, err)
	}

	// Update status of nthsection analysis
	nsa, err := datastoreutil.GetNthSectionAnalysis(c, cfa)
	if err != nil {
		return err
	}
	if nsa == nil {
		return nil
	}
	newStatus = nsa.Status
	if nsa.Status == pb.AnalysisStatus_RUNNING {
		newStatus = pb.AnalysisStatus_NOTFOUND
	}

	err = statusupdater.UpdateNthSectionStatus(c, nsa, newStatus, pb.AnalysisRunStatus_CANCELED)
	if err != nil {
		return errors.Fmt("couldn't update status for nthsection for analysis %d: %w", cfa.Id, err)
	}

	return nil
}

func updateCancelStatusForRerun(c context.Context, rerun *model.SingleRerun) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		// Update rerun
		err := datastore.Get(c, rerun)
		if err != nil {
			return err
		}
		rerun.EndTime = clock.Now(c)
		rerun.Status = pb.RerunStatus_RERUN_STATUS_CANCELED

		err = datastore.Put(c, rerun)
		if err != nil {
			return err
		}

		// Update rerun build model
		rerunBuild := &model.CompileRerunBuild{
			Id: rerun.RerunBuild.IntID(),
		}
		err = datastore.Get(c, rerunBuild)
		if err != nil {
			return err
		}
		rerunBuild.EndTime = clock.Now(c)
		rerunBuild.Status = bbpb.Status_CANCELED
		err = datastore.Put(c, rerunBuild)
		if err != nil {
			return err
		}

		// Also if the rerun is for culprit verification, set the status of the suspect
		if rerun.Suspect != nil {
			suspect := &model.Suspect{
				Id:             rerun.Suspect.IntID(),
				ParentAnalysis: rerun.Suspect.Parent(),
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
