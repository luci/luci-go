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

package handler

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Finalize finalizes a run.
func (*Impl) Finalize(ctx context.Context, s *state.RunState) (eventbox.SideEffectFn, *state.RunState, error) {
	switch status := s.Run.Status; {
	case status == run.Status_RUNNING:
		// First pass, switch the status to FINALIZING so that RunManager won't
		// abort the finalization in the middle when processing other events. This
		// is to ensure finalization is atomic although true atomic can't be
		// achieved because Gerrit itself isn't atomic.
		s = s.ShallowCopy()
		s.Run.Status = run.Status_FINALIZING
		return func(ctx context.Context) error {
			return run.Finalize(ctx, s.Run.ID)
		}, s, nil
	case status == run.Status_FINALIZING:
	case run.IsEnded(status):
		logging.Warningf(ctx, "requested to finalize after run %q has already been finalized", s.Run.ID)
		return nil, s, nil
	default:
		panic(fmt.Errorf("unexpected run status: %s", status))
	}
	// Second pass, do the actual work.
	// TODO(yiwzhang): implement
	//  1. Load the verified run reported by CQDaemon.
	//  2. If dryrun, cancel the trigger and post verification results.
	//  3. If fullRun & verification passed, sumbit the change
	//  4. If fullRUn & verification failed, cancel the trigger and post results.
	s = s.ShallowCopy()
	s.Run.Status = run.Status_SUCCEEDED
	return nil, s, nil
}
