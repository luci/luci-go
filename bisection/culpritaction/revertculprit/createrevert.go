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

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
)

var (
	// errHasDependency is returned when attempting to create a revert for
	// a culprit but that culprit has a merged dependency
	errHasDependency = errors.New("createRevert: culprit has a merged dependency")
	// errAlreadyReverted is returned when attempting to create a revert for
	// a culprit but that culprit has already been reverted
	errAlreadyReverted = errors.New("createRevert: culprit has already been reverted")
	// errHasRevert is returned when attempting to create a revert for
	// a culprit but that culprit has an existing revert
	errHasRevert = errors.New("createRevert: culprit has an existing revert")
)

// createRevert attempts to create a revert for the given culprit.
// Returns a revert for the culprit (may or may not have been created by
// LUCI Bisection).
// Note: this should only be called according to the service-wide configuration
// data for LUCI Bisection, i.e.
//   - Gerrit actions are enabled
//   - Creating reverts is enabled
//   - the daily limit of created reverts has not yet been reached
func createRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, culprit *gerritpb.ChangeInfo) (*gerritpb.ChangeInfo, error) {
	// Check if there's an existing revert
	reverts, err := gerritClient.GetReverts(ctx, culprit)
	if err != nil {
		return nil, err
	}

	for _, revert := range reverts {
		switch revert.Status {
		case gerritpb.ChangeStatus_MERGED:
			return revert, errAlreadyReverted
		case gerritpb.ChangeStatus_NEW:
			return revert, errHasRevert
		default:
			continue
		}
	}

	// If here, then either there are no existing reverts for the culprit, or
	// they are all abandoned reverts

	// Check if there are other merged changes depending on the culprit
	// before trying to revert it
	hasDep, err := gerritClient.HasDependency(ctx, culprit)
	if err != nil {
		return nil, err
	}
	if hasDep {
		return nil, errHasDependency
	}

	revertDescription, err := generateRevertDescription(ctx, culpritModel, culprit)
	if err != nil {
		return nil, err
	}

	// Create the revert
	revert, err := gerritClient.CreateRevert(ctx, culprit, revertDescription)
	if err != nil {
		return nil, err
	}

	// Update revert details for creation
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, culpritModel)
		if e != nil {
			return e
		}

		culpritModel.RevertURL = fmt.Sprintf("https://%s/c/%s/+/%d",
			gerritClient.Host(ctx), revert.Project, revert.Number)
		culpritModel.IsRevertCreated = true
		culpritModel.RevertCreateTime = clock.Now(ctx)

		return datastore.Put(ctx, culpritModel)
	}, nil)
	if err != nil {
		return revert, errors.Annotate(err,
			"couldn't update suspect revert creation details").Err()
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

	// TODO (aredulla): add link to file bug for LUCI Bisection if it's a
	// false positive

	// Set CQ flags in the last paragraph, i.e. footer of the CL description
	//
	// TODO (aredulla): also specify the revert's bug in this footer to be the
	// same as the culprit's bug
	paragraphs = append(paragraphs,
		"No-Presubmit: true\nNo-Tree-Checks: true\nNo-Try: true")

	return strings.Join(paragraphs, "\n\n"), nil
}
