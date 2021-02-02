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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRecursivePoke(t *testing.T) {
	t.Parallel()

	Convey("Poke", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const runID = "chromium/222-1-deadbeef"
		So(datastore.Put(ctx, &run.Run{
			ID:       runID,
			Status:   run.Status_RUNNING,
			EVersion: 10,
		}), ShouldBeNil)

		Convey("Recursive", func() {
			So(run.PokeNow(ctx, runID), ShouldBeNil)
			So(runtest.Runs(ct.TQ.Tasks()), ShouldResemble, common.RunIDs{runID})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))
			for i := 0; i < 10; i++ {
				now := clock.Now(ctx)
				runtest.AssertInEventbox(ctx, runID, &eventpb.Event{
					Event: &eventpb.Event_Poke{
						Poke: &eventpb.Poke{},
					},
					ProcessAfter: timestamppb.New(now.Add(pokeInterval)),
				})
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))
			}
		})

		Convey("Existing event due during the interval", func() {
			So(run.PokeNow(ctx, runID), ShouldBeNil)
			So(run.Poke(ctx, runID, 30*time.Second), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))

			runtest.AssertNotInEventbox(ctx, runID, &eventpb.Event{
				Event: &eventpb.Event_Poke{
					Poke: &eventpb.Poke{},
				},
				ProcessAfter: timestamppb.New(clock.Now(ctx).Add(pokeInterval)),
			})
			So(runtest.Tasks(ct.TQ.Tasks()), ShouldHaveLength, 1)
			task := runtest.Tasks(ct.TQ.Tasks())[0]
			So(task.ETA, ShouldResemble, clock.Now(ctx).UTC().Add(30*time.Second))
			So(task.Payload, ShouldResembleProto, &eventpb.PokeRunTask{RunId: string(runID)})
		})
	})
}
