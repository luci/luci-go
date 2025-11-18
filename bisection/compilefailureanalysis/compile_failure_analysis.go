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
// It has 2 main components: genai analysis and nth_section analysis
package compilefailureanalysis

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/compilelog"
	"go.chromium.org/luci/bisection/compilefailureanalysis/genai"
	"go.chromium.org/luci/bisection/compilefailureanalysis/nthsection"
	"go.chromium.org/luci/bisection/compilefailureanalysis/statusupdater"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/lucinotify"
	"go.chromium.org/luci/bisection/llm"
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
	genaiClient llm.Client,
) (*model.CompileFailureAnalysis, error) {
	logging.Infof(c, "AnalyzeFailure firstFailed = %d", firstFailedBuildID)
	firstFailedBuild, e := buildbucket.GetBuild(c, firstFailedBuildID, &bbpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"id", "builder", "input", "status", "steps"},
		},
	})
	if e != nil {
		return nil, fmt.Errorf("error getting build %d: %w", firstFailedBuildID, e)
	}
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
	stepName, err := buildbucket.GetFailedStepName(firstFailedBuild)
	if err != nil {
		// Just log the error and continue
		logging.Errorf(c, "could not get failed step for build %d: %v", firstFailedBuildID, err)
	} else if stepName != "" {
		logging.Infof(c, "checking tree closer status for build %d with failed step: %s", firstFailedBuildID, stepName)
		err = setTreeCloser(c, analysis, stepName)
		if err != nil {
			// Non-critical, just continue
			err := errors.Fmt("failed to check tree closer: %w", err)
			logging.Errorf(c, err.Error())
		}
	} else {
		// Log when stepName is empty - this indicates no failed step was found
		logging.Warningf(c, "no failed step found for build %d, skipping tree closer check", firstFailedBuildID)
	}

	// LLM analysis
	// TODO: b/440598819 calling LUCI ResultAI server prod/dev endpoints
	genaiAnalysisResult, err := genai.Analyze(c, genaiClient, analysis, regressionRange, compileLogs)
	if err != nil {
		logging.Errorf(c, "LLM analysis failed: %v", err)
	} else {
		shouldRunCulpritVerification, err := culpritverification.ShouldRunCulpritVerification(c, analysis)
		if err != nil {
			return nil, errors.Fmt("couldn't fetch config for culprit verification. Build %d: %w", firstFailedBuildID, err)
		}
		if shouldRunCulpritVerification {
			if !analysis.ShouldCancel {
				if err := verifyGenAIResult(c, genaiAnalysisResult, firstFailedBuildID, analysis.Id); err != nil {
					logging.Errorf(c, "Error verifying genai result for build %d: %s", firstFailedBuildID, err)
				}
			}
		}
	}

	// Nth-section analysis
	shouldRunNthSection, err := nthsection.ShouldRunNthSectionAnalysis(c, analysis)
	if err != nil {
		return nil, errors.Fmt("couldn't fetch config for nthsection. Build %d: %w", firstFailedBuildID, err)
	}
	if shouldRunNthSection {
		_, e = nthsection.Analyze(c, analysis)
		if e != nil {
			e = errors.Fmt("error during nthsection analysis for build %d: %w", firstFailedBuildID, e)
			logging.Errorf(c, e.Error())
		}
	}

	// Update status of analysis
	err = statusupdater.UpdateAnalysisStatus(c, analysis)
	if err != nil {
		return nil, errors.Fmt("couldn't update analysis status. Build %d: %w", firstFailedBuildID, err)
	}

	return analysis, nil
}

// verifyGenAIResult verifies if the suspect from GenAI analysis is the real culprit.
func verifyGenAIResult(c context.Context, genaiAnalysis *model.CompileGenAIAnalysis, failedBuildID int64, analysisID int64) error {
	suspects := []*model.Suspect{}
	q := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(c, genaiAnalysis))
	err := datastore.GetAllWithLimit(c, q, &suspects, 1)
	if err != nil {
		return err
	}
	if len(suspects) == 0 {
		logging.Infof(c, "No suspects found for GenAI analysis %d", genaiAnalysis.Id)
		return nil
	}
	err = culpritverification.VerifySuspect(c, suspects[0], failedBuildID, analysisID)
	if err != nil {
		logging.Errorf(c, "Error in verifying GenAI suspect %d for analysis %d", suspects[0].Id, analysisID)
	}
	return nil
}

// findRegressionRange takes in the first failed build and last passed buildID
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
		return nil, fmt.Errorf("couldn't get gitiles commit for builds (%d, %d)", lastPassedBuildID, firstFailedBuild.Id)
	}

	return &pb.RegressionRange{
		FirstFailed: firstFailedBuild.GetInput().GetGitilesCommit(),
		LastPassed:  lastPassedBuild.GetInput().GetGitilesCommit(),
	}, nil
}

// setTreeCloser checks and updates the analysis if it is for a treecloser failure.
func setTreeCloser(c context.Context, cfa *model.CompileFailureAnalysis, stepName string) error {
	fb, err := datastoreutil.GetBuild(c, cfa.CompileFailure.Parent().IntID())
	if err != nil {
		return errors.Fmt("getBuild: %w", err)
	}
	if fb == nil {
		return fmt.Errorf("couldn't find build for analysis %d", cfa.Id)
	}

	logging.Infof(c, "calling CheckTreeCloser for project=%s, bucket=%s, builder=%s, step=%s",
		fb.Project, fb.Bucket, fb.Builder, stepName)
	isTreeCloser, err := lucinotify.CheckTreeCloser(c, fb.Project, fb.Bucket, fb.Builder, stepName)
	if err != nil {
		return err
	}
	logging.Infof(c, "CheckTreeCloser result: isTreeCloser=%v for step=%s", isTreeCloser, stepName)

	return datastore.RunInTransaction(c, func(c context.Context) error {
		e := datastore.Get(c, cfa)
		if e != nil {
			return e
		}
		cfa.IsTreeCloser = isTreeCloser
		return datastore.Put(c, cfa)
	}, nil)
}
