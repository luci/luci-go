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
	"go.chromium.org/luci/cv/internal/common/eventbox/dsset"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRemoveOldProjectEvents(t *testing.T) {
	t.Parallel()

	Convey("Remove old events from old Projects", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		prjCount := 0
		mkProject := func(diff time.Duration, events ...*prjpb.Event) *prjmanager.Project {
			prjCount++
			pr := &prjmanager.Project{
				ID:         fmt.Sprintf("proj-%d", prjCount),
				EVersion:   1,
				UpdateTime: datastore.RoundTime(ignoreProjectsAfter.UTC().Add(diff)),
			}
			So(datastore.Put(ctx, pr), ShouldBeNil)

			set := dsset.Set{Parent: datastore.MakeKey(ctx, prjmanager.ProjectKind, pr.ID)}
			items := make([]dsset.Item, len(events))
			var err error
			for i, e := range events {
				items[i].ID = strconv.Itoa(i)
				items[i].Value, err = proto.Marshal(e)
				So(err, ShouldBeNil)
			}
			So(set.Add(ctx, items), ShouldBeNil)
			return pr
		}

		listEvents := func(pr *prjmanager.Project) []*prjpb.Event {
			set := dsset.Set{Parent: datastore.MakeKey(ctx, prjmanager.ProjectKind, pr.ID)}
			l, err := set.List(ctx, 100)
			So(err, ShouldBeNil)
			out := make([]*prjpb.Event, len(l.Items))
			for i, item := range l.Items {
				out[i] = &prjpb.Event{}
				err := proto.Unmarshal(item.Value, out[i])
				So(err, ShouldBeNil)
			}
			return out
		}

		delEvent := &prjpb.Event{Event: &prjpb.Event_ClUpdated{
			ClUpdated: &changelist.CLUpdatedEvent{Clid: 1, Eversion: 1},
		}}
		okEvent := &prjpb.Event{Event: &prjpb.Event_NewConfig{
			NewConfig: &prjpb.NewConfig{},
		}}

		table := []struct {
			name      string
			r         *prjmanager.Project
			expEvents []*prjpb.Event
		}{
			{
				"new",
				mkProject(+time.Hour, okEvent),
				[]*prjpb.Event{okEvent},
			},
			{
				"new with old events, just to ensure it won't be touched",
				mkProject(+time.Second, delEvent),
				[]*prjpb.Event{delEvent},
			},
			{
				"old, empty",
				mkProject(-time.Second),
				[]*prjpb.Event{},
			},
			{
				"old, must be fully cleaned",
				mkProject(-time.Minute, delEvent),
				[]*prjpb.Event{},
			},
			{
				"old, mix",
				mkProject(-time.Hour, delEvent, okEvent),
				[]*prjpb.Event{okEvent},
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
		jobID, err := a.DSMLaunchJob(ctx, &adminpb.DSMLaunchJobRequest{Name: "project-event-cleanup"})
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
