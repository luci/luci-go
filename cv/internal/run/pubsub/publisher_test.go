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

package pubsub

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/gae/service/datastore"

	cvpb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExportRunToBQ(t *testing.T) {
	t.Parallel()

	Convey("Publisher", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		publisher := NewPublisher(ct.TQDispatcher, ct.Env)
		epoch := ct.Clock.Now().UTC()
		r := run.Run{
			ID:            common.MakeRunID("lproject", epoch, 1, []byte("aaa")),
			Status:        run.Status_SUCCEEDED,
			ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
			CreateTime:    epoch,
			StartTime:     epoch.Add(time.Minute * 2),
			EndTime:       epoch.Add(time.Minute * 25),
			CLs:           common.CLIDs{1},
			Submission:    nil,
			Mode:          run.DryRun,
			EVersion:      123456,
		}

		Convey("RunEnded enqueues a task", func() {
			// A RunEnded task must be scheduled in a transaction.
			runEnded := func() error {
				return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
					return publisher.RunEnded(tCtx, r.ID, r.Status, r.EVersion)
				}, nil)
			}
			So(runEnded(), ShouldBeNil)
			tsk := ct.TQ.Tasks()[0]

			Convey("with attributes", func() {
				attrs := tsk.Message.GetAttributes()
				So(attrs, ShouldContainKey, "luci_project")
				So(attrs["luci_project"], ShouldEqual, r.ID.LUCIProject())
				So(attrs, ShouldContainKey, "status")
				So(attrs["status"], ShouldEqual, r.Status.String())
			})

			Convey("with JSONPB encoded message", func() {
				var msg cvpb.PubSubRun
				So(protojson.Unmarshal(tsk.Message.GetData(), &msg), ShouldBeNil)
				So(&msg, ShouldResembleProto, &cvpb.PubSubRun{
					Id:       r.ID.PublicID(),
					Status:   cvpb.Run_SUCCEEDED,
					Eversion: int64(r.EVersion),
					Hostname: ct.Env.LogicalHostname,
				})
			})
		})
	})
}
