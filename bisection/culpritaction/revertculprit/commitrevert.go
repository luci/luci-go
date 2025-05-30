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

package revertculprit

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/rotationproxy"
	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
)

// canCommit returns:
//   - whether a revert for the culprit CL can be committed;
//   - the reason a revert should not be committed if applicable; and
//   - the error if one occurred.
func canCommit(ctx context.Context, culprit *gerritpb.ChangeInfo, culpritModel *model.Suspect, project string) (bool, string, error) {
	// TODO(beining@): remove this when revert CL support has been added for test failure.
	if culpritModel.AnalysisType == bisectionpb.AnalysisType_TEST_FAILURE_ANALYSIS {
		return false, "LUCI Bisection has not yet support auto-commit of revert CL for test failure", nil
	}
	// Get gerrit config.
	gerritConfig, err := config.GetGerritCfgForSuspect(ctx, culpritModel, project)
	if err != nil {
		return false, "", errors.Fmt("error get gerrit config: %w", err)
	}

	// Check if the culprit was committed recently
	maxAge := time.Duration(gerritConfig.MaxRevertibleCulpritAge) * time.Second
	if !gerrit.IsRecentSubmit(ctx, culprit, maxAge) {
		// culprit was not submitted recently, so the revert should not be
		// automatically submitted
		return false, "the target of this revert was not committed recently", nil
	}

	// Check if LUCI Bisection's Gerrit config allows revert submission
	canSubmit, reason, err := config.CanSubmitRevert(ctx, gerritConfig)
	if err != nil {
		return false, "", errors.Fmt("error checking Submit Revert configs: %w", err)
	}
	if !canSubmit {
		// cannot submit revert based on config
		return false, reason, nil
	}

	// We can only proceed to commit if it is a confirmed culprit
	// This is for the case that a we may create a revert on verification error of an
	// nthsection suspect. But we definitely don't want to auto submit the revert.
	if culpritModel.VerificationStatus != model.SuspectVerificationStatus_ConfirmedCulprit {
		return false, "the suspect is not verified", nil
	}
	return true, "", nil
}

// commitRevert attempts to bot-commit the given revert.
// Note: this should only be called according to the service-wide configuration
// data for LUCI Bisection, i.e.
//   - Gerrit actions are enabled
//   - Submitting reverts is enabled
//   - the daily limit of submitted reverts has not yet been reached
//   - the culprit is not yet older than the maximum revertible culprit age
func commitRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) error {
	// CC on-call arborists
	ccEmails, err := rotationproxy.GetOnCallEmails(ctx,
		culpritModel.GitilesCommit.Project)
	if err != nil {
		// non-critical, just log the error
		err = errors.Fmt("failed getting accounts to CC on bot-commit: %w", err)
		logging.Errorf(ctx, err.Error())
	}

	_, err = gerritClient.CommitRevert(ctx, revert,
		"LUCI Bisection is automatically submitting this revert.", ccEmails)
	return err
}
