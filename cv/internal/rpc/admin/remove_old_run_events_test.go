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

package admin

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox/dsset"
	"go.chromium.org/luci/cv/internal/cvtesting"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRemoveOldRunEvents(t *testing.T) {
	t.Parallel()

	Convey("Remove old events from old Runs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const gHost = "x-review.example.com"

		runCnt := 0
		mkRun := func(diff time.Duration, events ...*eventpb.Event) *run.Run {
			runCnt++
			r := &run.Run{
				ID:         common.RunID(fmt.Sprintf("proj/%d-1-beef", runCnt)),
				Status:     run.Status_FAILED, // doesn't matter, so long as ended.
				CreateTime: datastore.RoundTime(ignoreRunsAfter.UTC().Add(diff)),
			}
			So(datastore.Put(ctx, r), ShouldBeNil)

			set := dsset.Set{Parent: datastore.MakeKey(ctx, run.RunKind, string(r.ID))}
			items := make([]dsset.Item, len(events))
			var err error
			for i, e := range events {
				items[i].ID = strconv.Itoa(i)
				items[i].Value, err = proto.Marshal(e)
				So(err, ShouldBeNil)
			}
			So(set.Add(ctx, items), ShouldBeNil)
			return r
		}

		listEvents := func(r *run.Run) []*eventpb.Event {
			set := dsset.Set{Parent: datastore.MakeKey(ctx, run.RunKind, string(r.ID))}
			l, err := set.List(ctx, 100)
			So(err, ShouldBeNil)
			out := make([]*eventpb.Event, len(l.Items))
			for i, item := range l.Items {
				out[i] = &eventpb.Event{}
				err := proto.Unmarshal(item.Value, out[i])
				So(err, ShouldBeNil)
			}
			return out
		}

		delEvent1 := &eventpb.Event{Event: &eventpb.Event_ClUpdated{
			ClUpdated: &changelist.CLUpdatedEvent{Clid: 1, Eversion: 1},
		}}
		delEvent2 := &eventpb.Event{Event: &eventpb.Event_CqdFinished{
			CqdFinished: &eventpb.CQDFinished{},
		}}
		delEvent3 := &eventpb.Event{Event: &eventpb.Event_SubmissionCompleted{
			SubmissionCompleted: &eventpb.SubmissionCompleted{
				FailureReason: &eventpb.SubmissionCompleted_ClFailure{
					ClFailure: &eventpb.SubmissionCompleted_CLSubmissionFailure{
						Clid:    1,
						Message: "old style",
					},
				},
			},
		}}
		// okEvent1 is same as delEvent3, but uses plural ClFailures.
		okEvent1 := &eventpb.Event{Event: &eventpb.Event_SubmissionCompleted{
			SubmissionCompleted: &eventpb.SubmissionCompleted{
				FailureReason: &eventpb.SubmissionCompleted_ClFailures{
					ClFailures: &eventpb.SubmissionCompleted_CLSubmissionFailures{
						Failures: []*eventpb.SubmissionCompleted_CLSubmissionFailure{
							{
								Clid:    1,
								Message: "new style",
							},
						},
					},
				},
			},
		}}
		okEvent2 := &eventpb.Event{Event: &eventpb.Event_CqdVerificationCompleted{
			CqdVerificationCompleted: &eventpb.CQDVerificationCompleted{},
		}}

		table := []struct {
			name      string
			r         *run.Run
			expEvents []*eventpb.Event
		}{
			{
				"new",
				mkRun(+time.Hour, okEvent2),
				[]*eventpb.Event{okEvent2},
			},
			{
				"new with old events, just to ensure it won't be touched",
				mkRun(+time.Second, delEvent1),
				[]*eventpb.Event{delEvent1},
			},
			{
				"old, empty",
				mkRun(-time.Second),
				[]*eventpb.Event{},
			},
			{
				"old, must be fully cleaned",
				mkRun(-time.Minute, delEvent1, delEvent2, delEvent3),
				[]*eventpb.Event{},
			},
			{
				"old, mix",
				mkRun(-time.Hour, delEvent1, okEvent1, delEvent2, okEvent2, delEvent3),
				[]*eventpb.Event{okEvent1, okEvent2},
			},
		}

		// Run the migration.
		ctrl := &dsmapper.Controller{}
		ctrl.Install(ct.TQDispatcher)
		a := New(ct.TQDispatcher, ctrl, nil, nil, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "run-event-cleanup"})
		So(err, ShouldBeNil)
		ct.TQ.Run(ctx, tqtesting.StopWhenDrained())
		jobInfo, err := a.DSMGetJob(ctx, jobID)
		So(err, ShouldBeNil)
		So(jobInfo.GetInfo().GetState(), ShouldEqual, dsmapperpb.State_SUCCESS)

		// Verify.
		for _, tcase := range table {
			Println(tcase.name)
			So(listEvents(tcase.r), ShouldResembleProto, tcase.expEvents)
		}
	})
}
