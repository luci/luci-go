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

package tasks

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNotification(t *testing.T) {
	t.Parallel()

	Convey("notifyPubsub", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{})
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("w/o callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return NotifyPubSub(ctx, &model.Build{ID: 123})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, 123)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetCallback(), ShouldBeFalse)
		})

		Convey("w/ callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				cb := model.PubSubCallback{"token", "topic", []byte("user_data")}
				return NotifyPubSub(ctx, &model.Build{ID: 123, PubSubCallback: cb})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 2)

			n1 := tasks[0].Payload.(*taskdefs.NotifyPubSub)
			n2 := tasks[1].Payload.(*taskdefs.NotifyPubSub)
			So(n1.GetBuildId(), ShouldEqual, 123)
			So(n2.GetBuildId(), ShouldEqual, 123)
			// One w/ callback and one w/o callback.
			So(n1.GetCallback() != n2.GetCallback(), ShouldBeTrue)
		})
	})
}
