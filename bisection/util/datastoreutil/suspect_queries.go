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

	compileFailure, err := GetCompileFailureForAnalysis(ctx, analysisID)
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
