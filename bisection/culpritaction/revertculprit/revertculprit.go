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

// Package revertculprit contains the logic to revert culprits
package revertculprit

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/rotationproxy"
	"go.chromium.org/luci/bisection/model"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

const (
	taskClass = "revert-culprit-action"
	queue     = "revert-culprit-action"
)

// RegisterTaskClass registers the task class for tq dispatcher
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: (*taskpb.RevertCulpritTask)(nil),
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler:   processRevertCulpritTask,
	})
}

func processRevertCulpritTask(ctx context.Context, payload proto.Message) error {
	task := payload.(*taskpb.RevertCulpritTask)

	analysisID := task.GetAnalysisId()
	culpritID := task.GetCulpritId()

	ctx, err := loggingutil.UpdateLoggingWithAnalysisID(ctx, analysisID)
	if err != nil {
		// not critical, just log
		err := errors.Annotate(err, "failed UpdateLoggingWithAnalysisID %d", analysisID)
		logging.Errorf(ctx, "%v", err)
	}

	logging.Infof(ctx,
		"Processing revert culprit task for analysis ID=%d, culprit ID=%d",
		analysisID, culpritID)

	cfa, err := datastoreutil.GetCompileFailureAnalysis(ctx, analysisID)
	if err != nil {
		// failed getting the CompileFailureAnalysis, so no point retrying
		err = errors.Annotate(err,
			"failed getting CompileFailureAnalysis when processing culprit revert task").Err()
		logging.Errorf(ctx, err.Error())
		return nil
	}

	// The analysis should be canceled. We should not do any gerrit actions.
	if cfa.ShouldCancel {
		logging.Errorf(ctx, "Analysis %d was canceled. No gerrit action required.")
		return nil
	}

	var culprit *model.Suspect
	for _, verifiedCulprit := range cfa.VerifiedCulprits {
		if verifiedCulprit.IntID() == culpritID {
			culprit, err = datastoreutil.GetSuspect(ctx, culpritID, verifiedCulprit.Parent())
			if err != nil {
				// failed getting the Suspect, so no point retrying
				err = errors.Annotate(err,
					"failed getting Suspect when processing culprit revert task").Err()
				logging.Errorf(ctx, err.Error())
				return nil
			}
			break
		}
	}

	if culprit == nil {
		// culprit is not within the analysis' verified culprits, so no point retrying
		logging.Errorf(ctx, "failed to find the culprit within the analysis' verified culprits")
		return nil
	}

	// Revert heuristic culprit
	if culprit.Type == model.SuspectType_Heuristic {
		err = RevertHeuristicCulprit(ctx, culprit)
		if err != nil {
			// If the error is transient, return err to retry
			if transient.Tag.In(err) {
				return err
			}

			// non-transient error, so do not retry
			logging.Errorf(ctx, err.Error())
			return nil
		}
		return nil
	}

	// TODO (aredulla): add functionality to revert nth section culprit

	logging.Infof(ctx, "Culprit type '%s' not supported for revert", culprit.Type)
	return nil
}

// RevertHeuristicCulprit attempts to automatically revert a culprit
// identified as a result of a heuristic analysis.
// If an unexpected error occurs, it is logged and returned.
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

	// Check for existing reverts
	reverts, err := gerritClient.GetReverts(ctx, culprit)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	activeReverts := []*gerritpb.ChangeInfo{}
	abandonedReverts := []*gerritpb.ChangeInfo{}
	for _, revert := range reverts {
		logging.Debugf(ctx, "Existing revert found for culprit %s~%d - revert is %s~%d",
			culprit.Project, culprit.Number, revert.Project, revert.Number)

		switch revert.Status {
		case gerritpb.ChangeStatus_MERGED:
			// there is a merged revert - no further action required
			err = saveRevertURL(ctx, gerritClient, culpritModel, revert)
			if err != nil {
				// not critical - just log the error
				logging.Errorf(ctx, err.Error())
			}

			return nil
		case gerritpb.ChangeStatus_ABANDONED:
			abandonedReverts = append(abandonedReverts, revert)
		case gerritpb.ChangeStatus_NEW:
			activeReverts = append(activeReverts, revert)
		}
	}

	if len(activeReverts) > 0 {
		// there is an existing revert yet to be merged

		// update the revert URL using the first revert
		revert := activeReverts[0]
		err = saveRevertURL(ctx, gerritClient, culpritModel, revert)
		if err != nil {
			// not critical - just log the error
			logging.Errorf(ctx, err.Error())
		}

		// add a supporting comment to the first revert
		err = commentSupportOnExistingRevert(ctx, gerritClient, culpritModel, activeReverts[0])
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	if len(abandonedReverts) > 0 {
		// there is an abandoned revert

		// update the revert URL using the first revert
		revert := abandonedReverts[0]
		err = saveRevertURL(ctx, gerritClient, culpritModel, revert)
		if err != nil {
			// not critical - just log the error
			logging.Errorf(ctx, err.Error())
		}

		// add a comment on the culprit since the revert has been abandoned
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			"an abandoned revert already exists")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	hasFlag, err := gerrit.HasAutoRevertOffFlagSet(ctx, culprit)
	if err != nil {
		err = errors.Annotate(err, "issue checking for auto-revert flag").Err()
		logging.Errorf(ctx, err.Error())
		return err
	}
	if hasFlag {
		// comment that auto-revert has been disabled for this change
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			"auto-revert has been disabled for this CL by its description")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	cannotRevert, err := HasIrrevertibleAuthor(ctx, culprit)
	if err != nil {
		err = errors.Annotate(err, "issue getting culprit's commit author").Err()
		logging.Errorf(ctx, err.Error())
		return err
	}
	if cannotRevert {
		// comment that the author of the culprit is irrevertible
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			"LUCI Bisection cannot revert changes from this CL's author")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	// Check if there are other merged changes depending on the culprit
	// before trying to revert it
	hasDep, err := gerritClient.HasDependency(ctx, culprit)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if hasDep {
		// revert should not be created as the culprit has merged dependencies
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			"there are merged changes depending on it")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	// Check if the Gerrit config allows revert creation
	canCreate, reason, err := config.CanCreateRevert(ctx, cfg.GerritConfig)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if !canCreate {
		// cannot create revert based on config
		err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
			reason)
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	// Create revert
	revert, err := createRevert(ctx, gerritClient, culpritModel, culprit)
	if err != nil {
		// unexpected error from creating the revert
		logging.Errorf(ctx, err.Error())

		if revert != nil {
			// a revert was created by LUCI Bisection - add reviewers to it
			err = sendRevertForReview(ctx, gerritClient, culpritModel, revert,
				"an unexpected error occurred when LUCI Bisection created this revert")
			if err != nil {
				logging.Errorf(ctx, err.Error())
				return err
			}
		}

		return nil
	}

	// Check if the culprit was committed recently
	maxAge := time.Duration(cfg.GerritConfig.MaxRevertibleCulpritAge) * time.Second
	if !gerrit.IsRecentSubmit(ctx, culprit, maxAge) {
		// culprit was not submitted recently, so the revert should not be
		// automatically submitted
		err = sendRevertForReview(ctx, gerritClient, culpritModel, revert,
			"the target of this revert was not committed recently")
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		// no longer submitting revert as it's too old - stop now
		return nil
	}

	// Check if the Gerrit config allows revert submission
	canSubmit, reason, err := config.CanSubmitRevert(ctx, cfg.GerritConfig)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if !canSubmit {
		// cannot submit revert based on config - send the revert to be
		// manually reviewed instead
		err = sendRevertForReview(ctx, gerritClient, culpritModel, revert, reason)
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	// Commit revert
	committed, err := commitRevert(ctx, gerritClient, culpritModel, revert)
	if err != nil {
		// Send the revert to be manually reviewed if it was not committed
		if !committed {
			reviewErr := sendRevertForReview(ctx, gerritClient, culpritModel, revert,
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
	bugURL := util.ConstructLUCIBisectionBugURL(ctx, analysisURL, culpritModel.ReviewUrl)

	_, err = gerritClient.AddComment(ctx, revert,
		fmt.Sprintf("LUCI Bisection recommends submitting this revert because"+
			" it has confirmed the target of this revert is the culprit of a"+
			" build failure. See the analysis: %s\n\n"+
			"Sample failed build: %s\n\n"+
			"If this is a false positive, please report it at %s",
			analysisURL, buildURL, bugURL))
	if err != nil {
		return errors.Annotate(err,
			"error when adding supporting comment to existing revert").Err()
	}

	// Update culprit for the supporting comment action
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}

		// set the flag to record the revert has a supporting comment from LUCI Bisection
		culpritModel.HasSupportRevertComment = true
		culpritModel.SupportRevertCommentTime = clock.Now(ctx)

		return datastore.Put(ctx, culpritModel)
	}, nil)

	if err != nil {
		return errors.Annotate(err,
			"couldn't update suspect details when commenting support for existing revert").Err()
	}

	return nil
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
	bugURL := util.ConstructLUCIBisectionBugURL(ctx, analysisURL, culpritModel.ReviewUrl)

	message := fmt.Sprintf("LUCI Bisection has identified this"+
		" change as the culprit of a build failure. See the analysis: %s\n\n"+
		"A revert for this change was not created because %s.\n\n"+
		"Sample failed build: %s\n\n"+
		"If this is a false positive, please report it at %s",
		analysisURL, reason, buildURL, bugURL)

	_, err = gerritClient.AddComment(ctx, culprit, message)
	if err != nil {
		return errors.Annotate(err, "error when commenting on culprit").Err()
	}

	// Update culprit for the comment action
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}

		// set the flag to note that the culprit has a comment from LUCI Bisection
		culpritModel.HasCulpritComment = true
		culpritModel.CulpritCommentTime = clock.Now(ctx)

		return datastore.Put(ctx, culpritModel)
	}, nil)

	if err != nil {
		return errors.Annotate(err,
			"couldn't update suspect details when commenting on the culprit").Err()
	}

	return nil
}

// sendRevertForReview adds a comment from LUCI Bisection on a revert CL
// explaining why a revert was not automatically submitted.
// TODO: this function will also add arborists on rotation as reviewers to the
// revert CL
func sendRevertForReview(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo, reason string) error {
	// TODO (aredulla): store reason in datastore to allow investigation

	// Get on-call arborists
	reviewerEmails, err := rotationproxy.GetOnCallEmails(ctx,
		culpritModel.GitilesCommit.Project)
	if err != nil {
		return errors.Annotate(err, "failed getting reviewers for manual review").Err()
	}

	// For now, no accounts are additionally CC'd
	ccEmails := []string{}

	message := fmt.Sprintf("LUCI Bisection could not automatically"+
		" submit this revert because %s.", reason)
	_, err = gerritClient.SendForReview(ctx, revert, message,
		reviewerEmails, ccEmails)
	return err
}

// saveRevertURL updates the revert URL for the given Suspect
func saveRevertURL(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}
		// set the revert CL URL
		culpritModel.RevertURL = util.ConstructGerritCodeReviewURL(ctx, gerritClient, revert)
		return datastore.Put(ctx, culpritModel)
	}, nil)

	if err != nil {
		err = errors.Annotate(err,
			"couldn't update suspect details for culprit with existing revert").Err()
		return err
	}

	return nil
}
