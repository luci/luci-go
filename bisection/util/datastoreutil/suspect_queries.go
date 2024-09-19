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

package datastoreutil

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/tracing"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
)

// CountLatestRevertsCreated returns the number of reverts created within
// the last number of hours
func CountLatestRevertsCreated(c context.Context, hours int64, analysisType pb.AnalysisType) (int64, error) {
	cutoffTime := clock.Now(c).Add(-time.Hour * time.Duration(hours))
	q := datastore.NewQuery("Suspect").
		Gt("revert_create_time", cutoffTime).
		Eq("is_revert_created", true).
		Eq("analysis_type", analysisType)

	count, err := datastore.Count(c, q)
	if err != nil {
		err = errors.Annotate(err, "failed counting latest reverts created").Err()
		return 0, err
	}

	return count, nil
}

// CountLatestRevertsCommitted returns the number of reverts committed within
// the last number of hours
func CountLatestRevertsCommitted(c context.Context, hours int64) (int64, error) {
	cutoffTime := clock.Now(c).Add(-time.Hour * time.Duration(hours))
	q := datastore.NewQuery("Suspect").
		Gt("revert_commit_time", cutoffTime).
		Eq("is_revert_committed", true)

	count, err := datastore.Count(c, q)
	if err != nil {
		err = errors.Annotate(err, "failed counting latest reverts committed").Err()
		return 0, err
	}

	return count, nil
}

// GetSuspect returns the Suspect given its ID and key to parent analysis
func GetSuspect(ctx context.Context, suspectID int64,
	parentAnalysis *datastore.Key) (*model.Suspect, error) {
	suspect := &model.Suspect{
		Id:             suspectID,
		ParentAnalysis: parentAnalysis,
	}

	err := datastore.Get(ctx, suspect)
	if err != nil {
		return nil, err
	}

	return suspect, nil
}

// GetAssociatedBuildID returns the build ID of the failure associated with the suspect
func GetAssociatedBuildID(ctx context.Context, suspect *model.Suspect) (int64, error) {
	// Get parent analysis - either heuristic or nth section
	if suspect.ParentAnalysis == nil {
		return 0, fmt.Errorf("suspect with ID '%d' had no parent analysis",
			suspect.Id)
	}
	switch suspect.AnalysisType {
	case pb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		buildID, err := GetBuildIDForCompileSuspect(ctx, suspect)
		if err != nil {
			return 0, errors.Annotate(err, "get build id for suspect").Err()
		}
		return buildID, nil
	case pb.AnalysisType_TEST_FAILURE_ANALYSIS:
		tfa, err := GetTestFailureAnalysisForSuspect(ctx, suspect)
		if err != nil {
			return 0, errors.Annotate(err, "fetch test failure analysis for suspect").Err()
		}
		return tfa.FailedBuildID, nil
	}
	return 0, fmt.Errorf("unknown analysis type of suspect %s", suspect.AnalysisType.String())
}

func GetBuildIDForCompileSuspect(ctx context.Context, suspect *model.Suspect) (int64, error) {
	if suspect.AnalysisType != pb.AnalysisType_COMPILE_FAILURE_ANALYSIS {
		return 0, errors.Reason("Invalid suspect type %v", suspect.AnalysisType).Err()
	}
	// Get failure analysis that the heuristic/nth section analysis relates to
	analysisKey := suspect.ParentAnalysis.Parent()
	if analysisKey == nil {
		return 0, fmt.Errorf("suspect with ID '%d' had no parent failure analysis",
			suspect.Id)
	}
	analysisID := analysisKey.IntID()

	compileFailure, err := GetCompileFailureForAnalysisID(ctx, analysisID)
	if err != nil {
		return 0, errors.Annotate(err, "get compile failure for analysis id").Err()
	}
	if compileFailure.Build == nil {
		return 0, fmt.Errorf("compile failure with ID '%d' did not have a failed build",
			compileFailure.Id)
	}
	return compileFailure.Build.IntID(), nil

}

// FetchSuspectsForAnalysis returns all suspects (from heuristic and nthsection) for an analysis
func FetchSuspectsForAnalysis(c context.Context, cfa *model.CompileFailureAnalysis) ([]*model.Suspect, error) {
	suspects := []*model.Suspect{}
	ha, err := GetHeuristicAnalysis(c, cfa)
	if err != nil {
		return nil, errors.Annotate(err, "getHeuristicAnalysis").Err()
	}
	if ha != nil {
		haSuspects, err := fetchSuspectsForParentKey(c, datastore.KeyForObj(c, ha))
		if err != nil {
			return nil, errors.Annotate(err, "fetchSuspects heuristic analysis").Err()
		}
		suspects = append(suspects, haSuspects...)
	}

	nsa, err := GetNthSectionAnalysis(c, cfa)
	if err != nil {
		return nil, errors.Annotate(err, "getNthSectionAnalysis").Err()
	}
	if nsa != nil {
		haSuspects, err := fetchSuspectsForParentKey(c, datastore.KeyForObj(c, nsa))
		if err != nil {
			return nil, errors.Annotate(err, "fetchSuspects nthsection analysis").Err()
		}
		suspects = append(suspects, haSuspects...)
	}
	return suspects, nil
}

// GetSuspectForTestAnalysis returns suspect for test analysis.
func GetSuspectForTestAnalysis(ctx context.Context, tfa *model.TestFailureAnalysis) (*model.Suspect, error) {
	nsa, err := GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get test nthsection for analysis").Err()
	}
	if nsa == nil {
		return nil, nil
	}
	suspects, err := fetchSuspectsForParentKey(ctx, datastore.KeyForObj(ctx, nsa))
	if err != nil {
		return nil, errors.Annotate(err, "fetch suspects for parent key").Err()
	}
	if len(suspects) == 0 {
		return nil, nil
	}
	if len(suspects) > 1 {
		return nil, errors.Reason("more than 1 suspect found: %d", len(suspects)).Err()
	}
	return suspects[0], nil
}

func fetchSuspectsForParentKey(c context.Context, parentKey *datastore.Key) ([]*model.Suspect, error) {
	suspects := []*model.Suspect{}
	q := datastore.NewQuery("Suspect").Ancestor(parentKey)
	err := datastore.GetAll(c, q, &suspects)
	if err != nil {
		return nil, err
	}
	return suspects, nil
}

func FetchTestFailuresForSuspect(c context.Context, suspect *model.Suspect) (*model.TestFailureBundle, error) {
	nsa, err := getTestNthSectionAnalysisForSuspect(c, suspect)
	if err != nil {
		return nil, err
	}
	return getTestFailureBundleWithAnalysisKey(c, nsa.ParentAnalysisKey)
}

func GetTestFailureAnalysisForSuspect(c context.Context, suspect *model.Suspect) (*model.TestFailureAnalysis, error) {
	nsa, err := getTestNthSectionAnalysisForSuspect(c, suspect)
	if err != nil {
		return nil, err
	}
	tfa := &model.TestFailureAnalysis{ID: nsa.ParentAnalysisKey.IntID()}
	if err := datastore.Get(c, tfa); err != nil {
		return nil, err
	}
	return tfa, nil
}

func getTestNthSectionAnalysisForSuspect(c context.Context, suspect *model.Suspect) (*model.TestNthSectionAnalysis, error) {
	if suspect.AnalysisType != pb.AnalysisType_TEST_FAILURE_ANALYSIS {
		return nil, errors.New("invalid suspect analysis type")
	}
	nsa := &model.TestNthSectionAnalysis{ID: suspect.ParentAnalysis.IntID()}
	if err := datastore.Get(c, nsa); err != nil {
		return nil, errors.Annotate(err, "get test nthsection analysis").Err()
	}
	return nsa, nil
}

func GetVerifiedCulpritForTestAnalysis(ctx context.Context, tfa *model.TestFailureAnalysis) (s *model.Suspect, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/util/datastoreutil/suspect_queries.GetVerifiedCulpritForTestAnalysis")
	defer func() { tracing.End(ts, err) }()

	culpritKey := tfa.VerifiedCulpritKey
	if culpritKey == nil {
		return nil, nil
	}
	suspect, err := GetSuspect(ctx, culpritKey.IntID(), culpritKey.Parent())
	if err != nil {
		return nil, errors.Annotate(err, "get suspect").Err()
	}
	return suspect, nil
}

func GetProjectForSuspect(ctx context.Context, suspect *model.Suspect) (string, error) {
	switch suspect.AnalysisType {
	case pb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		return GetProjectForCompileFailureAnalysisID(ctx, suspect.ParentAnalysis.Parent().IntID())
	case pb.AnalysisType_TEST_FAILURE_ANALYSIS:
		tfa, err := GetTestFailureAnalysisForSuspect(ctx, suspect)
		if err != nil {
			return "", errors.Annotate(err, "get test failure analysis for suspect").Err()
		}
		return tfa.Project, nil
	}
	return "", fmt.Errorf("unknown analysis type of suspect %s", suspect.AnalysisType.String())
}
