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
	pb "go.chromium.org/luci/bisection/proto"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/datastoreutil"
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
				err := errors.Annotate(err, "cancelAnalysis id=%d", task.GetAnalysisId()).Err()
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
	logging.Infof(c, "Cancel analysis %d", analysisID)

	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, analysisID)
	if err != nil {
		return errors.Annotate(err, "couldn't get analysis %d", analysisID).Err()
	}
	reruns, err := datastoreutil.GetRerunsForAnalysis(c, cfa)
	if err != nil {
		return errors.Annotate(err, "couldn't get reruns for analysis %d", analysisID).Err()
	}

	var errs []error
	for _, rerun := range reruns {
		if rerun.Status == pb.RerunStatus_RERUN_STATUS_IN_PROGRESS {
			bbid := rerun.RerunBuild.IntID()
			_, err := buildbucket.CancelBuild(c, bbid, "analysis was canceled")
			if err != nil {
				errs = append(errs, errors.Annotate(err, "couldn't cancel build %d", bbid).Err())
			} else {
				err = updateCancelStatusForRerun(c, rerun)
				if err != nil {
					errs = append(errs, errors.Annotate(err, "couldn't update rerun status %d", rerun.RerunBuild.IntID()).Err())
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
		return errors.Annotate(err, "couldn't update status for analysis %d", cfa.Id).Err()
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
		return errors.Annotate(err, "couldn't update status for nthsection for analysis %d", cfa.Id).Err()
	}

	return nil
}

func updateCancelStatusForRerun(c context.Context, rerun *model.SingleRerun) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		// Update rerun
		e := datastore.Get(c, rerun)
		if e != nil {
			return e
		}
		rerun.EndTime = clock.Now(c)
		rerun.Status = pb.RerunStatus_RERUN_STATUS_CANCELED

		e = datastore.Put(c, rerun)
		if e != nil {
			return e
		}

		// Update rerun build model
		rerunBuild := &model.CompileRerunBuild{
			Id: rerun.RerunBuild.IntID(),
		}
		e = datastore.Get(c, rerunBuild)
		if e != nil {
			return e
		}
		rerunBuild.EndTime = clock.Now(c)
		rerunBuild.Status = bbpb.Status_CANCELED
		return datastore.Put(c, rerunBuild)
	}, nil)
}
