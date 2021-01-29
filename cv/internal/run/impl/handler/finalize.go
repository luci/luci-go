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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnFinished finalizes the Run using the finished Run reported by CQDaemon.
func (*Impl) OnFinished(ctx context.Context, s *state.RunState) (eventbox.SideEffectFn, *state.RunState, error) {
	switch status := s.Run.Status; {
	case status == run.Status_RUNNING:
		s = s.ShallowCopy()
		s.Run.Status = run.Status_FINALIZING
		se := eventbox.Chain(s.RefreshCLs, func(ctx context.Context) error {
			// Revisit after 1 minute.
			// Assuming all CLs MUST be up-to-date by then.
			return run.NotifyFinished(ctx, s.Run.ID, 1*time.Minute)
		})
		return se, s, nil
	case status == run.Status_FINALIZING || run.IsEnded(status):
		s = s.ShallowCopy()
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
		return s.RemoveRunFromCLs, s, nil
	default:
		panic(fmt.Errorf("unexpected run status: %s", status))
	}
}
