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

package impl

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run"
)

func start(ctx context.Context, runID common.RunID, s *state) (eventbox.SideEffectFn, *state, error) {
	switch status := s.Run.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: can't start a Run %q with unspecified status", runID).Err()
		common.LogError(ctx, err)
		panic(err)
	case status != run.Status_PENDING:
		logging.Debugf(ctx, "Skip starting Run %q because this Run is %s", runID, status)
		return nil, s, nil
	}

	ret := s.shallowCopy()
	ret.Run.Status = run.Status_RUNNING
	ret.Run.StartTime = clock.Now(ctx).UTC()
	return nil, ret, nil
}
