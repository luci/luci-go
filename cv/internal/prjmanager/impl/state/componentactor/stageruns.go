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

package componentactor

import (
	"context"
	"time"

	"go.chromium.org/luci/cv/internal/config"
)

// stageNewRuns sets .runBuilders if new Runs have to be created.
//
// Otherwise, returns the time when this could be done.
// NOTE: because this function may take considerable time, the returned time may
// be already in the past.
//
// Guarantees that a CL can be in at most 1 RunBuilder. This avoids conflicts
// whereby 2+ Runs need to modify the same CL entity.
func (a *Actor) stageNewRuns(ctx context.Context) (time.Time, error) {
	a.visitedCLs = map[int64]struct{}{}
	defer func() { a.visitedCLs = nil }() // won't be useful afterwards.

	var t time.Time
	for clid, info := range a.cls {
		switch nt, err := a.stageNewRunsFrom(ctx, clid, info); {
		case err != nil:
			return t, err
		case !nt.IsZero() && (t.IsZero() || nt.Before(t)):
			t = nt
		}
	}
	return t, nil
}

func (a *Actor) stageNewRunsFrom(ctx context.Context, clid int64, info *clInfo) (time.Time, error) {
	if !a.markVisited(clid) || !info.ready {
		return time.Time{}, nil
	}
	cgIndex := info.pcl.GetConfigGroupIndexes()[0]
	cg := a.s.ConfigGroup(cgIndex)
	if cg.Content.GetCombineCls() == nil {
		return a.stageNewRunsSingle(ctx, info, cg)
	}
	return a.stageNewRunsCombo(ctx, info, cg)
}

func (a *Actor) stageNewRunsSingle(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	//TODO(tandrii): implement
	return time.Time{}, nil
}

func (a *Actor) stageNewRunsCombo(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	//TODO(tandrii): implement
	return time.Time{}, nil
}

// markVisited makes CL visited if not already and returns if action was taken.
func (a *Actor) markVisited(clid int64) bool {
	if _, visited := a.visitedCLs[clid]; visited {
		return false
	}
	a.visitedCLs[clid] = struct{}{}
	return true
}
