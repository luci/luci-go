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

package heuristic

import (
	"context"
	"fmt"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/compilelog"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

func Analyze(
	c context.Context,
	cfa *model.CompileFailureAnalysis,
	rr *pb.RegressionRange,
	compileLogs *model.CompileLogs) (*model.CompileHeuristicAnalysis, error) {
	// Create a new HeuristicAnalysis Entity
	heuristicAnalysis := &model.CompileHeuristicAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         pb.AnalysisStatus_RUNNING,
		RunStatus:      pb.AnalysisRunStatus_STARTED,
	}

	if err := datastore.Put(c, heuristicAnalysis); err != nil {
		return nil, err
	}

	// Get changelogs for heuristic analysis
	changelogs, err := changelogutil.GetChangeLogs(c, rr, false)
	if err != nil {
		setStatusError(c, heuristicAnalysis)
		return nil, fmt.Errorf("failed getting changelogs %w", err)
	}
	logging.Infof(c, "Changelogs has %d logs", len(changelogs))

	// Gets compile logs from logdog, if it is not passed in
	// We need this to get the failure signals
	if compileLogs == nil {
		compileLogs, err = compilelog.GetCompileLogs(c, cfa.FirstFailedBuildId)
		if err != nil {
			setStatusError(c, heuristicAnalysis)
			return nil, fmt.Errorf("failed getting compile log: %w", err)
		}
	}
	logging.Infof(c, "Compile log: %v", compileLogs)
	signal, err := ExtractSignals(c, compileLogs)
	if err != nil {
		setStatusError(c, heuristicAnalysis)
		return nil, fmt.Errorf("error extracting signals %w", err)
	}
	signal.CalculateDependencyMap(c)

	// Update CompileFailure with failed files from signal
	err = updateFailedFiles(c, heuristicAnalysis, signal)
	if err != nil {
		setStatusError(c, heuristicAnalysis)
		logging.Errorf(c, "error in updateFailedFiles: %s", err)
		return nil, err
	}

	analysisResult, err := AnalyzeChangeLogs(c, signal, changelogs)
	if err != nil {
		setStatusError(c, heuristicAnalysis)
		return nil, fmt.Errorf("error in justifying changelogs %w", err)
	}

	for _, item := range analysisResult.Items {
		logging.Infof(c, "Commit %s (%s), with review URL %s, has score of %d", item.Commit, item.ReviewTitle, item.ReviewUrl, item.Justification.GetScore())
	}

	// Updates heuristic analysis
	if len(analysisResult.Items) > 0 {
		heuristicAnalysis.Status = pb.AnalysisStatus_SUSPECTFOUND
		err = saveResultsToDatastore(c, heuristicAnalysis, analysisResult, rr.LastPassed.Host, rr.LastPassed.Project, rr.LastPassed.Ref)
		if err != nil {
			return nil, fmt.Errorf("Failed to store result in datastore: %w", err)
		}
	} else {
		heuristicAnalysis.Status = pb.AnalysisStatus_NOTFOUND
	}

	heuristicAnalysis.RunStatus = pb.AnalysisRunStatus_ENDED
	heuristicAnalysis.EndTime = clock.Now(c)
	if err := datastore.Put(c, heuristicAnalysis); err != nil {
		return nil, fmt.Errorf("Failed to update heuristic analysis: %w", err)
	}

	return heuristicAnalysis, nil
}

func updateFailedFiles(c context.Context, heuristicAnalysis *model.CompileHeuristicAnalysis, signal *model.CompileFailureSignal) error {
	cfModel, err := datastoreutil.GetCompileFailureForAnalysisID(c, heuristicAnalysis.ParentAnalysis.IntID())
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(signal.Files))
	for k := range signal.Files {
		keys = append(keys, k)
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		e := datastore.Get(c, cfModel)
		if e != nil {
			return e
		}
		cfModel.FailedFiles = keys
		return datastore.Put(c, cfModel)
	}, nil)
}

func saveResultsToDatastore(c context.Context, analysis *model.CompileHeuristicAnalysis, result *model.HeuristicAnalysisResult, gitilesHost string, gitilesProject string, gitilesRef string) error {
	suspects := make([]*model.Suspect, len(result.Items))
	for i, item := range result.Items {
		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
			ReviewUrl:      item.ReviewUrl,
			ReviewTitle:    item.ReviewTitle,
			Score:          item.Justification.GetScore(),
			Justification:  item.Justification.GetReasons(),
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    gitilesHost,
				Project: gitilesProject,
				Ref:     gitilesRef,
				Id:      item.Commit,
			},
			VerificationStatus: model.SuspectVerificationStatus_Unverified,
			Type:               model.SuspectType_Heuristic,
			AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspects[i] = suspect
	}
	return datastore.Put(c, suspects)
}

// GetConfidenceLevel returns a description of how likely a suspect to be the
// real culprit.
func GetConfidenceLevel(score int) pb.SuspectConfidenceLevel {
	switch {
	// score >= 10 means at least the suspect touched a file in the failure log
	case score >= 10:
		return pb.SuspectConfidenceLevel_HIGH
	case score >= 5:
		return pb.SuspectConfidenceLevel_MEDIUM
	default:
		return pb.SuspectConfidenceLevel_LOW
	}
}

func setStatusError(c context.Context, heuristicAnalysis *model.CompileHeuristicAnalysis) {
	// if cannot set status error, just log the error here, because the calls
	// are made from an error block
	// We don't need a transaction here because heuristic analysis is run in a single thread
	// and there will not be a race condition
	heuristicAnalysis.Status = pb.AnalysisStatus_ERROR
	heuristicAnalysis.EndTime = clock.Now(c)
	heuristicAnalysis.RunStatus = pb.AnalysisRunStatus_ENDED
	err := datastore.Put(c, heuristicAnalysis)
	if err != nil {
		err = errors.Annotate(err, "couldn't setStatusError for heuristic analysis %d", heuristicAnalysis.ParentAnalysis.IntID()).Err()
		logging.Errorf(c, err.Error())
	}
}
