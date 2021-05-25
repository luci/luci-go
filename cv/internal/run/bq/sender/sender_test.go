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

package sender

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPurgeCL(t *testing.T) {
	t.Parallel()

	Convey("SendRunRow works", t, func() {
		ct := cvtesting.Test{AppID: "cv"}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Make a tq dispatcher.
		// This is where we make a fake BQ client.

		//pmNotifier := prjmanager.NewNotifier(ct.TQDispatcher)

		// Then, make new sender.
		//purger := New(pmNotifier, clUpdater)

		const lProject = "lprj"
		const gHost = "x-review"
		const gRepo = "repo"
		const change = 43

		cfg := makeConfig(gHost, gRepo)
		ct.Cfg.Create(ctx, lProject, cfg)
		So(gobmap.Update(ctx, lProject), ShouldBeNil)

		// Set up datastore by putting a sample Run in.

		// Make a task
		task := &prjpb.PurgeCLTask{
			LuciProject: lProject,
			PurgingCl: &prjpb.PurgingCL{
				OperationId: "op",
				Clid:        int64(clBefore.ID),
				Deadline:    timestamppb.New(ct.Clock.Now().Add(10 * time.Minute)),
			},
			Trigger: trigger.Find(ci, cfg.GetConfigGroups()[0]),
			Reasons: []*changelist.CLError{
				{Kind: &changelist.CLError_OwnerLacksEmail{OwnerLacksEmail: true}},
			},
		}
		So(task.Trigger, ShouldNotBeNil)

		// XXX Why is it necssary to schedule like this?
		schedule := func() error {
			return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
				return purger.Schedule(tCtx, task)
			}, nil)
		}

		// XXX Why is it necessary to change time?
		ct.Clock.Add(time.Minute)

		// XXX Note about what needs to be asserted:
		// Simple case where row is sent.
		// Some error cases like:
		//   - Run is not valid or finished? ..

		Convey("Happy path: A finished Run has runs created.", func() {
			//So(schedule(), ShouldBeNil)
			//ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))

			Convey("Idempotent: if TQ task is retried, just notify PM", func() {
				// Use different Operation ID s.t. we can easily assert PM was notified
				// the 2nd time.
				//task.PurgingCl.OperationId = "op-2"
				//So(schedule(), ShouldBeNil)
				//ct.TQ.Run(ctx, tqtesting.StopAfterTask(prjpb.PurgeProjectCLTaskClass))
				//So(loadCL().EVersion, ShouldEqual, clAfter.EVersion)
				//assertPMNotified("op-2")
				//So(pmDispatcher.LatestETAof(lProject), ShouldHappenBefore, ct.Clock.Now().Add(2*time.Second))
			})
		})

	})
}
