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

package culpritaction

import (
	"context"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
)

// commitRevert attempts to bot-commit the given revert.
// Returns whether the revert was successfully committed.
// Note: this should only be called according to the service-wide configuration
// data for LUCI Bisection, i.e.
//   - Gerrit actions are enabled
//   - Submitting reverts is enabled
//   - the daily limit of submitted reverts has not yet been reached
//   - the culprit is not yet older than the maximum revertible culprit age
func commitRevert(ctx context.Context, gerritClient *gerrit.Client,
	culpritModel *model.Suspect, revert *gerritpb.ChangeInfo) (bool, error) {
	// TODO (aredulla): CC sheriffs on rotation
	_, err := gerritClient.CommitRevert(
		ctx,
		revert,
		"LUCI Bisection is automatically submitting this revert.",
		[]*gerritpb.AccountInfo{},
	)
	if err != nil {
		return false, err
	}

	// Update revert details for commit action
	culpritModel.IsRevertCommitted = true
	culpritModel.RevertCommitTime = clock.Now(ctx)
	err = datastore.Put(ctx, culpritModel)
	if err != nil {
		return true, errors.Annotate(err,
			"couldn't update suspect revert commit details").Err()
	}

	return true, nil
}
