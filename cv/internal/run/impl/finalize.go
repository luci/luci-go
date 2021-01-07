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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/run"
)

func onFinalize(ctx context.Context, runID common.RunID, s *state) (se eventbox.SideEffectFn, ret *state, err error) {
	switch status := s.Run.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err = errors.Reason("CRITICAL: can't cancel a Run with unspecified status").Err()
		errors.Log(ctx, err)
		panic(err)
	case run.IsEnded(status):
		logging.Debugf(ctx, "can't finalize an already ended Run")
		return nil, s, nil
	}
	// TODO(yiwzhang): Check whether this Run is still qualified for finalization
	// again because it is possible that the Config used by this Run has been
	// changed, hence, new Tryjobs are requested.
	ret = s.shallowCopy()
	if err = migration.FinalizeRun(ctx, &ret.Run); err != nil {
		return
	}
	se = func(ctx context.Context) error {
		return prjmanager.NotifyRunFinished(ctx, runID)
	}
	return
}
