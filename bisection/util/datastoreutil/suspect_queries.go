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

	"go.chromium.org/luci/bisection/model"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// CountLatestRevertsCreated returns the number of reverts created within
// the last number of hours
func CountLatestRevertsCreated(c context.Context, hours int64) (int64, error) {
	cutoffTime := clock.Now(c).Add(-time.Hour * time.Duration(hours))
	q := datastore.NewQuery("Suspect").
		Gt("revert_create_time", cutoffTime).
		Eq("is_revert_created", true)

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

	// Get failure analysis that the heuristic/nth section analysis relates to
	analysisKey := suspect.ParentAnalysis.Parent()
	if analysisKey == nil {
		return 0, fmt.Errorf("suspect with ID '%d' had no parent failure analysis",
			suspect.Id)
	}
	analysisID := analysisKey.IntID()

	compileFailure, err := GetCompileFailureForAnalysisID(ctx, analysisID)
	if err != nil {
		return 0, fmt.Errorf("analysis with ID '%d' did not have a compile failure",
			analysisID)
	}

	if compileFailure.Build == nil {
		return 0, fmt.Errorf("compile failure with ID '%d' did not have a failed build",
			compileFailure.Id)
	}

	return compileFailure.Build.IntID(), nil
}

// GetSuspectsForAnalysis returns all suspects (from heuristic and nthsection) for an analysis
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
