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

package postaction

import (
	"context"
	"fmt"
	"sort"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/quota"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

// CreditRunQuotaPostActionName is the name of the internal post action that
// credits the run quota after the provided run is ended.
const CreditRunQuotaPostActionName = "credit-run-quota"

func (exe *Executor) creditQuota(ctx context.Context) (string, error) {
	// Credit back run quota when the run ends.
	switch quotaOp, err := exe.QM.CreditRunQuota(ctx, exe.Run); {
	case err == nil && quotaOp != nil:
		logging.Debugf(ctx, "Run quota credited back to %q; new balance: %d", exe.Run.CreatedBy.Email(), quotaOp.GetNewBalance())
		// continue to pick the next pending run to start for the run creator
	case err == nil:
		// no quota operation executed means no run quota limit has been specified
		// for the user.
		return fmt.Sprintf("run quota limit is not specified for user %q", exe.Run.CreatedBy.Email()), nil
	case err == quota.ErrQuotaApply:
		return "", errors.Annotate(err, "QM.CreditRunQuota: unexpected quotaOp Status %s", quotaOp.GetStatus()).Tag(transient.Tag).Err()
	case err != nil:
		return "", errors.Annotate(err, "QM.CreditRunQuota").Tag(transient.Tag).Err()
	}

	switch nextRun, err := exe.pickNextRunToStart(ctx); {
	case err != nil:
		return "", err
	case nextRun != nil:
		if err := exe.RM.Start(ctx, nextRun.ID); err != nil {
			return "", errors.Annotate(err, "failed to notify run %q to start", nextRun.ID).Tag(transient.Tag).Err()
		}
		return fmt.Sprintf("notified next Run %q to start", nextRun.ID), nil
	}
	return "", nil
}

// pickNextRunToStart search against all eligible pending runs and try to
// find the earliest one.
func (exe *Executor) pickNextRunToStart(ctx context.Context) (*run.Run, error) {
	curRun := exe.Run
	account := exe.QM.RunQuotaAccountID(curRun)
	var tok *runquery.PageToken
	var candidates []*run.Run
	var knownPendingRuns common.RunIDs
	for {
		// process 25 runs at a time.
		qb := runquery.ProjectQueryBuilder{
			Project: curRun.ID.LUCIProject(),
			Status:  run.Status_PENDING,
			Limit:   25,
		}.PageToken(tok)

		runs, token, err := qb.LoadRuns(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			knownPendingRuns = append(knownPendingRuns, r.ID)
			if proto.Equal(exe.QM.RunQuotaAccountID(r), account) {
				// only consider the runs that belong to the same quota account
				candidates = append(candidates, r)
			}
		}
		if token == nil {
			break
		}
		tok = token
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Sort(knownPendingRuns)
	// Sort the candidates chronically from earliest to latest and return the
	// earliest run that has not pending dep runs.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreateTime.Before(candidates[j].CreateTime)
	})
	for _, candidate := range candidates {
		switch hasNoPendingDepRun, err := hasNoPendingDepRun(ctx, candidate, knownPendingRuns); {
		case err != nil:
			return nil, err
		case hasNoPendingDepRun:
			return candidate, nil
		}
	}
	return nil, nil
}

// hasNoPendingDepRun returns true the provided run has *no* dep Run that is in
// PENDING status.
func hasNoPendingDepRun(ctx context.Context, r *run.Run, knownPendingRuns common.RunIDs) (bool, error) {
	if len(r.DepRuns) == 0 { // no deps at all
		return true, nil
	}
	depRuns := make([]*run.Run, len(r.DepRuns))
	for i, depRunID := range r.DepRuns {
		if knownPendingRuns.ContainsSorted(depRunID) {
			return false, nil
		}
		depRuns[i] = &run.Run{ID: depRunID}
	}
	if err := datastore.Get(ctx, depRuns); err != nil {
		return false, errors.Annotate(err, "failed to load depRuns %s", r.DepRuns).Tag(transient.Tag).Err()
	}
	for _, depRun := range depRuns {
		switch depRun.Status {
		case run.Status_STATUS_UNSPECIFIED:
			panic(fmt.Errorf("the status of run %q is not specified", depRun.ID))
		case run.Status_PENDING:
			return false, nil
		}
	}
	return true, nil
}
