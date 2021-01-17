// Copyright 2020 The LUCI Authors.
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

package state

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
)

// reevalPCLs re-evaluates PCLs after a project config change.
//
// If components have to be re-evaluated, only marks PB.DirtyComponents.
func (s *State) reevalPCLs(ctx context.Context) error {
	_, _, err := s.loadCLsForPCLs(ctx)
	if err != nil {
		return err
	}
	// TODO(tandrii): implement.
	return nil
}

// loadCLsForPCLs loads CLs from Datastore corresponding to PCLs.
//
// Returns slice of CLs, error.MultiError slice corresponding to
// per-CL errors *(always exists and has same length as CLs)*, and a
// top level error if it can't be attributed to any CL.
//
// *each error in per-CL errors* is not annotated and is nil if CL was loaded
// successfully.
func (s *State) loadCLsForPCLs(ctx context.Context) ([]*changelist.CL, errors.MultiError, error) {
	cls := make([]*changelist.CL, len(s.PB.GetPcls()))
	for i, pcl := range s.PB.GetPcls() {
		cls[i] = &changelist.CL{ID: common.CLID(pcl.GetClid())}
	}

	// At 0.007 KiB (serialized) per CL as of Jan 2021, this should scale 2000 CLs
	// with reasonable RAM footprint and well within 10s because
	// changelist.LoadMulti splits it into concurrently queried batches.
	// To support more, CLs would need to be loaded and processed in batches,
	// or CL snapshot size reduced.
	err := changelist.LoadMulti(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return cls, make(errors.MultiError, len(cls)), nil
	case ok:
		return cls, merr, nil
	default:
		return nil, nil, errors.Annotate(err, "failed to load %d CLs", len(cls)).Tag(transient.Tag).Err()
	}
}
