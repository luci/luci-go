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

package config

import (
	"context"
	"fmt"

	configpb "go.chromium.org/luci/bisection/proto/config"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// CanCreateRevert returns:
//   - whether a revert can be created;
//   - the reason it cannot be created if applicable; and
//   - the error if one occurred.
func CanCreateRevert(ctx context.Context, gerritCfg *configpb.GerritConfig) (bool, string, error) {
	// Check if Gerrit actions are enabled
	if !gerritCfg.ActionsEnabled {
		reason := "all Gerrit actions are disabled"
		return false, reason, nil
	}

	// Check if revert creation is enabled
	if !gerritCfg.CreateRevertSettings.Enabled {
		reason := "LUCI Bisection's revert creation has been disabled"
		return false, reason, nil
	}

	// Check the daily limit for revert creations has not been reached
	createdCount, err := datastoreutil.CountLatestRevertsCreated(ctx, 24)
	if err != nil {
		return false, "", err
	}
	if createdCount >= int64(gerritCfg.CreateRevertSettings.DailyLimit) {
		// revert creation daily limit has been reached
		reason := fmt.Sprintf("LUCI Bisection's daily limit for revert creation"+
			" (%d) has been reached; %d reverts have already been created",
			gerritCfg.CreateRevertSettings.DailyLimit, createdCount)
		return false, reason, nil
	}

	return true, "", nil
}

// CanSubmitRevert returns:
//   - whether a revert can be submitted;
//   - the reason it cannot be submitted if applicable; and
//   - the error if one occurred.
func CanSubmitRevert(ctx context.Context, gerritCfg *configpb.GerritConfig) (bool, string, error) {
	// Check if Gerrit actions are enabled
	if !gerritCfg.ActionsEnabled {
		reason := "all Gerrit actions are disabled"
		return false, reason, nil
	}

	// Check if revert submission is enabled
	if !gerritCfg.SubmitRevertSettings.Enabled {
		reason := "LUCI Bisection's revert submission has been disabled"
		return false, reason, nil
	}

	// Check the daily limit for revert submissions has not been reached
	committedCount, err := datastoreutil.CountLatestRevertsCommitted(ctx, 24)
	if err != nil {
		return false, "", err
	}
	if committedCount >= int64(gerritCfg.SubmitRevertSettings.DailyLimit) {
		reason := fmt.Sprintf("LUCI Bisection's daily limit for revert submission"+
			" (%d) has been reached; %d reverts have already been submitted",
			gerritCfg.SubmitRevertSettings.DailyLimit, committedCount)
		return false, reason, nil
	}

	return true, "", nil
}
