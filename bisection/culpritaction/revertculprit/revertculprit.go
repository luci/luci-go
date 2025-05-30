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
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

var CompileFailureTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "revert-culprit-action",
	Prototype: (*taskpb.RevertCulpritTask)(nil),
	Queue:     "revert-culprit-action",
	Kind:      tq.NonTransactional,
})

var TestFailureTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "test-failure-culprit-action",
	Prototype: (*taskpb.TestFailureCulpritActionTask)(nil),
	Queue:     "test-failure-culprit-action",
	Kind:      tq.NonTransactional,
})

// RegisterTaskClass registers the task class for tq dispatcher
func RegisterTaskClass(srv *server.Server, luciAnalysisProjectFunc func(luciProject string) string) error {
	client, err := lucianalysis.NewClient(srv.Context, srv.Options.CloudProject, luciAnalysisProjectFunc)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		client.Close()
	})

	CompileFailureTasks.AttachHandler(processRevertCulpritTask)
	TestFailureTasks.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskpb.TestFailureCulpritActionTask)
		if err := processTestFailureCulpritTask(ctx, task.AnalysisId, client); err != nil {
			err := errors.Fmt("run test failure culprit action: %w", err)
			logging.Errorf(ctx, err.Error())

			// Return nil so the task will not be retried.
			// If in the future, task retry is required, remember to update HasTakenActions
			// field to false before retry.
			return nil
		}
		return nil
	})
	return nil
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
		err = errors.Fmt("failed getting CompileFailureAnalysis when processing culprit revert task: %w", err)

		logging.Errorf(ctx, err.Error())
		return nil
	}

	var culprit *model.Suspect
	for _, verifiedCulprit := range cfa.VerifiedCulprits {
		if verifiedCulprit.IntID() == culpritID {
			culprit, err = datastoreutil.GetSuspect(ctx, culpritID, verifiedCulprit.Parent())
			if err != nil {
				// failed getting the Suspect, so no point retrying
				err = errors.Fmt("failed getting Suspect when processing culprit revert task: %w", err)

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

	// The analysis should be canceled. We should not do any gerrit actions.
	if cfa.ShouldCancel {
		logging.Infof(ctx, "Analysis %d was canceled. No gerrit action required.", analysisID)
		saveInactionReason(ctx, culprit, pb.CulpritInactionReason_ANALYSIS_CANCELED)
		return nil
	}

	// Revert culprit
	err = TakeCulpritAction(ctx, culprit)
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

func isSuspectGerritActionReady(ctx context.Context, culpritModel *model.Suspect, gerritConfig *configpb.GerritConfig) (bool, error) {
	// We only proceed with heuristic culprit if it is a confirmed culprit
	if culpritModel.Type == model.SuspectType_Heuristic {
		if culpritModel.VerificationStatus == model.SuspectVerificationStatus_ConfirmedCulprit {
			return true, nil
		}
		return false, fmt.Errorf("suspect (commit %s) has verification status %s and should not be reverted",
			culpritModel.GitilesCommit.Id, culpritModel.VerificationStatus)
	}
	// Nthsection
	if culpritModel.Type == model.SuspectType_NthSection {
		settings := gerritConfig.NthsectionSettings
		if !settings.Enabled {
			logging.Infof(ctx, "Nthsection settings is disabled")
			return false, nil
		}
		if culpritModel.VerificationStatus == model.SuspectVerificationStatus_ConfirmedCulprit || (settings.ActionWhenVerificationError && culpritModel.VerificationStatus == model.SuspectVerificationStatus_VerificationError) {
			return true, nil
		}
		return false, fmt.Errorf("suspect (commit %s) has verification status %s and should not be reverted",
			culpritModel.GitilesCommit.Id, culpritModel.VerificationStatus)
	}
	return false, fmt.Errorf("unsupported suspect type: %s", culpritModel.Type)
}

// TakeCulpritAction attempts to comment culprit, comment revert, create revert and commit revert for a culprit
// when the culprit satisfies the critieria of the action.
// A culprit is identified as a result of a heuristic analysis or an nthsection analysis.
func TakeCulpritAction(ctx context.Context, culpritModel *model.Suspect) error {
	project, err := datastoreutil.GetProjectForSuspect(ctx, culpritModel)
	if err != nil {
		return errors.Fmt("get project for suspect: %w", err)
	}
	// Get gerrit config.
	gerritConfig, err := config.GetGerritCfgForSuspect(ctx, culpritModel, project)
	if err != nil {
		return errors.Fmt("get gerrit config for suspect: %w", err)
	}
	// Check if Gerrit actions are disabled
	if !gerritConfig.ActionsEnabled {
		logging.Infof(ctx, "Gerrit actions have been disabled")
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_ACTIONS_DISABLED)
		return nil
	}

	// Check the culprit verification status
	shouldTakeAction, err := isSuspectGerritActionReady(ctx, culpritModel, gerritConfig)
	if err != nil {
		return err
	}
	if !shouldTakeAction {
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
	existingRevert, err := getMostRelevantRevert(ctx, gerritClient, culprit)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if existingRevert != nil {
		err = saveRevertURL(ctx, gerritClient, culpritModel, existingRevert)
		if err != nil {
			// not critical - just log the error
			logging.Errorf(ctx, err.Error())
		}

		switch existingRevert.Status {
		case gerritpb.ChangeStatus_MERGED:
			// There is a merged revert - no further action required.

			// Update the inaction reason based on whether the revert was created by
			// LUCI Bisection.
			lbOwned, err := gerrit.IsOwnedByLUCIBisection(ctx, existingRevert)
			if err != nil {
				// Not critical - just log the error and skip updating the
				// inaction reason.
				err = errors.Fmt("no action required but failed to find owner of existing revert: %w", err)

				logging.Errorf(ctx, err.Error())
			} else {
				reason := pb.CulpritInactionReason_REVERTED_MANUALLY
				if lbOwned {
					reason = pb.CulpritInactionReason_REVERTED_BY_BISECTION
				}
				saveInactionReason(ctx, culpritModel, reason)
			}

			return nil
		case gerritpb.ChangeStatus_NEW:
			// add a supporting comment to the first revert
			err = commentSupportOnExistingRevert(ctx, gerritClient, culpritModel, existingRevert)
			if err != nil {
				logging.Errorf(ctx, err.Error())
				return err
			}
			return nil
		case gerritpb.ChangeStatus_ABANDONED:
			// add a comment on the culprit since the revert has been abandoned
			err = commentReasonOnCulprit(ctx, gerritClient, culpritModel, culprit,
				"an abandoned revert already exists")
			if err != nil {
				logging.Errorf(ctx, err.Error())
				return err
			}
			return nil
		default:
			logging.Errorf(ctx,
				"status was not recognized for existing revert %s~%d [status='%v']",
				existingRevert.Project, existingRevert.Number, existingRevert.Status)
		}

		return nil
	}

	shouldCreateRevert, reason, err := isCulpritRevertible(ctx, gerritClient, culprit, culpritModel, project)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if !shouldCreateRevert {
		// Add a comment on the culprit CL to explain why a revert was not created
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
		logging.Errorf(ctx, err.Error())

		if status.Convert(errors.Unwrap(err)).Code() == codes.DeadlineExceeded {
			// Workaround for Gerrit performance issue with revert creations
			// (see b/261896675). The request may have timed out but the revert may
			// have been successfully created, so look for the newly created revert
			createdRevert, searchErr := searchForCreatedRevert(ctx, gerritClient,
				culpritModel, culprit)
			if searchErr != nil {
				logging.Errorf(ctx, searchErr.Error())
				return searchErr
			}

			if createdRevert != nil {
				logging.Debugf(ctx, "continuing revert process; found created revert")
				revert = createdRevert
			} else {
				logging.Debugf(ctx, "could not find the revert created by LUCI Bisection")
				return err
			}
		} else {
			return err
		}
	}

	err = saveCreationDetails(ctx, gerritClient, culpritModel, revert)
	if err != nil {
		logging.Errorf(ctx, err.Error())

		// a revert was created by LUCI Bisection - add reviewers to it
		shouldReview, reviewErr := isRevertActive(ctx, gerritClient, revert)
		if reviewErr != nil {
			logging.Errorf(ctx, reviewErr.Error())
			return reviewErr
		}
		if shouldReview {
			reviewErr = sendRevertForReview(ctx, gerritClient, culpritModel, revert,
				"an unexpected error occurred after LUCI Bisection created this revert")
			if reviewErr != nil {
				logging.Errorf(ctx, reviewErr.Error())
				return reviewErr
			}
		}

		return err
	}

	// Check again for merged reverts for the culprit, in case
	// another revert was manually created and merged while waiting for Gerrit
	// to finish creating the revert.
	existingReverts, err := gerritClient.GetReverts(ctx, culprit)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	for _, existingRevert := range existingReverts {
		if existingRevert.Status == gerritpb.ChangeStatus_MERGED {
			// A revert has already been merged, so there is no need to commit the
			// revert created by LUCI Bisection
			logging.Debugf(ctx, "existing revert %s~%d already merged for culprit %s~%d",
				existingRevert.Project, existingRevert.Number,
				culprit.Project, culprit.Number)

			// TODO (nqmtuan): Automatically abandon the revert created by
			// LUCI Bisection if this merged revert is different. Currently, the
			// created revert will be left open until manually abandoned.

			return nil
		}
	}

	// Check the revert is still active, as creation can take a long time so
	// it may have been manually updated
	isActive, err := isRevertActive(ctx, gerritClient, revert)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if !isActive {
		// revert has been manually updated, so no further action is required
		return nil
	}

	shouldCommit, reason, err := canCommit(ctx, culprit, culpritModel, project)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}
	if !shouldCommit {
		// Send the revert for manual review and add a comment to explain why the
		// revert was not automatically submitted
		err = sendRevertForReview(ctx, gerritClient, culpritModel, revert,
			reason)
		if err != nil {
			logging.Errorf(ctx, err.Error())
			return err
		}

		return nil
	}

	// Commit revert
	err = commitRevert(ctx, gerritClient, culpritModel, revert)
	if err != nil {
		logging.Errorf(ctx, err.Error())

		// Send the revert to be manually reviewed
		reviewErr := sendRevertForReview(ctx, gerritClient, culpritModel, revert,
			"an error occurred when attempting to submit it")
		if reviewErr != nil {
			logging.Errorf(ctx, reviewErr.Error())
			return reviewErr
		}

		return err
	}
	err = saveCommitDetails(ctx, culpritModel)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return err
	}

	return nil
}

// getMostRelevantRevert returns the most relevant revert based on the
// revert change's status, in the order of merged > active > abandoned > nil.
func getMostRelevantRevert(ctx context.Context, gerritClient *gerrit.Client,
	culprit *gerritpb.ChangeInfo) (*gerritpb.ChangeInfo, error) {
	// Check for existing reverts
	reverts, err := gerritClient.GetReverts(ctx, culprit)
	if err != nil {
		return nil, err
	}

	var activeRevert *gerritpb.ChangeInfo = nil
	var abandonedRevert *gerritpb.ChangeInfo = nil
	for _, revert := range reverts {
		logging.Debugf(ctx, "Existing revert found for culprit %s~%d - revert is %s~%d",
			culprit.Project, culprit.Number, revert.Project, revert.Number)

		switch revert.Status {
		case gerritpb.ChangeStatus_MERGED:
			return revert, nil
		case gerritpb.ChangeStatus_ABANDONED:
			if abandonedRevert == nil {
				abandonedRevert = revert
			}
		case gerritpb.ChangeStatus_NEW:
			if activeRevert == nil {
				activeRevert = revert
			}
		default:
			logging.Debugf(ctx, "ignoring revert %s~%d due to its unrecognized status %v",
				revert.Project, revert.Number, revert.Status)
		}
	}

	if activeRevert != nil {
		// there is an existing revert yet to be merged
		return activeRevert, nil
	}

	if abandonedRevert != nil {
		// there is an abandoned revert
		return abandonedRevert, nil
	}

	return nil, nil
}

// searchForCreatedRevert returns the revert CL created by LUCI Bisection
// when processing the given Suspect, if it exists.
func searchForCreatedRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, culprit *gerritpb.ChangeInfo) (*gerritpb.ChangeInfo, error) {
	// Construct the revert description to use for comparison since different
	// analyses can result in the same culprit CL. The revert description
	// similarity can be used to ascertain whether a revert CL was created for
	// this specific Suspect
	generatedRevertDescription, err := generateRevertDescription(ctx, culpritModel, culprit)
	if err != nil {
		return nil, errors.Annotate(err, "failed generating revert description"+
			" for comparison when searching for created revert").Err()
	}
	// Drop the last paragraph for the comparison as Gerrit may have inserted
	// its own values in the footer
	paragraphs := strings.Split(generatedRevertDescription, "\n\n")
	descriptionStart := strings.Join(paragraphs[:len(paragraphs)-1], "\n\n")

	// Check for existing reverts
	reverts, err := gerritClient.GetReverts(ctx, culprit)
	if err != nil {
		return nil, errors.Fmt("failed getting existing reverts when searching for created revert: %w", err)

	}

	var createdRevert *gerritpb.ChangeInfo = nil
	for _, revert := range reverts {
		lbOwned, err := gerrit.IsOwnedByLUCIBisection(ctx, revert)
		if err != nil {
			// non-critical - log the error and move on
			err = errors.Fmt("error searching for created revert when checking owner: %w", err)

			logging.Errorf(ctx, err.Error())
			continue
		}

		// Check if the revert was created by LUCI Bisection
		if lbOwned {
			revertDescription, err := gerrit.CommitMessage(ctx, revert)
			if err != nil {
				// non-critical - log the error and move on
				err = errors.Fmt("error searching for created revert when getting commit message: %w", err)

				logging.Errorf(ctx, err.Error())
				continue
			}

			// Check if the description starts as expected, to confirm this revert CL
			// was the newly-created one for this specific Suspect and not from
			// another analysis
			if strings.HasPrefix(revertDescription, descriptionStart) {
				createdRevert = revert
				break
			}
		}
	}

	return createdRevert, nil
}

func isRevertActive(ctx context.Context, gerritClient *gerrit.Client,
	revert *gerritpb.ChangeInfo) (bool, error) {
	// Refetch the created revert to get its latest status
	revert, err := gerritClient.RefetchChange(ctx, revert)
	if err != nil {
		return false, errors.Fmt("error refetching revert created by LUCI Bisection: %w", err)

	}

	if revert.Status == gerritpb.ChangeStatus_NEW {
		return true, nil
	} else {
		// the revert created by LUCI Bisection has been manually updated
		logging.Debugf(ctx, "revert %s~%d created by LUCI Bisection was updated"+
			" manually [status=%v]", revert.Project, revert.Number, revert.Status)
		return false, nil
	}
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
		err = errors.Fmt("couldn't update suspect details for culprit with existing revert: %w", err)

		return err
	}

	return nil
}

func saveCreationDetails(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) error {
	// Update tsmon metrics
	err := updateCulpritActionCounter(ctx, culpritModel, ActionTypeCreateRevert)
	if err != nil {
		logging.Errorf(ctx, errors.Fmt("updateCulpritActionCounter: %w", err).Error())
	}

	// Update revert details for creation
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}

		culpritModel.RevertURL = util.ConstructGerritCodeReviewURL(ctx, gerritClient, revert)
		culpritModel.IsRevertCreated = true
		culpritModel.RevertCreateTime = clock.Now(ctx)

		return datastore.Put(ctx, culpritModel)
	}, nil)
	if err != nil {
		return errors.Fmt("couldn't update suspect revert creation details: %w", err)

	}
	return nil
}

func saveCommitDetails(ctx context.Context, culpritModel *model.Suspect) error {
	// Update tsmon metrics
	err := updateCulpritActionCounter(ctx, culpritModel, ActionTypeSubmitRevert)
	if err != nil {
		logging.Errorf(ctx, errors.Fmt("updateCulpritActionCounter: %w", err).Error())
	}

	// Update revert details for commit action
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}

		culpritModel.IsRevertCommitted = true
		culpritModel.RevertCommitTime = clock.Now(ctx)

		return datastore.Put(ctx, culpritModel)
	}, nil)
	if err != nil {
		return errors.Fmt("couldn't update suspect revert commit details: %w", err)

	}
	return nil
}

// saveInactionReason updates the inaction reason for the given Suspect.
func saveInactionReason(ctx context.Context, culpritModel *model.Suspect,
	reason pb.CulpritInactionReason) {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}
		// Set the inaction reason
		culpritModel.InactionReason = reason
		return datastore.Put(ctx, culpritModel)
	}, nil)

	if err != nil {
		// not critical - just log the error
		err = errors.Fmt("couldn't update suspect inaction reason: %w", err)

		logging.Errorf(ctx, err.Error())
	}
}

func ScheduleTestFailureTask(ctx context.Context, analysisID int64) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: &taskpb.TestFailureCulpritActionTask{
			AnalysisId: analysisID,
		},
		Title: fmt.Sprintf("test_failure_culprit_action_%d", analysisID),
	})
}
