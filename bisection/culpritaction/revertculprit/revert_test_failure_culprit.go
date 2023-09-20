// Copyright 2023 The LUCI Authors.
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

	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

func processTestFailureCulpritTask(ctx context.Context, analysisID int64) error {
	ctx = loggingutil.SetAnalysisID(ctx, analysisID)
	logging.Infof(ctx, "Processing test failure culprit action")

	// Get culprit model.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Annotate(err, "get test failure analysis").Err()
	}
	culpritKey := tfa.VerifiedCulpritKey
	culpritModel, err := datastoreutil.GetSuspect(ctx, culpritKey.IntID(), culpritKey.Parent())
	if err != nil {
		return errors.Annotate(err, "get suspect").Err()
	}
	if err := TakeCulpritAction(ctx, culpritModel); err != nil {
		return errors.Annotate(err, "revert culprit suspect_id %d", culpritModel.Id).Err()
	}
	return nil
}
