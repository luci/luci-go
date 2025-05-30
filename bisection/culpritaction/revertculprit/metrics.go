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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/bisection/metrics"
	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

var (
	culpritActionCounter = metric.NewCounter(
		"bisection/culprit_action/action_taken",
		"The number of actions that LUCI Bisection took against culprits",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "compile" or "test"
		field.String("failure_type"),
		// "create_revert", "submit_revert", "comment_culprit", "comment_revert"
		field.String("action_type"),
	)
)

type ActionType string

const (
	// LUCI Bisection created a revert for this culprit
	ActionTypeCreateRevert ActionType = "create_revert"
	// LUCI Bisection submitted a revert for this culprit
	ActionTypeSubmitRevert = "submit_revert"
	// LUCI Bisection commented on the culprit
	ActionTypeCommentCulprit = "comment_culprit"
	// LUCI Bisection commented on a revert for this culprit
	ActionTypeCommentRevert = "comment_revert"
)

func updateCulpritActionCounter(c context.Context, suspect *model.Suspect, actionType ActionType) error {
	var failureType string
	var project string
	switch suspect.AnalysisType {
	case bisectionpb.AnalysisType_COMPILE_FAILURE_ANALYSIS:
		bbid, err := datastoreutil.GetAssociatedBuildID(c, suspect)
		if err != nil {
			return errors.Fmt("GetAssociatedBuildID: %w", err)
		}
		build, err := datastoreutil.GetBuild(c, bbid)
		if err != nil {
			return errors.Fmt("getting build %d: %w", bbid, err)
		}
		if build == nil {
			return fmt.Errorf("no build %d", bbid)
		}
		project = build.Project
		failureType = string(metrics.AnalysisTypeCompile)
	case bisectionpb.AnalysisType_TEST_FAILURE_ANALYSIS:
		tfa, err := datastoreutil.GetTestFailureAnalysisForSuspect(c, suspect)
		if err != nil {
			return err
		}
		project = tfa.Project
		failureType = string(metrics.AnalysisTypeTest)
	default:
		logging.Errorf(c, "unknown analysis type of suspect %s", suspect.AnalysisType.String())
		return nil
	}
	culpritActionCounter.Add(c, 1, project, failureType, string(actionType))
	return nil
}
