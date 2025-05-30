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
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// isCulpritRevertible returns:
//   - whether a revert should be created for the culprit CL;
//   - the reason it should not be created if applicable; and
//   - the error if one occurred.
func isCulpritRevertible(ctx context.Context, gerritClient *gerrit.Client,
	culprit *gerritpb.ChangeInfo, culpritModel *model.Suspect, project string) (bool, string, error) {
	// We only create revert if it belongs to a builder being watched
	// by sheriffs.
	if culpritModel.AnalysisType == bisectionpb.AnalysisType_TEST_FAILURE_ANALYSIS {
		tfa, err := datastoreutil.GetTestFailureAnalysisForSuspect(ctx, culpritModel)
		if err != nil {
			return false, "", errors.Fmt("get test failure analysis for suspect: %w", err)
		}
		if len(tfa.SheriffRotations) == 0 {
			return false, "the builder that this CL broke is not watched by gardeners, therefore less important. You can consider revert this CL, fix forward or let builder owners resolve it themselves", nil
		}
	} else if culpritModel.AnalysisType == bisectionpb.AnalysisType_COMPILE_FAILURE_ANALYSIS {
		buildID, err := datastoreutil.GetBuildIDForCompileSuspect(ctx, culpritModel)
		if err != nil {
			return false, "", errors.Fmt("get build id for suspect: %w", err)
		}
		build, err := datastoreutil.GetBuild(ctx, buildID)
		if err != nil {
			return false, "", errors.Fmt("get build: %w", err)
		}
		if build == nil {
			return false, "", errors.Fmt("no build found: %d", buildID)
		}
		if len(build.SheriffRotations) == 0 {
			return false, "the builder that this CL broke is not watched by gardeners, therefore less important. You can consider revert this CL, fix forward or let builder owners resolve it themselves", nil
		}
	}

	// Check if the culprit's description has disabled autorevert
	hasFlag, err := gerrit.HasAutoRevertOffFlagSet(ctx, culprit)
	if err != nil {
		return false, "", errors.Fmt("error checking for auto-revert flag: %w", err)
	}
	if hasFlag {
		return false, "auto-revert has been disabled for this CL by its description", nil
	}

	// Check if the author of the culprit is irrevertible
	cannotRevert, err := HasIrrevertibleAuthor(ctx, culprit)
	if err != nil {
		return false, "", errors.Fmt("error getting culprit's commit author: %w", err)
	}
	if cannotRevert {
		return false, "LUCI Bisection cannot revert changes from this CL's author", nil
	}

	// Check if there are other merged changes depending on the culprit
	hasDep, err := gerritClient.HasDependency(ctx, culprit)
	if err != nil {
		return false, "", errors.Fmt("error checking for dependencies: %w", err)
	}
	if hasDep {
		return false, "there are merged changes depending on it", nil
	}
	// Check if LUCI Bisection's Gerrit config allows revert creation
	gerritCfg, err := config.GetGerritCfgForSuspect(ctx, culpritModel, project)
	if err != nil {
		return false, "", errors.Fmt("error fetching configs: %w", err)
	}
	canCreate, reason, err := config.CanCreateRevert(ctx, gerritCfg, culpritModel.AnalysisType)
	if err != nil {
		return false, "", errors.Fmt("error checking Create Revert configs: %w", err)
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

	var message string
	var err error
	switch culpritModel.AnalysisType {
	case bisectionpb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		message, err = compileFailureComment(ctx, culpritModel, "", "blameComment")
		if err != nil {
			return "", errors.Fmt("compile failure comment: %w", err)
		}
	case bisectionpb.AnalysisType_TEST_FAILURE_ANALYSIS:
		message, err = testFailureComment(ctx, culpritModel, "", "blameComment")
		if err != nil {
			return "", errors.Fmt("test failure comment: %w", err)
		}
	}
	message = "Reason for revert:\n" + message
	paragraphs = append(paragraphs, message)

	// Lines in the footer of the description, such as related bugs
	footerLines := []string{}

	// Add the culprit's description, with each line prefixed with "> "; this is
	// skipped if the description for the culprit is not available.
	culpritDescription, err := gerrit.CommitMessage(ctx, culprit)
	if err == nil {
		bugPattern := regexp.MustCompile(`(?i)^bug\s*[:=]`)
		prefixedLines := []string{"Original change's description:"}
		for _, line := range strings.Split(culpritDescription, "\n") {
			// Check if the line specifies related bugs, so it can be used in the
			// footer of the revert
			if bugPattern.MatchString(line) {
				footerLines = append(footerLines, line)
			}

			prefixedLines = append(prefixedLines,
				strings.TrimSpace(fmt.Sprintf("> %s", line)))
		}
		paragraphs = append(paragraphs, strings.Join(prefixedLines, "\n"))
	}

	// Set CQ flags in the last paragraph, i.e. footer of the CL description
	cqFlags := []string{
		"No-Presubmit: true",
		"No-Tree-Checks: true",
		"No-Try: true",
	}
	footerLines = append(footerLines, cqFlags...)
	paragraphs = append(paragraphs, strings.Join(footerLines, "\n"))

	return strings.Join(paragraphs, "\n\n"), nil
}
