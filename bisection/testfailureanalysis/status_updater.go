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

// Package testfailureanalysis handles test failure analysis.
package testfailureanalysis

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// UpdateAnalysisStatus updates status of a test failure analysis.
func UpdateAnalysisStatus(ctx context.Context, tfa *model.TestFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, tfa)
		if e != nil {
			return e
		}

		// If the run has ended or canceled, we don't want to do anything.
		if tfa.RunStatus == pb.AnalysisRunStatus_ENDED || tfa.RunStatus == pb.AnalysisRunStatus_CANCELED {
			return nil
		}

		// All the same, no need to update.
		if tfa.RunStatus == runStatus && tfa.Status == status {
			return nil
		}

		if runStatus == pb.AnalysisRunStatus_ENDED || runStatus == pb.AnalysisRunStatus_CANCELED {
			tfa.EndTime = clock.Now(ctx)
		}
		// Do not update start time again if it has started.
		if runStatus == pb.AnalysisRunStatus_STARTED && tfa.RunStatus != pb.AnalysisRunStatus_STARTED {
			tfa.StartTime = clock.Now(ctx)
		}

		tfa.Status = status
		tfa.RunStatus = runStatus
		return datastore.Put(ctx, tfa)
	}, nil)
}

// UpdateNthSectionAnalysisStatus updates status of a test failure analysis.
func UpdateNthSectionAnalysisStatus(ctx context.Context, nsa *model.TestNthSectionAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, nsa)
		if e != nil {
			return e
		}

		// If the run has ended or canceled, we don't want to do anything.
		if nsa.RunStatus == pb.AnalysisRunStatus_ENDED || nsa.RunStatus == pb.AnalysisRunStatus_CANCELED {
			return nil
		}

		// All the same, no need to update.
		if nsa.RunStatus == runStatus && nsa.Status == status {
			return nil
		}

		nsa.Status = status
		nsa.RunStatus = runStatus
		if runStatus == pb.AnalysisRunStatus_ENDED || runStatus == pb.AnalysisRunStatus_CANCELED {
			nsa.EndTime = clock.Now(ctx)
		}
		return datastore.Put(ctx, nsa)
	}, nil)
}

// UpdateAnalysisStatusWhenError updates analysis and nthsection analysis
// when an error occured.
// As there still maybe reruns in progress, we should check if there are still
// running reruns.
func UpdateAnalysisStatusWhenError(ctx context.Context, tfa *model.TestFailureAnalysis) error {
	reruns, err := datastoreutil.GetInProgressReruns(ctx, tfa)
	if err != nil {
		return errors.Fmt("get in progress rerun: %w", err)
	}
	// Analysis still in progress, we don't need to do anything.
	if len(reruns) > 0 {
		return nil
	}
	// Otherwise, update status to error.
	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return errors.Fmt("get test nthsection for analysis: %w", err)
	}
	if nsa == nil {
		return errors.New("no nthsection analysis")
	}
	// This will be a no-op if the nthsection analysis already ended or canceled.
	err = UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
	if err != nil {
		return errors.Fmt("update nthsection analysis status: %w", err)
	}

	// This will be a no-op if the analysis already ended or canceled.
	err = UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
	if err != nil {
		return errors.Fmt("update analysis status: %w", err)
	}
	return nil
}
