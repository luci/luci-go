// Copyright 2023 The LUCI Authors.
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

package postaction

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	apipb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/run"
)

func TestIsTriggeringConditionMet(t *testing.T) {
	t.Parallel()

	ftt.Run("IsTriggeringConditionMet", t, func(t *ftt.Test) {
		pa := &cfgpb.ConfigGroup_PostAction{Name: "action q"}
		addCond := func(m string, sts ...apipb.Run_Status) {
			pa.Conditions = append(pa.Conditions, &cfgpb.ConfigGroup_PostAction_TriggeringCondition{
				Mode:     m,
				Statuses: sts,
			})
		}
		r := &run.Run{}
		shouldMeet := func(m run.Mode, st run.Status) {
			r.Mode, r.Status = m, st
			assert.Loosely(t, IsTriggeringConditionMet(pa, r), should.BeTrue)
		}
		shouldNotMeet := func(m run.Mode, st run.Status) {
			r.Mode, r.Status = m, st
			assert.Loosely(t, IsTriggeringConditionMet(pa, r), should.BeFalse)
		}
		addCond("DRY_RUN", apipb.Run_SUCCEEDED, apipb.Run_CANCELLED)
		addCond("FULL_RUN", apipb.Run_SUCCEEDED, apipb.Run_FAILED)
		addCond("CUSTOM_RUN", apipb.Run_SUCCEEDED)

		t.Run("dry_run mode", func(t *ftt.Test) {
			shouldMeet(run.DryRun, run.Status_SUCCEEDED)
			shouldMeet(run.DryRun, run.Status_CANCELLED)
			shouldNotMeet(run.DryRun, run.Status_FAILED)
		})
		t.Run("full_run mode", func(t *ftt.Test) {
			shouldMeet(run.FullRun, run.Status_SUCCEEDED)
			shouldNotMeet(run.FullRun, run.Status_CANCELLED)
			shouldMeet(run.FullRun, run.Status_FAILED)
		})
		t.Run("custom_run mode", func(t *ftt.Test) {
			CustomRun := run.Mode("CUSTOM_RUN")
			shouldMeet(CustomRun, run.Status_SUCCEEDED)
			shouldNotMeet(CustomRun, run.Status_CANCELLED)
			shouldNotMeet(CustomRun, run.Status_FAILED)
		})
		t.Run("a not matched run mode", func(t *ftt.Test) {
			MyRun := run.Mode("MY_RUN")
			shouldNotMeet(MyRun, run.Status_SUCCEEDED)
			shouldNotMeet(MyRun, run.Status_CANCELLED)
			shouldNotMeet(MyRun, run.Status_FAILED)
		})
	})
}
