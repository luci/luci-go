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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnCLUpdated(t *testing.T) {
	Convey("OnCLUpdated", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		s := &state{
			Run: run.Run{
				ID:        common.RunID("chromium/111-2-deadbeef"),
				StartTime: clock.Now(ctx),
				Status:    run.Status_RUNNING,
			},
		}
		Convey("Cancels Run on new Patchset", func() {
			err := datastore.Put(ctx,
				&changelist.CL{
					ID: 1,
					Snapshot: &changelist.Snapshot{
						MinEquivalentPatchset: 6,
					},
				},
				&run.RunCL{
					ID:  1,
					Run: datastore.MakeKey(ctx, run.RunKind, string(s.Run.ID)),
					Detail: &changelist.Snapshot{
						MinEquivalentPatchset: 5,
					},
				},
			)
			So(err, ShouldBeNil)

			_, rs, err := onCLUpdated(ctx, s, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(rs.Run.Status, ShouldEqual, run.Status_CANCELLED)
		})
		Convey("Cancels Run on removed trigger", func() {
			err := datastore.Put(ctx,
				&changelist.CL{
					ID: 1,
					Snapshot: &changelist.Snapshot{
						MinEquivalentPatchset: 5,
					},
					// No trigger available
				},
				&run.RunCL{
					ID:  1,
					Run: datastore.MakeKey(ctx, run.RunKind, string(s.Run.ID)),
					Detail: &changelist.Snapshot{
						MinEquivalentPatchset: 5,
					},
					Trigger: &run.Trigger{
						Mode:            string(run.FullRun),
						GerritAccountId: 123,
					},
				},
			)
			So(err, ShouldBeNil)

			_, rs, err := onCLUpdated(ctx, s, common.CLIDs{1})
			So(err, ShouldBeNil)
			So(rs.Run.Status, ShouldEqual, run.Status_CANCELLED)
		})

		statuses := []run.Status{
			run.Status_FINALIZING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}

		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				s.Run.Status = status
				se, rs, err := onCLUpdated(ctx, s, common.CLIDs{1})
				So(err, ShouldBeNil)
				So(rs, ShouldEqual, s)
				So(se, ShouldBeNil)
			})
		}
	})
}
