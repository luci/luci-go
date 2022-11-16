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

// Package culpritaction contains the logic to deal with culprits
package culpritaction

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// RevertHeuristicCulprit attempts to automatically revert a culprit
// identified as a result of a heuristic analysis.
// If an unexpected error occurs, it is logged and returned.
// TODO (aredulla): call this when processing revert tasks in the task queue.
func RevertHeuristicCulprit(ctx context.Context, culpritModel *model.Suspect) error {
	// Check the culprit verification status
	if culpritModel.VerificationStatus != model.SuspectVerificationStatus_ConfirmedCulprit {
		return fmt.Errorf("suspect (commit %s) has verification status %s; must be %s to be reverted",
			culpritModel.GitilesCommit.Id, culpritModel.VerificationStatus,
			model.SuspectVerificationStatus_ConfirmedCulprit)
	}

	// Get config for LUCI Bisection
	cfg, err := config.Get(ctx)
	if err != nil {
		return err
	}

	// Check if Gerrit actions are disabled
	if !cfg.GerritConfig.ActionsEnabled {
		logging.Infof(ctx, "Gerrit actions have been disabled")
		return nil
	}

	// Make Gerrit client
	gerritHost, err := gerrit.GetHost(ctx, culpritModel.ReviewUrl)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	gerritClient, err := gerrit.NewClient(ctx, gerritHost)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}

	// Get the culprit's Gerrit change
	culprit, err := gerritClient.GetChange(ctx,
		culpritModel.GitilesCommit.Project, culpritModel.GitilesCommit.Id)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}

	// Check if revert creation is disabled
	if !cfg.GerritConfig.CreateRevertSettings.Enabled {
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			"LUCI Bisection's revert creation has been disabled")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// revert creation is disabled - stop now
		return nil
	}

	// Check the daily limit for revert creations has not been reached
	createdCount, err := datastoreutil.CountLatestRevertsCreated(ctx, 24)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}

	if createdCount >= int64(cfg.GerritConfig.CreateRevertSettings.DailyLimit) {
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			fmt.Sprintf("LUCI Bisection's daily limit for revert creation (%d)"+
				" has been reached; %d reverts have already been created.",
				cfg.GerritConfig.CreateRevertSettings.DailyLimit, createdCount))
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// revert creation daily limit has been reached - stop now
		return nil
	}

	// Create revert
	revert, err := createRevert(ctx, gerritClient, culpritModel, culprit)
	if err != nil {
		switch err {
		case errAlreadyReverted:
			// there is a merged revert - no further action required
			err = nil
		case errHasRevert:
			// there is an existing revert
			err = commentSupportOnExistingRevert(ctx, gerritClient, culpritModel,
				revert)
		case errHasDependency:
			// revert should not be created as the culprit has merged dependencies
			err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
				"there are merged changes depending on it")
		default:
			// unexpected error from creating the revert
			if revert != nil {
				// a revert was created by LUCI Bisection - add reviewers to it
				reviewErr := sendRevertForReview(ctx, gerritClient, revert,
					"an unexpected error occurred when LUCI Bisection created this revert")
				if reviewErr != nil {
					logging.Errorf(ctx, reviewErr.Error())
					return reviewErr
				}
			}
		}

		if err != nil {
			// log and return the unexpected error from:
			//    * creating the revert; or
			//    * commenting on an existing revert; or
			//    * commenting on a culprit.
			logging.Errorf(ctx, err.Error())
			return err
		}

		// no further action required for this culprit - stop now
		return nil
	}

	// Check if revert submission is disabled
	if !cfg.GerritConfig.SubmitRevertSettings.Enabled {
		// Send the revert to be manually reviewed as auto-committing is disabled
		err = sendRevertForReview(ctx, gerritClient, revert,
			"LUCI Bisection's revert submission has been disabled")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// revert submission is disabled - stop now
		return nil
	}

	// Get the number of reverts committed in the past day
	committedCount, err := datastoreutil.CountLatestRevertsCommitted(ctx, 24)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	// Check the daily limit for revert submissions has not been reached
	if committedCount >= int64(cfg.GerritConfig.SubmitRevertSettings.DailyLimit) {
		err = sendRevertForReview(ctx, gerritClient, revert,
			fmt.Sprintf("LUCI Bisection's daily limit for revert submission (%d)"+
				" has been reached; %d reverts have already been submitted.",
				cfg.GerritConfig.SubmitRevertSettings.DailyLimit, committedCount))
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// revert submission daily limit has been reached - stop now
		return nil
	}

	// Check if the culprit was committed recently
	maxAge := time.Duration(cfg.GerritConfig.MaxRevertibleCulpritAge) * time.Second
	if !gerrit.IsRecentSubmit(ctx, culprit, maxAge) {
		// culprit was not submitted recently, so the revert should not be
		// automatically submitted
		err = sendRevertForReview(ctx, gerritClient, revert,
			"the target of this revert was not committed recently")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// no longer submitting revert as it's too old - stop now
		return nil
	}

	// Commit revert
	committed, err := commitRevert(ctx, gerritClient, culpritModel, revert)
	if err != nil {
		// Send the revert to be manually reviewed if it was not committed
		if !committed {
			reviewErr := sendRevertForReview(ctx, gerritClient, revert,
				"an error occurred when attempting to submit it")
			if reviewErr != nil {
				logging.Errorf(ctx, reviewErr.Error())
				return reviewErr
			}
		}

		// log and return the unexpected error from committing revert
		logging.Errorf(ctx, err.Error())
		return err
	}

	return nil
}

func commentSupportOnExistingRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) error {
	// TODO: save the existing revert URL in datastore

	lbOwned, err := gerrit.IsOwnedByLUCIBisection(ctx, revert)
	if err != nil {
		return errors.Annotate(err,
			"failed handling existing revert when finding owner").Err()
	}

	if lbOwned {
		// Revert is owned by LUCI Bisection - no further action required
		return nil
	}

	// Revert is not owned by LUCI Bisection
	lbCommented, err := gerrit.HasLUCIBisectionComment(ctx, revert)
	if err != nil {
		return errors.Annotate(err,
			"failed handling existing revert when checking for pre-existing comment").Err()
	}

	if lbCommented {
		// Revert already has a comment by LUCI Bisection - no further action
		// required
		return nil
	}

	// If here, revert is not owned by LUCI Bisection and has no supporting comment

	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, culpritModel)
	if err != nil {
		return err
	}
	analysisURL := util.ConstructAnalysisURL(ctx, bbid)
	buildURL := util.ConstructBuildURL(ctx, bbid)

	_, err = gerritClient.AddComment(ctx, revert,
		fmt.Sprintf("LUCI Bisection recommends submitting this revert because"+
			" it has confirmed the target of this revert is the culprit of a"+
			" build failure. See the analysis: %s\n\n"+
			"Sample failed build: %s", analysisURL, buildURL))
	return err
}

// commentReasonOnCulprit adds a comment from LUCI Bisection on a culprit CL
// explaining why a revert was not automatically created
func commentReasonOnCulprit(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, culprit *gerritpb.ChangeInfo, reason string) error {
	// TODO (aredulla): store reason in datastore to allow investigation
	lbCommented, err := gerrit.HasLUCIBisectionComment(ctx, culprit)
	if err != nil {
		return errors.Annotate(err,
			"failed handling failed revert creation when checking for pre-existing comment").Err()
	}

	if lbCommented {
		// Culprit already has a comment by LUCI Bisection - no further action
		// required
		return nil
	}

	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, culpritModel)
	if err != nil {
		return err
	}
	analysisURL := util.ConstructAnalysisURL(ctx, bbid)
	buildURL := util.ConstructBuildURL(ctx, bbid)

	message := fmt.Sprintf("LUCI Bisection has identified this"+
		" change as the culprit of a build failure. See the analysis: %s\n\n"+
		"A revert for this change was not created because %s.\n\n"+
		"Sample failed build: %s", analysisURL, reason, buildURL)

	_, err = gerritClient.AddComment(ctx, culprit, message)
	return err
}

// sendRevertForReview adds a comment from LUCI Bisection on a revert CL
// explaining why a revert was not automatically submitted.
// TODO: this function will also add sheriffs on rotation as reviewers to the
// revert CL
func sendRevertForReview(ctx context.Context, gerritClient *gerrit.Client,
	revert *gerritpb.ChangeInfo, reason string) error {
	// TODO (aredulla): store reason in datastore to allow investigation

	// TODO (aredulla): add sheriffs on rotation as reviewers
	message := fmt.Sprintf("LUCI Bisection could not automatically"+
		" submit this revert because %s.", reason)
	_, err := gerritClient.SendForReview(ctx, revert, message,
		[]*gerritpb.AccountInfo{}, []*gerritpb.AccountInfo{})
	return err
}
