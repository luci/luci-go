// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/eventbox"
	gobupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
)

func onFinished(ctx context.Context, s *state) (eventbox.SideEffectFn, *state, error) {
	switch status := s.Run.Status; {
	case status == run.Status_RUNNING:
		s = s.shallowCopy()
		s.Run.Status = run.Status_FINALIZING
		se := eventbox.Chain(s.refreshCLs, func(ctx context.Context) error {
			// Revisit after 1 minutes.
			// Assuming all CLs MUST be up-to-date by then.
			return run.NotifyFinished(ctx, s.Run.ID, 1*time.Minute)
		})
		return se, s, nil
	case status == run.Status_FINALIZING || run.IsEnded(status):
		s = s.shallowCopy()
		rid := s.Run.ID
		mfr := &migration.FinishedRun{ID: rid}
		switch err := datastore.Get(ctx, mfr); {
		case err == datastore.ErrNoSuchEntity:
			return nil, nil, errors.Reason("Run %q received Finished event but no FinishedRun entity found", rid).Err()
		case err != nil:
			return nil, nil, errors.Annotate(err, "failed to load FinishedRun %q", rid).Tag(transient.Tag).Err()
		}
		s.Run.Status = mfr.Status
		s.Run.EndTime = mfr.EndTime
		return s.removeRunFromCLs, s, nil
	default:
		panic(fmt.Errorf("unexpected run status: %s", status))
	}
}

func (s *state) refreshCLs(ctx context.Context) error {
	poolSize := len(s.Run.CLs)
	if poolSize > 20 {
		poolSize = 20
	}
	return parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for _, clID := range s.Run.CLs {
			clID := clID
			work <- func() error {
				cl := changelist.CL{ID: clID}
				switch err := datastore.Get(ctx, &cl); {
				case err == datastore.ErrNoSuchEntity:
					return errors.Reason("CL %q not found in Datastore", clID).Err()
				case err != nil:
					return errors.Annotate(err, "failed to load CL %q", clID).Tag(transient.Tag).Err()
				}
				host, change, err := cl.ExternalID.ParseGobID()
				if err != nil {
					return err
				}
				err = gobupdater.Schedule(ctx, &gobupdater.RefreshGerritCL{
					LuciProject: s.Run.ID.LUCIProject(),
					Host:        host,
					Change:      change,
					ClidHint:    int64(clID),
				})
				return errors.Annotate(err, "failed to schedule a refresh for CL %q", clID).Tag(transient.Tag).Err()
			}
		}
	})
}

// removeRunFromCLs removes the Run from the IncompleteRuns list of all
// CL entities associated with this Run.
func (s *state) removeRunFromCLs(ctx context.Context) error {
	cls := make([]*changelist.CL, len(s.Run.CLs))
	for i, clid := range s.Run.CLs {
		cls[i] = &changelist.CL{ID: clid}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
	}
	changedCLs := cls[:0]
	for _, cl := range cls {
		changed := cl.Mutate(ctx, func(cl *changelist.CL) bool {
			return cl.IncompleteRuns.DelSorted(s.Run.ID)
		})
		if changed {
			changedCLs = append(changedCLs, cl)
		}
	}
	if err := datastore.Put(ctx, changedCLs); err != nil {
		return errors.Annotate(err, "failed to put CLs").Tag(transient.Tag).Err()
	}
	return nil
}
