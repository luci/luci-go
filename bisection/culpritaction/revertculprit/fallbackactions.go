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
	"fmt"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/rotationproxy"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
)

func commentSupportOnExistingRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) error {
	lbOwned, err := gerrit.IsOwnedByLUCIBisection(ctx, revert)
	if err != nil {
		return errors.Fmt("failed handling existing revert when finding owner: %w", err)

	}

	if lbOwned {
		// Revert is owned by LUCI Bisection - no further action required
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_REVERT_OWNED_BY_BISECTION)
		return nil
	}

	// Revert is not owned by LUCI Bisection
	lbCommented, err := gerrit.HasLUCIBisectionComment(ctx, revert)
	if err != nil {
		return errors.Fmt("failed handling existing revert when checking for pre-existing comment: %w", err)

	}

	if lbCommented {
		// Revert already has a comment by LUCI Bisection - no further action
		// required
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_REVERT_HAS_COMMENT)
		return nil
	}
	var message string
	switch culpritModel.AnalysisType {
	case pb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		message, err = compileFailureComment(ctx, culpritModel, "", "supportComment")
		if err != nil {
			return errors.Fmt("generate compile failure support comment: %w", err)
		}
	case pb.AnalysisType_TEST_FAILURE_ANALYSIS:
		message, err = testFailureComment(ctx, culpritModel, "", "supportComment")
		if err != nil {
			return errors.Fmt("generate test failure support comment: %w", err)
		}
	}
	_, err = gerritClient.AddComment(ctx, revert, message)
	if err != nil {
		return errors.Fmt("error when adding supporting comment to existing revert: %w", err)

	}

	// Update tsmon metrics
	err = updateCulpritActionCounter(ctx, culpritModel, ActionTypeCommentRevert)
	if err != nil {
		logging.Errorf(ctx, errors.Fmt("updateCulpritActionCounter: %w", err).Error())
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
		return errors.Fmt("couldn't update suspect details when commenting support for existing revert: %w", err)

	}
	return nil
}

// commentReasonOnCulprit adds a comment from LUCI Bisection on a culprit CL
// explaining why a revert was not automatically created
func commentReasonOnCulprit(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, culprit *gerritpb.ChangeInfo, reason string) error {
	logging.Debugf(ctx, "commenting on culprit %s~%d; a revert could not be"+
		" created because %s", culprit.Project, culprit.Number, reason)

	lbCommented, err := gerrit.HasLUCIBisectionComment(ctx, culprit)
	if err != nil {
		return errors.Fmt("failed handling failed revert creation when checking for pre-existing comment: %w", err)

	}

	if lbCommented {
		// Culprit already has a comment by LUCI Bisection - no further action
		// required
		saveInactionReason(ctx, culpritModel, pb.CulpritInactionReason_CULPRIT_HAS_COMMENT)
		return nil
	}

	var message string
	switch culpritModel.AnalysisType {
	case pb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		message, err = compileFailureComment(ctx, culpritModel, reason, "blameComment")
		if err != nil {
			return errors.Fmt("generate compile failure blame comment: %w", err)
		}
	case pb.AnalysisType_TEST_FAILURE_ANALYSIS:
		message, err = testFailureComment(ctx, culpritModel, reason, "blameComment")
		if err != nil {
			return errors.Fmt("generate test failure blame comment: %w", err)
		}
	}

	_, err = gerritClient.AddComment(ctx, culprit, message)
	if err != nil {
		return errors.Fmt("error when commenting on culprit: %w", err)
	}

	// Update tsmon metrics
	err = updateCulpritActionCounter(ctx, culpritModel, ActionTypeCommentCulprit)
	if err != nil {
		logging.Errorf(ctx, errors.Fmt("updateCulpritActionCounter: %w", err).Error())
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
		return errors.Fmt("couldn't update suspect details when commenting on the culprit: %w", err)

	}
	return nil
}

// sendRevertForReview adds a comment from LUCI Bisection on a revert CL
// explaining why a revert was not automatically submitted.
func sendRevertForReview(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo, reason string) error {
	logging.Debugf(ctx, "sending revert %s~%d for review because %s",
		revert.Project, revert.Number, reason)

	// Get on-call arborists
	reviewerEmails, err := rotationproxy.GetOnCallEmails(ctx,
		culpritModel.GitilesCommit.Project)
	if err != nil {
		return errors.Fmt("failed getting reviewers for manual review: %w", err)
	}

	// For now, no accounts are additionally CC'd
	ccEmails := []string{}

	message := fmt.Sprintf("LUCI Bisection could not automatically"+
		" submit this revert because %s.", reason)
	_, err = gerritClient.SendForReview(ctx, revert, message,
		reviewerEmails, ccEmails)
	return err
}
