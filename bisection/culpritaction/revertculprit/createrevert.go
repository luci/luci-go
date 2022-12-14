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
	"strings"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// isCulpritRevertible returns:
//   - whether a revert should be created for the culprit CL;
//   - the reason it should not be created if applicable; and
//   - the error if one occurred.
func isCulpritRevertible(ctx context.Context, gerritClient *gerrit.Client,
	culprit *gerritpb.ChangeInfo) (bool, string, error) {
	// Check if the culprit's description has disabled autorevert
	hasFlag, err := gerrit.HasAutoRevertOffFlagSet(ctx, culprit)
	if err != nil {
		return false, "", errors.Annotate(err, "error checking for auto-revert flag").Err()
	}
	if hasFlag {
		return false, "auto-revert has been disabled for this CL by its description", nil
	}

	// Check if the author of the culprit is irrevertible
	cannotRevert, err := HasIrrevertibleAuthor(ctx, culprit)
	if err != nil {
		return false, "", errors.Annotate(err, "error getting culprit's commit author").Err()
	}
	if cannotRevert {
		return false, "LUCI Bisection cannot revert changes from this CL's author", nil
	}

	// Check if there are other merged changes depending on the culprit
	hasDep, err := gerritClient.HasDependency(ctx, culprit)
	if err != nil {
		return false, "", errors.Annotate(err, "error checking for dependencies").Err()
	}
	if hasDep {
		return false, "there are merged changes depending on it", nil
	}

	// Check if LUCI Bisection's Gerrit config allows revert creation
	cfg, err := config.Get(ctx)
	if err != nil {
		return false, "", errors.Annotate(err, "error fetching configs").Err()
	}
	canCreate, reason, err := config.CanCreateRevert(ctx, cfg.GerritConfig)
	if err != nil {
		return false, "", errors.Annotate(err, "error checking Create Revert configs").Err()
	}
	if !canCreate {
		return false, reason, nil
	}

	return true, "", nil
}

// createRevert creates a revert for the given culprit.
// Returns a revert for the culprit, created by LUCI Bisection.
// Note: this should only be called according to the service-wide configuration
// data for LUCI Bisection, i.e.
//   - Gerrit actions are enabled
//   - creating reverts is enabled
//   - the daily limit of created reverts has not yet been reached
func createRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, culprit *gerritpb.ChangeInfo) (*gerritpb.ChangeInfo, error) {
	revertDescription, err := generateRevertDescription(ctx, culpritModel, culprit)
	if err != nil {
		return nil, err
	}

	// Create the revert
	revert, err := gerritClient.CreateRevert(ctx, culprit, revertDescription)
	if err != nil {
		return nil, err
	}

	return revert, nil
}

func generateRevertDescription(ctx context.Context, culpritModel *model.Suspect,
	culprit *gerritpb.ChangeInfo) (string, error) {
	paragraphs := []string{}

	if culprit.Subject != "" {
		paragraphs = append(paragraphs,
			fmt.Sprintf("Revert \"%s\"", culprit.Subject))
	} else {
		paragraphs = append(paragraphs,
			fmt.Sprintf("Revert \"%s~%d\"", culprit.Project, culprit.Number))
	}

	paragraphs = append(paragraphs,
		fmt.Sprintf("This reverts commit %s.", culpritModel.GitilesCommit.Id))

	// Add link to LUCI Bisection failure analysis details and failed build
	bbid, err := datastoreutil.GetAssociatedBuildID(ctx, culpritModel)
	if err != nil {
		return "", err
	}
	analysisURL := util.ConstructAnalysisURL(ctx, bbid)
	buildURL := util.ConstructBuildURL(ctx, bbid)

	paragraphs = append(paragraphs, fmt.Sprintf("Reason for revert:\n"+
		"LUCI Bisection identified this CL as the culprit of a build failure."+
		" See the analysis: %s", analysisURL))

	paragraphs = append(paragraphs, fmt.Sprintf("Sample failed build: %s",
		buildURL))

	paragraphs = append(paragraphs,
		fmt.Sprintf("If this is a false positive, please report it at %s",
			util.ConstructLUCIBisectionBugURL(ctx, analysisURL, culpritModel.ReviewUrl)))

	// Set CQ flags in the last paragraph, i.e. footer of the CL description
	//
	// TODO (aredulla): also specify the revert's bug in this footer to be the
	// same as the culprit's bug
	paragraphs = append(paragraphs,
		"No-Presubmit: true\nNo-Tree-Checks: true\nNo-Try: true")

	return strings.Join(paragraphs, "\n\n"), nil
}
