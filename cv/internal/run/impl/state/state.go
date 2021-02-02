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

// Package state defines the model for a Run state.
package state

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	gobupdater "go.chromium.org/luci/cv/internal/gerrit/updater"
	"go.chromium.org/luci/cv/internal/run"
)

// RunState represents the current state of a Run.
//
// It consists of the Run entity and its child entities (could be partial
// depending on the event received).
type RunState struct {
	Run run.Run
	// TODO(yiwzhang): add RunOwner, []RunCL, []RunTryjob.
}

// ShallowCopy returns a shallow copy of run state
func (rs *RunState) ShallowCopy() *RunState {
	if rs == nil {
		return nil
	}
	ret := &RunState{
		Run: rs.Run,
	}
	return ret
}

// loadCLs loads all CL entities involved in the Run.
//
// Return nil error iff all CLs are successfully loaded.
func (rs *RunState) loadCLs(ctx context.Context) ([]*changelist.CL, error) {
	cls := make([]*changelist.CL, len(rs.Run.CLs))
	for i, clID := range rs.Run.CLs {
		cls[i] = &changelist.CL{ID: clID}
	}
	err := changelist.LoadMulti(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return cls, nil
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("CL %d not found in Datastore", cls[i].ID).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d CLs", count, len(cls)).Tag(transient.Tag).Err()
	default:
		return nil, errors.Annotate(err, "failed to load %d CLs", len(cls)).Tag(transient.Tag).Err()
	}
}

// RefreshCLs submits tasks for refresh all CLs involved in this Run.
func (rs *RunState) RefreshCLs(ctx context.Context) error {
	cls, err := rs.loadCLs(ctx)
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
					LuciProject: rs.Run.ID.LUCIProject(),
					Host:        host,
					Change:      change,
					ClidHint:    int64(cl.ID),
				})
			}
		}
	})
	return common.MostSevereError(err)
}

// RemoveRunFromCLs removes the Run from the IncompleteRuns list of all
// CL entities associated with this Run.
func (rs *RunState) RemoveRunFromCLs(ctx context.Context) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in a transaction")
	}
	cls, err := rs.loadCLs(ctx)
	if err != nil {
		return err
	}
	changedCLs := cls[:0]
	for _, cl := range cls {
		changed := cl.Mutate(ctx, func(cl *changelist.CL) bool {
			return cl.IncompleteRuns.DelSorted(rs.Run.ID)
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
