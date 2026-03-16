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

	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/genai/fixforward"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

type AnalysisClient interface {
	TestIsUnexpectedConsistently(ctx context.Context, project string, key lucianalysis.TestVerdictKey, sinceCommitPosition int64) (bool, error)
}

func processTestFailureCulpritTask(ctx context.Context, analysisID int64, luciAnalysis AnalysisClient, genaiClient llm.Client) error {
	ctx = loggingutil.SetAnalysisID(ctx, analysisID)
	logging.Infof(ctx, "Processing test failure culprit action")

	// Get culprit model.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Fmt("get test failure analysis: %w", err)
	}

	culpritKey := tfa.VerifiedCulpritKey
	culpritModel, err := datastoreutil.GetSuspect(ctx, culpritKey.IntID(), culpritKey.Parent())
	if err != nil {
		return errors.Fmt("get suspect: %w", err)
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
		return errors.Fmt("get primary test failure: %w", err)
	}
	key := lucianalysis.TestVerdictKey{
		TestID:      primaryTestFailure.TestID,
		VariantHash: primaryTestFailure.VariantHash,
		RefHash:     primaryTestFailure.RefHash,
	}
	stillFail, err := luciAnalysis.TestIsUnexpectedConsistently(ctx, tfa.Project, key, primaryTestFailure.RegressionEndPosition)
	if err != nil {
		return errors.Fmt("test is unexpected consistently: %w", err)
	}

	// If the latest verdict is not unexpected anymore, do not perform any action.
	if !stillFail {
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_TEST_NO_LONGER_UNEXPECTED)
		return nil
	}

	if err := TakeCulpritAction(ctx, culpritModel); err != nil {
		return errors.Fmt("revert culprit suspect_id %d: %w", culpritModel.Id, err)
	}

	// Also trigger the fixforward generation inline if we have a genai client
	if genaiClient != nil {
		if err := triggerTestFixforward(ctx, genaiClient, tfa, culpritModel); err != nil {
			logging.Errorf(ctx, "Failed to generate fixforward CL for test failure (Analysis ID: %d): %v", analysisID, err)
		}
	}

	return nil
}

func triggerTestFixforward(ctx context.Context, genaiClient llm.Client, tfa *model.TestFailureAnalysis, suspect *model.Suspect) error {
	// 1. Fetch the primaryTestFailure to construct the failureLog
	primaryTestFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Fmt("failed to get primary test failure: %w", err)
	}

	failureLog := fmt.Sprintf("Test Name: %s\n\nError Message:\n%s\n\nStack Trace:\n%s",
		primaryTestFailure.TestID, // Actually, TestID is not exactly name but closest
		"",                        // We don't have the message handy here without fetching results
		"")                        // or stack trace

	// Actually, wait, let's just make a generic failure log if we don't have the details
	// since datastoreutil.GetPrimaryTestFailure doesn't contain the log.
	failureLog = fmt.Sprintf("Test: %s", primaryTestFailure.TestID)

	// 2. Determine repository
	repoUrl := gitiles.GetRepoUrl(ctx, &suspect.GitilesCommit)

	// 3. Create gerrit client
	gerritHost, err := gerrit.GetHost(ctx, suspect.ReviewUrl)
	if err != nil {
		return errors.Fmt("failed to get gerrit host: %w", err)
	}
	gerritClient, err := gerrit.NewClient(ctx, gerritHost)
	if err != nil {
		return errors.Fmt("failed to create gerrit client: %w", err)
	}

	// 4. Generate the fixforward CL
	err = fixforward.GenerateFixforwardCL(ctx, genaiClient, gerritClient, suspect.GitilesCommit.GetId(), failureLog, repoUrl, suspect.RevertURL)
	if err != nil {
		return errors.Fmt("fixforward generation failed: %w", err)
	}

	logging.Infof(ctx, "Successfully triggered test fixforward for test failure %s", primaryTestFailure.TestID)
	return nil
}
