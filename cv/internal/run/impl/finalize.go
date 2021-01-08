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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/gae/service/datastore"
)

// RemoveRunFromCLs removes the Run from the IncompleteRuns list of all
// CL entities associated with this Run.
//
// TODO(yiwzhang): Stop exporting after CV is able to finalize Runs itself.
func RemoveRunFromCLs(ctx context.Context, r *run.Run) error {
	cls := make([]*changelist.CL, len(r.CLs))
	for i, clid := range r.CLs {
		cls[i] = &changelist.CL{ID: clid}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		return errors.Annotate(err, "failed to fetch CLs").Tag(transient.Tag).Err()
	}
	changedCLs := cls[:0]
	for _, cl := range cls {
		changed := cl.Mutate(ctx, func(cl *changelist.CL) bool {
			return cl.IncompleteRuns.DelSorted(r.ID)
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
