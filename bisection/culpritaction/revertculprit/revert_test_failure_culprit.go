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

// Package revertculprit contains the logic to revert culprits
package revertculprit

import (
	"context"

	"go.chromium.org/luci/bisection/internal/lucianalysis"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

type AnalysisClient interface {
	TestIsUnexpectedConsistently(ctx context.Context, project string, key lucianalysis.TestVerdictKey, sinceCommitPosition int64) (bool, error)
}

func processTestFailureCulpritTask(ctx context.Context, analysisID int64, luciAnalysis AnalysisClient) error {
	ctx = loggingutil.SetAnalysisID(ctx, analysisID)
	logging.Infof(ctx, "Processing test failure culprit action")

	// Get culprit model.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Annotate(err, "get test failure analysis").Err()
	}

	culpritKey := tfa.VerifiedCulpritKey
	culpritModel, err := datastoreutil.GetSuspect(ctx, culpritKey.IntID(), culpritKey.Parent())
	if err != nil {
		return errors.Annotate(err, "get suspect").Err()
	}

	// We mark that actions have been taken for the analyses.
	defer func() {
		culpritModel.HasTakenActions = true
		err := datastore.Put(ctx, culpritModel)
		if err != nil {
			// Just log, nothing we can do now.
			logging.Errorf(ctx, "failed to set HasTakenActions: %v", err)
		}
	}()

	// Check if primary test is still failing.
	primaryTestFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get primary test failure").Err()
	}
	key := lucianalysis.TestVerdictKey{
		TestID:      primaryTestFailure.TestID,
		VariantHash: primaryTestFailure.VariantHash,
		RefHash:     primaryTestFailure.RefHash,
	}
	stillFail, err := luciAnalysis.TestIsUnexpectedConsistently(ctx, tfa.Project, key, primaryTestFailure.RegressionEndPosition)
	if err != nil {
		return errors.Annotate(err, "test is unexpected consistently").Err()
	}

	// If the latest verdict is not unexpected anymore, do not perform any action.
	if !stillFail {
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_TEST_NO_LONGER_UNEXPECTED)
		return nil
	}

	if err := TakeCulpritAction(ctx, culpritModel); err != nil {
		return errors.Annotate(err, "revert culprit suspect_id %d", culpritModel.Id).Err()
	}
	return nil
}
