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
	"go.chromium.org/luci/cv/internal/common"
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
			// Revisit after 1 minute.
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
	cls, err := s.loadCLs(ctx)
	if err != nil {
		return err
	}

	poolSize := len(cls)
	if poolSize > 20 {
		poolSize = 20
	}
	err = parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for i := range cls {
			cl := cls[i]
			work <- func() error {
				host, change, err := cl.ExternalID.ParseGobID()
				if err != nil {
					return err
				}
				return gobupdater.Schedule(ctx, &gobupdater.RefreshGerritCL{
					LuciProject: s.Run.ID.LUCIProject(),
					Host:        host,
					Change:      change,
					ClidHint:    int64(cl.ID),
				})
			}
		}
	})
	return common.MostSevereError(err)
}

// removeRunFromCLs removes the Run from the IncompleteRuns list of all
// CL entities associated with this Run.
func (s *state) removeRunFromCLs(ctx context.Context) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in a transaction")
	}
	cls, err := s.loadCLs(ctx)
	if err != nil {
		return err
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
