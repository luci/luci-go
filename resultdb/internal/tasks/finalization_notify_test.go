// Copyright 2022 The LUCI Authors.
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

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNotifyInvocationFinalized(t *testing.T) {
	t.Parallel()

	Convey("With fake task queue scheduler", t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, tq := tq.TestingContext(ctx, nil)

		Convey("Enqueues a pub/sub notification", func() {
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				msg := &pb.InvocationFinalizedNotification{
					Invocation: "invocations/x",
					Realm:      "myproject:myrealm",
				}
				NotifyInvocationFinalized(ctx, msg)
				return nil
			})
			So(err, ShouldBeNil)

			tasks := tq.Tasks()
			So(tasks, ShouldHaveLength, 1)
			t := tasks[0]

			attrs := t.Message.GetAttributes()
			So(attrs, ShouldContainKey, "luci_project")
			So(attrs["luci_project"], ShouldEqual, "myproject")

			var msg pb.InvocationFinalizedNotification
			So(protojson.Unmarshal(t.Message.GetData(), &msg), ShouldBeNil)
			So(&msg, ShouldResembleProto, &pb.InvocationFinalizedNotification{
				Invocation: "invocations/x",
				Realm:      "myproject:myrealm",
			})
		})
	})
}
