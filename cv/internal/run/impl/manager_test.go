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
	"fmt"
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

func TestStartRun(t *testing.T) {
	t.Parallel()

	Convey("StartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const runID = "chromium/1111111111111-deadbeef"
		initialEversion := 10

		Convey("Starts when Run is PENDING", func() {
			err := datastore.Put(ctx, &run.Run{
				ID:       runID,
				Status:   run.Status_PENDING,
				EVersion: initialEversion,
			})
			So(err, ShouldBeNil)
			So(run.Start(ctx, runID), ShouldBeNil)
			startEvent := &eventpb.Event{
				Event: &eventpb.Event_Start{
					Start: &eventpb.Start{},
				},
			}
			runtest.AssertInEventbox(ctx, runID, startEvent)
			So(runtest.Runs(ct.TQ.Tasks()), ShouldResemble, common.RunIDs{runID})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))
			runtest.AssertNotInEventbox(ctx, runID, startEvent)

			r := run.Run{ID: runID}
			So(datastore.Get(ctx, &r), ShouldBeNil)
			So(r, ShouldResemble, run.Run{
				ID:         runID,
				Status:     run.Status_RUNNING,
				EVersion:   initialEversion + 1,
				StartTime:  datastore.RoundTime(ct.Clock.Now().UTC()),
				UpdateTime: datastore.RoundTime(ct.Clock.Now().UTC()),
			})
		})

		Convey("Panic when Run Status is not specified", func() {
			err := datastore.Put(ctx, &run.Run{
				ID:       runID,
				EVersion: initialEversion,
			})
			So(err, ShouldBeNil)
			So(run.Start(ctx, runID), ShouldBeNil)
			So(func() { ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run")) }, ShouldPanic)
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_FINALIZING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				err := datastore.Put(ctx, &run.Run{
					ID:       runID,
					Status:   status,
					EVersion: initialEversion,
				})
				So(err, ShouldBeNil)
				So(run.Start(ctx, runID), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))

				r := run.Run{ID: runID}
				So(datastore.Get(ctx, &r), ShouldBeNil)
				So(r.EVersion, ShouldEqual, initialEversion)
			})
		}
	})
}

func TestCancelRun(t *testing.T) {
	t.Parallel()

	Convey("CancelRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const runID = "chromium/1111111111111-deadbeef"
		initialEversion := 10

		for _, status := range []run.Status{run.Status_PENDING, run.Status_RUNNING} {
			Convey(fmt.Sprintf("Cancels when Run is %s", status), func() {
				startTime := time.Time{}
				if status == run.Status_RUNNING {
					startTime = datastore.RoundTime(ct.Clock.Now().UTC())
				}
				err := datastore.Put(ctx, &run.Run{
					ID:        runID,
					Status:    status,
					EVersion:  initialEversion,
					StartTime: startTime,
				})
				So(err, ShouldBeNil)
				ct.Clock.Add(1 * time.Minute)
				So(run.Cancel(ctx, runID), ShouldBeNil)
				cancelEvent := &eventpb.Event{
					Event: &eventpb.Event_Cancel{
						Cancel: &eventpb.Cancel{},
					},
				}
				runtest.AssertInEventbox(ctx, runID, cancelEvent)
				So(runtest.Runs(ct.TQ.Tasks()), ShouldResemble, common.RunIDs{runID})
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))
				runtest.AssertNotInEventbox(ctx, runID, cancelEvent)

				r := run.Run{ID: runID}
				So(datastore.Get(ctx, &r), ShouldBeNil)

				now := datastore.RoundTime(ct.Clock.Now().UTC())
				if startTime.IsZero() {
					startTime = now // Cancel will fill in start time if empty
				}
				So(r, ShouldResemble, run.Run{
					ID:         runID,
					Status:     run.Status_CANCELLED,
					EVersion:   initialEversion + 1,
					StartTime:  startTime,
					EndTime:    now,
					UpdateTime: now,
				})
			})
		}

		Convey("Panic when Run Status is not specified", func() {
			err := datastore.Put(ctx, &run.Run{
				ID:       runID,
				EVersion: initialEversion,
			})
			So(err, ShouldBeNil)
			So(run.Cancel(ctx, runID), ShouldBeNil)
			So(func() { ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run")) }, ShouldPanic)
		})

		statuses := []run.Status{
			run.Status_FINALIZING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				err := datastore.Put(ctx, &run.Run{
					ID:       runID,
					Status:   status,
					EVersion: initialEversion,
				})
				So(err, ShouldBeNil)
				So(run.Cancel(ctx, runID), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))

				r := run.Run{ID: runID}
				So(datastore.Get(ctx, &r), ShouldBeNil)
				So(r.EVersion, ShouldEqual, initialEversion)
			})
		}

		Convey("Cancel takes precedence over Start", func() {
			err := datastore.Put(ctx, &run.Run{
				ID:       runID,
				Status:   run.Status_PENDING,
				EVersion: initialEversion,
			})
			So(err, ShouldBeNil)

			So(run.Start(ctx, runID), ShouldBeNil)
			So(run.Cancel(ctx, runID), ShouldBeNil)
			startEvent := &eventpb.Event{
				Event: &eventpb.Event_Start{
					Start: &eventpb.Start{},
				},
			}
			cancelEvent := &eventpb.Event{
				Event: &eventpb.Event_Cancel{
					Cancel: &eventpb.Cancel{},
				},
			}
			runtest.AssertInEventbox(ctx, runID, startEvent, cancelEvent)

			So(runtest.Runs(ct.TQ.Tasks()), ShouldResemble, common.RunIDs{runID})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-manage-run"))
			// Consumed both events
			runtest.AssertNotInEventbox(ctx, runID, startEvent, cancelEvent)

			r := run.Run{ID: runID}
			So(datastore.Get(ctx, &r), ShouldBeNil)
			So(r.Status, ShouldEqual, run.Status_CANCELLED)
			So(r.EVersion, ShouldEqual, initialEversion+1)
			// TODO(yiwzhang): Figure out if there's a way to test if the result
			// state is the same as start+poke+cancel+poke.
		})
	})
}

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
