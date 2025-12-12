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

// Package culpritverification performs culprit verification for test failures.
package culpritverification

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/rerun"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/nthsection"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/projectbisector"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

func processTestFailureTask(ctx context.Context, task *tpb.TestFailureCulpritVerificationTask) error {
	analysisID := task.AnalysisId
	ctx = loggingutil.SetAnalysisID(ctx, analysisID)
	logging.Infof(ctx, "Processing culprit verification for test failure bisection task")
	// TODO(beining@): set analysis status on error.
	// Retrieves analysis from datastore.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Fmt("get test failure analysis: %w", err)
	}

	// Retrieves suspect.
	suspect, err := datastoreutil.GetSuspectForTestAnalysis(ctx, tfa)
	if err != nil {
		return errors.Fmt("couldn't get suspect: %w", err)
	}
	if suspect == nil {
		return errors.New("couldn't get suspect: <unknown reason>")
	}

	return verifyTestFailureSuspect(ctx, tfa, suspect)
}

func verifyTestFailureSuspect(ctx context.Context, tfa *model.TestFailureAnalysis, suspect *model.Suspect) error {
	projectBisector, err := nthsection.GetProjectBisector(ctx, tfa)
	if err != nil {
		return errors.Fmt("get project bisector: %w", err)
	}
	// Get test failure bundle.
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Fmt("get test failure bundle: %w", err)
	}
	// Only rerun the non-diverged test failures.
	tfs := bundle.NonDiverged()
	commit := &suspect.GitilesCommit
	option := projectbisector.RerunOption{
		FullRun: true,
	}
	suspectBuild, err := projectBisector.TriggerRerun(ctx, tfa, tfs, commit, option)
	if err != nil {
		return errors.Fmt("trigger suspect rerun for commit %s: %w", commit.Id, err)
	}

	parentCommit, err := getParentCommit(ctx, commit)
	if err != nil {
		return errors.Fmt("get parent commit for commit %s: %w", commit.Id, err)
	}
	parentBuild, err := projectBisector.TriggerRerun(ctx, tfa, tfs, parentCommit, option)
	if err != nil {
		return errors.Fmt("trigger parent rerun for commit %s: %w", parentCommit.Id, err)
	}

	// Save rerun models.
	options := rerun.CreateTestRerunModelOptions{
		TestFailureAnalysis: tfa,
		SuspectKey:          datastore.KeyForObj(ctx, suspect),
		TestFailures:        tfs,
		Build:               suspectBuild,
		RerunType:           model.RerunBuildType_CulpritVerification,
	}
	suspectRerun, err := rerun.CreateTestRerunModel(ctx, options)
	if err != nil {
		return errors.Fmt("create test rerun model for suspect rerun: %w", err)
	}
	options.Build = parentBuild
	parentRerun, err := rerun.CreateTestRerunModel(ctx, options)
	if err != nil {
		return errors.Fmt("create test rerun model for parent rerun: %w", err)
	}

	// Update suspect.
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, suspect)
		if e != nil {
			return e
		}
		suspect.VerificationStatus = model.SuspectVerificationStatus_UnderVerification
		suspect.SuspectRerunBuild = datastore.KeyForObj(ctx, suspectRerun)
		suspect.ParentRerunBuild = datastore.KeyForObj(ctx, parentRerun)
		return datastore.Put(ctx, suspect)
	}, nil)
}
