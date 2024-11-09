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

// Package compilefailureanalysis is the component for analyzing
// compile failures.
// It has 2 main components: heuristic analysis and nth_section analysis
package compilefailureanalysis

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/compilelog"
	"go.chromium.org/luci/bisection/compilefailureanalysis/heuristic"
	"go.chromium.org/luci/bisection/compilefailureanalysis/nthsection"
	"go.chromium.org/luci/bisection/compilefailureanalysis/statusupdater"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/lucinotify"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

// AnalyzeFailure receives failure information and perform analysis.
// Note that this assumes that the failure is new (i.e. the client of this
// function should make sure this is not a duplicate analysis)
func AnalyzeFailure(
	c context.Context,
	cf *model.CompileFailure,
	firstFailedBuildID int64,
	lastPassedBuildID int64,
) (*model.CompileFailureAnalysis, error) {
	logging.Infof(c, "AnalyzeFailure firstFailed = %d", firstFailedBuildID)
	regressionRange, e := findRegressionRange(c, firstFailedBuildID, lastPassedBuildID)
	if e != nil {
		return nil, e
	}

	logging.Infof(c, "Regression range: %v", regressionRange)

	// Get failed targets
	compileLogs, e := compilelog.GetCompileLogs(c, firstFailedBuildID)
	if e != nil {
		return nil, e
	}
	failedTargets := compilelog.GetFailedTargets(compileLogs)

	e = datastore.RunInTransaction(c, func(c context.Context) error {
		e := datastore.Get(c, cf)
		if e != nil {
			return e
		}
		cf.OutputTargets = failedTargets
		return datastore.Put(c, cf)
	}, nil)

	if e != nil {
		return nil, e
	}

	// Creates a new CompileFailureAnalysis entity in datastore
	analysis := &model.CompileFailureAnalysis{
		CompileFailure:         datastore.KeyForObj(c, cf),
		CreateTime:             clock.Now(c),
		Status:                 pb.AnalysisStatus_RUNNING,
		RunStatus:              pb.AnalysisRunStatus_STARTED,
		FirstFailedBuildId:     firstFailedBuildID,
		LastPassedBuildId:      lastPassedBuildID,
		InitialRegressionRange: regressionRange,
	}

	e = datastore.Put(c, analysis)
	if e != nil {
		return nil, e
	}
	c = loggingutil.SetAnalysisID(c, analysis.Id)

	// Check if the analysis is for tree closer, if yes, set the flag.
	err := setTreeCloser(c, analysis)
	if err != nil {
		// Non-critical, just continue
		err := errors.Annotate(err, "failed to check tree closer").Err()
		logging.Errorf(c, err.Error())
	}

	// Heuristic analysis
	heuristicResult, e := heuristic.Analyze(c, analysis, regressionRange, compileLogs)
	if e != nil {
		// As this is only heuristic analysis, we log the error and continue with nthsection analysis
		logging.Errorf(c, "Error during heuristic analysis for build %d: %v", firstFailedBuildID, e)
	}

	// If heuristic analysis does not return error, we proceed to verify its results (if any)
	if e == nil {
		shouldRunCulpritVerification, err := culpritverification.ShouldRunCulpritVerification(c, analysis)
		if err != nil {
			return nil, errors.Annotate(err, "couldn't fetch config for culprit verification. Build %d", firstFailedBuildID).Err()
		}
		if shouldRunCulpritVerification {
			if !analysis.ShouldCancel {
				if err := verifyHeuristicResults(c, heuristicResult, firstFailedBuildID, analysis.Id); err != nil {
					// Do not return error here, just log
					logging.Errorf(c, "Error verifying heuristic result for build %d: %s", firstFailedBuildID, err)
				}
			}
		}
	}

	// Nth-section analysis
	shouldRunNthSection, err := nthsection.ShouldRunNthSectionAnalysis(c, analysis)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't fetch config for nthsection. Build %d", firstFailedBuildID).Err()
	}
	if shouldRunNthSection {
		_, e = nthsection.Analyze(c, analysis)
		if e != nil {
			e = errors.Annotate(e, "error during nthsection analysis for build %d", firstFailedBuildID).Err()
			logging.Errorf(c, e.Error())
		}
	}

	// Update status of analysis
	err = statusupdater.UpdateAnalysisStatus(c, analysis)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't update analysis status. Build %d", firstFailedBuildID).Err()
	}

	return analysis, nil
}

// verifyHeuristicResults verifies if the suspects of heuristic analysis are the real culprit.
// analysisID is CompileFailureAnalysis ID. It is meant to be propagated all the way to the
// recipe, so we can identify the analysis in buildbucket.
func verifyHeuristicResults(c context.Context, heuristicAnalysis *model.CompileHeuristicAnalysis, failedBuildID int64, analysisID int64) error {
	// TODO (nqmtuan): Move the verification into a task queue
	suspects, err := getHeuristicSuspectsToVerify(c, heuristicAnalysis)
	if err != nil {
		return err
	}
	for _, suspect := range suspects {
		err := culpritverification.VerifySuspect(c, suspect, failedBuildID, analysisID)
		if err != nil {
			// Just log the error and continue for other suspects
			logging.Errorf(c, "Error in verifying suspect %d for analysis %d", suspect.Id, analysisID)
		}
	}
	return nil
}

// In case heuristic analysis returns too many results, we don't want to verify all of them.
// Instead, we want to be selective in what we want to verify.
// For now, we will just take top 3 results of heuristic analysis.
func getHeuristicSuspectsToVerify(c context.Context, heuristicAnalysis *model.CompileHeuristicAnalysis) ([]*model.Suspect, error) {
	// Getting the suspects for heuristic analysis
	suspects := []*model.Suspect{}
	q := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(c, heuristicAnalysis)).Order("-score")
	err := datastore.GetAll(c, q, &suspects)
	if err != nil {
		return nil, err
	}

	// Get top 3 suspects to verify
	nSuspects := 3
	if nSuspects > len(suspects) {
		nSuspects = len(suspects)
	}
	return suspects[:nSuspects], nil
}

// findRegressionRange takes in the first failed and last passed buildID
// and returns the regression range based on GitilesCommit.
func findRegressionRange(
	c context.Context,
	firstFailedBuildID int64,
	lastPassedBuildID int64,
) (*pb.RegressionRange, error) {
	firstFailedBuild, err := buildbucket.GetBuild(c, firstFailedBuildID, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting build %d: %w", firstFailedBuildID, err)
	}

	lastPassedBuild, err := buildbucket.GetBuild(c, lastPassedBuildID, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting build %d: %w", lastPassedBuildID, err)
	}

	if firstFailedBuild.GetInput().GetGitilesCommit() == nil || lastPassedBuild.GetInput().GetGitilesCommit() == nil {
		return nil, fmt.Errorf("couldn't get gitiles commit for builds (%d, %d)", lastPassedBuildID, firstFailedBuildID)
	}

	return &pb.RegressionRange{
		FirstFailed: firstFailedBuild.GetInput().GetGitilesCommit(),
		LastPassed:  lastPassedBuild.GetInput().GetGitilesCommit(),
	}, nil
}

// setTreeCloser checks and updates the analysis if it is for a treecloser failure.
func setTreeCloser(c context.Context, cfa *model.CompileFailureAnalysis) error {
	fb, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
	if err != nil {
		return errors.Annotate(err, "getBuild").Err()
	}
	if fb == nil {
		return fmt.Errorf("couldn't find build for analysis %d", cfa.Id)
	}

	// TODO (nqmtuan): Pass in step name when we support arbitrary
	// step name which may not be "compile"
	isTreeCloser, err := lucinotify.CheckTreeCloser(c, fb.Project, fb.Bucket, fb.Builder, "compile")
	if err != nil {
		return err
	}

	return datastore.RunInTransaction(c, func(c context.Context) error {
		e := datastore.Get(c, cfa)
		if e != nil {
			return e
		}
		cfa.IsTreeCloser = isTreeCloser
		return datastore.Put(c, cfa)
	}, nil)
}
