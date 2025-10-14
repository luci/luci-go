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
		err = errors.Fmt("failed counting latest reverts created: %w", err)
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
		err = errors.Fmt("failed counting latest reverts committed: %w", err)
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
	// Get parent analysis - either genai or nth section
	if suspect.ParentAnalysis == nil {
		return 0, fmt.Errorf("suspect with ID '%d' had no parent analysis",
			suspect.Id)
	}
	switch suspect.AnalysisType {
	case pb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		buildID, err := GetBuildIDForCompileSuspect(ctx, suspect)
		if err != nil {
			return 0, errors.Fmt("get build id for suspect: %w", err)
		}
		return buildID, nil
	case pb.AnalysisType_TEST_FAILURE_ANALYSIS:
		tfa, err := GetTestFailureAnalysisForSuspect(ctx, suspect)
		if err != nil {
			return 0, errors.Fmt("fetch test failure analysis for suspect: %w", err)
		}
		return tfa.FailedBuildID, nil
	}
	return 0, fmt.Errorf("unknown analysis type of suspect %s", suspect.AnalysisType.String())
}

func GetBuildIDForCompileSuspect(ctx context.Context, suspect *model.Suspect) (int64, error) {
	if suspect.AnalysisType != pb.AnalysisType_COMPILE_FAILURE_ANALYSIS {
		return 0, errors.Fmt("Invalid suspect type %v", suspect.AnalysisType)
	}
	// Get failure analysis that the genai/nth section analysis relates to
	analysisKey := suspect.ParentAnalysis.Parent()
	if analysisKey == nil {
		return 0, fmt.Errorf("suspect with ID '%d' had no parent failure analysis",
			suspect.Id)
	}
	analysisID := analysisKey.IntID()

	compileFailure, err := GetCompileFailureForAnalysisID(ctx, analysisID)
	if err != nil {
		return 0, errors.Fmt("get compile failure for analysis id: %w", err)
	}
	if compileFailure.Build == nil {
		return 0, fmt.Errorf("compile failure with ID '%d' did not have a failed build",
			compileFailure.Id)
	}
	return compileFailure.Build.IntID(), nil
}

// FetchSuspectsForAnalysis returns all suspects (from genai and nthsection) for an analysis
func FetchSuspectsForAnalysis(c context.Context, cfa *model.CompileFailureAnalysis) ([]*model.Suspect, error) {
	suspects := []*model.Suspect{}
	ga, err := GetGenAIAnalysis(c, cfa)
	if err != nil {
		return nil, errors.Fmt("getGenAIAnalysis: %w", err)
	}
	if ga != nil {
		gaSuspect, err := fetchSuspectsForParentKey(c, datastore.KeyForObj(c, ga))
		if err != nil {
			return nil, errors.Fmt("fetchSuspects genai analysis: %w", err)
		}
		suspects = append(suspects, gaSuspect...)
	}

	nsa, err := GetNthSectionAnalysis(c, cfa)
	if err != nil {
		return nil, errors.Fmt("getNthSectionAnalysis: %w", err)
	}
	if nsa != nil {
		haSuspects, err := fetchSuspectsForParentKey(c, datastore.KeyForObj(c, nsa))
		if err != nil {
			return nil, errors.Fmt("fetchSuspects nthsection analysis: %w", err)
		}
		suspects = append(suspects, haSuspects...)
	}
	return suspects, nil
}

// GetSuspectForTestAnalysis returns suspect for test analysis.
func GetSuspectForTestAnalysis(ctx context.Context, tfa *model.TestFailureAnalysis) (*model.Suspect, error) {
	nsa, err := GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Fmt("get test nthsection for analysis: %w", err)
	}
	if nsa == nil {
		return nil, nil
	}
	suspects, err := fetchSuspectsForParentKey(ctx, datastore.KeyForObj(ctx, nsa))
	if err != nil {
		return nil, errors.Fmt("fetch suspects for parent key: %w", err)
	}
	if len(suspects) == 0 {
		return nil, nil
	}
	if len(suspects) > 1 {
		return nil, errors.Fmt("more than 1 suspect found: %d", len(suspects))
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
		return nil, errors.Fmt("get test nthsection analysis: %w", err)
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
		return nil, errors.Fmt("get suspect: %w", err)
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
			return "", errors.Fmt("get test failure analysis for suspect: %w", err)
		}
		return tfa.Project, nil
	}
	return "", fmt.Errorf("unknown analysis type of suspect %s", suspect.AnalysisType.String())
}
