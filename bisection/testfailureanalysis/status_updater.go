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

// Package testfailureanalysis handles test failure analysis.
package testfailureanalysis

import (
	"context"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
)

// UpdateAnalysisStatus updates status of a test failure analysis.
func UpdateAnalysisStatus(ctx context.Context, tfa *model.TestFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		e := datastore.Get(ctx, tfa)
		if e != nil {
			return e
		}

		// If the run has ended or canceled, we don't want to do anything.
		if tfa.RunStatus == pb.AnalysisRunStatus_ENDED || tfa.RunStatus == pb.AnalysisRunStatus_CANCELED {
			return nil
		}

		// All the same, no need to update.
		if tfa.RunStatus == runStatus && tfa.Status == status {
			return nil
		}

		tfa.Status = status
		tfa.RunStatus = runStatus
		if runStatus == pb.AnalysisRunStatus_ENDED || runStatus == pb.AnalysisRunStatus_CANCELED {
			tfa.EndTime = clock.Now(ctx)
		}
		if runStatus == pb.AnalysisRunStatus_STARTED {
			tfa.StartTime = clock.Now(ctx)
		}
		return datastore.Put(ctx, tfa)
	}, nil)
}
