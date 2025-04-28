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
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
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
		return errors.Annotate(err, "get test failure analysis").Err()
	}

	// Retrieves suspect.
	suspect, err := datastoreutil.GetSuspectForTestAnalysis(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "couldn't get suspect").Err()
	}
	if suspect == nil {
		return errors.New("couldn't get suspect: <unknown reason>")
	}

	return verifyTestFailureSuspect(ctx, tfa, suspect)
}

func verifyTestFailureSuspect(ctx context.Context, tfa *model.TestFailureAnalysis, suspect *model.Suspect) error {
	projectBisector, err := bisection.GetProjectBisector(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get project bisector").Err()
	}
	// Get test failure bundle.
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failure bundle").Err()
	}
	// Only rerun the non-diverged test failures.
	tfs := bundle.NonDiverged()
	commit := &suspect.GitilesCommit
	option := projectbisector.RerunOption{
		FullRun: true,
	}
	suspectBuild, err := projectBisector.TriggerRerun(ctx, tfa, tfs, commit, option)
	if err != nil {
		return errors.Annotate(err, "trigger suspect rerun for commit %s", commit.Id).Err()
	}

	parentCommit, err := getParentCommit(ctx, commit)
	if err != nil {
		return errors.Annotate(err, "get parent commit for commit %s", commit.Id).Err()
	}
	parentBuild, err := projectBisector.TriggerRerun(ctx, tfa, tfs, parentCommit, option)
	if err != nil {
		return errors.Annotate(err, "trigger parent rerun for commit %s", parentCommit.Id).Err()
	}

	// Save rerun models.
	options := bisection.CreateRerunModelOptions{
		TestFailureAnalysis: tfa,
		SuspectKey:          datastore.KeyForObj(ctx, suspect),
		TestFailures:        tfs,
		Build:               suspectBuild,
		RerunType:           model.RerunBuildType_CulpritVerification,
	}
	suspectRerun, err := bisection.CreateTestRerunModel(ctx, options)
	if err != nil {
		return errors.Annotate(err, "create test rerun model for suspect rerun").Err()
	}
	options.Build = parentBuild
	parentRerun, err := bisection.CreateTestRerunModel(ctx, options)
	if err != nil {
		return errors.Annotate(err, "create test rerun model for parent rerun").Err()
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
