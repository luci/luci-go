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
	"testing"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFinalize(t *testing.T) {
	t.Parallel()

	Convey("Finalize", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		h := &Impl{}
		rs := &state.RunState{
			Run: run.Run{
				ID:  common.RunID("chromium/111-1-beef"),
				CLs: []common.CLID{1},
			},
		}

		Convey("Changes the status to FINALIZING", func() {
			rs.Run.Status = run.Status_RUNNING
			sideEffectFn, newrs, err := h.Finalize(ctx, rs)
			So(err, ShouldBeNil)
			So(newrs.Run.Status, ShouldEqual, run.Status_FINALIZING)
			So(sideEffectFn(ctx), ShouldBeNil)

			runtest.AssertInEventbox(ctx, newrs.Run.ID, &eventpb.Event{
				Event: &eventpb.Event_Finalize{
					Finalize: &eventpb.Finalize{},
				},
			})
		})
	})
}
