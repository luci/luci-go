// Copyright 2025 The LUCI Authors.
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
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/lucinotify"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// NotifyRevertLanded notifies LUCI Notify about a culprit revert landing.
// LUCI Notify will then consider this event when automatically managing tree status,
// potentially reopening the tree if the revert landed after all failing builds started.
// It checks if the culprit is a tree closer before notifying.
func NotifyRevertLanded(ctx context.Context, treeName string, culpritModel *model.Suspect, revertURL string) error {
	// Check if this culprit is a tree closer
	isTreeCloser, err := isCulpritTreeCloser(ctx, treeName, culpritModel)
	if err != nil {
		// Log the error but don't fail the revert flow
		logging.Warningf(ctx, "Failed to check if culprit is tree closer: %v", err)
		return nil
	}

	if !isTreeCloser {
		logging.Infof(ctx, "Culprit is not a tree closer, skipping tree reopen notification")
		return nil
	}

	// Propagate actual revert creation/commit timestamp instead of synthetic current time
	revertTime := time.Now()
	if !culpritModel.ActionDetails.RevertCommitTime.IsZero() {
		revertTime = culpritModel.ActionDetails.RevertCommitTime
	} else if !culpritModel.ActionDetails.RevertCreateTime.IsZero() {
		revertTime = culpritModel.ActionDetails.RevertCreateTime
	}

	// Culprit is a tree closer, notify LUCI Notify about the revert
	logging.Infof(ctx, "Culprit is a tree closer, notifying LUCI Notify about revert for tree %s at %v", treeName, revertTime)

	// Notify LUCI Notify that the revert has landed
	// LUCI Notify's tree status cron will consider this event and decide whether to reopen the tree
	// based on whether the revert landed after all currently-failing builds started.
	err = lucinotify.NotifyRevertLanded(ctx, treeName, revertTime, culpritModel.ReviewUrl, revertURL)
	if err != nil {
		return errors.Fmt("notifying LUCI Notify about revert for tree %s: %w", treeName, err)
	}

	logging.Infof(ctx, "Successfully notified LUCI Notify about revert for tree %s", treeName)
	return nil
}

// isCulpritTreeCloser checks if a suspect is associated with any tree closer analysis for treeName.
// It queries all suspects with the same commit hash and checks if any of their
// associated CompileFailureAnalysis has IsTreeCloser set to true for treeName.
func isCulpritTreeCloser(ctx context.Context, treeName string, culprit *model.Suspect) (bool, error) {
	if culprit.GitilesCommit.Id == "" {
		return false, nil
	}

	// Direct check if suspect's parent analysis is available
	if culprit.ParentAnalysis != nil && culprit.ParentAnalysis.Parent() != nil {
		analysisID := culprit.ParentAnalysis.Parent().IntID()
		cfa, err := datastoreutil.GetCompileFailureAnalysis(ctx, analysisID)
		if err != nil {
			logging.Warningf(ctx, "Failed to get CompileFailureAnalysis %d: %v", analysisID, err)
		} else if cfa != nil && cfa.IsTreeCloser && cfa.Project == treeName {
			return true, nil
		}
	}

	// Step 1: Query all suspects with the same commit hash
	// The GitilesCommit struct is embedded, so we access its Id field directly
	q := datastore.NewQuery("Suspect").Eq("Id", culprit.GitilesCommit.Id)
	suspects, err := datastore.RunQuery[*model.Suspect](ctx, q).AsSlice()
	if err != nil {
		return false, errors.Fmt("failed to query suspects with same commit ID: %w", err)
	}

	// Step 2: Collect unique CompileFailureAnalysis IDs
	analysisIDs := make(map[int64]bool)
	for _, suspect := range suspects {
		// Only consider compile failure suspects
		if suspect.AnalysisType != pb.AnalysisType_COMPILE_FAILURE_ANALYSIS {
			continue
		}
		// Match Gitiles commit host and project
		if suspect.GitilesCommit.Host != culprit.GitilesCommit.Host ||
			suspect.GitilesCommit.Project != culprit.GitilesCommit.Project {
			continue
		}
		// Get the CompileFailureAnalysis ID from the suspect's parent analysis
		if suspect.ParentAnalysis != nil && suspect.ParentAnalysis.Parent() != nil {
			analysisID := suspect.ParentAnalysis.Parent().IntID()
			analysisIDs[analysisID] = true
		}
	}

	// Step 3: Check if any CompileFailureAnalysis has IsTreeCloser set to true for treeName
	for analysisID := range analysisIDs {
		cfa, err := datastoreutil.GetCompileFailureAnalysis(ctx, analysisID)
		if err != nil {
			// Log the error but continue checking other analyses
			logging.Warningf(ctx, "Failed to get CompileFailureAnalysis %d: %v", analysisID, err)
			continue
		}
		if cfa.IsTreeCloser && cfa.Project == treeName {
			return true, nil
		}
	}

	return false, nil
}
