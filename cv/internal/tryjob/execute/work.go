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

package execute

import (
	"context"
	"sort"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func (e *Executor) startTryjobs(ctx context.Context, r *run.Run, definitions []*tryjob.Definition, executions []*tryjob.ExecutionState_Execution) ([]*tryjob.Tryjob, error) {
	cls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	w := &worker{
		backend:          e.Backend,
		rm:               e.RM,
		run:              r,
		cls:              cls,
		knownTryjobIDs:   make(common.TryjobIDSet),
		knownExternalIDs: make(stringset.Set),
		reuseKey:         computeReuseKey(cls),
		clPatchsets:      make(tryjob.CLPatchsets, len(cls)),
	}
	for _, execution := range executions {
		for _, attempt := range execution.GetAttempts() {
			if tjID := common.TryjobID(attempt.GetTryjobId()); tjID != 0 {
				w.knownTryjobIDs.Add(tjID)
			}
			if eid := attempt.GetExternalId(); eid != "" {
				w.knownExternalIDs.Add(eid)
			}
		}
	}
	for i, cl := range cls {
		w.clPatchsets[i] = tryjob.MakeCLPatchset(cl.ID, cl.Detail.GetPatchset())
	}
	sort.Sort(w.clPatchsets)

	return w.start(ctx, definitions)
}

// worker implements the workflow to trigger tryjobs for the given definitions
// by searching for Tryjobs that can be reused first and then launch new
// Tryjobs if nothing can be reused.
type worker struct {
	run              *run.Run
	cls              []*run.RunCL
	knownTryjobIDs   common.TryjobIDSet
	knownExternalIDs stringset.Set

	reuseKey    string
	clPatchsets tryjob.CLPatchsets
	backend     TryjobBackend
	rm          rm
}

func (w *worker) start(ctx context.Context, definitions []*tryjob.Definition) ([]*tryjob.Tryjob, error) {
	panic("implement")
}
