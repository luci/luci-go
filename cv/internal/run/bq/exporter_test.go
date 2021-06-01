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

package bq

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

const PurgeProjectCLTaskClass = "purge-project-cl"

func TestExportRunToBQ(t *testing.T) {
	t.Parallel()

	Convey("Exporting a Run to BQ works", t, func() {
		ct := cvtesting.Test{AppID: "cv"}
		ctx, cancel := ct.SetUp()
		defer cancel()

		// Get a TQ dispatcher and BQ client from cvtesting.Test
		// and make a new Exporter.
		exporter := NewExporter(ct.TQDispatcher, ct.BQFake)

		// Set up datastore by putting a sample Run + RunCLs in datastore.
		epoch := ct.Clock.Now().UTC()
		runID := common.MakeRunID("lproject", epoch, 1, []byte("aaa"))
		r := &run.Run{
			ID:            common.RunID(runID),
			Status:        run.Status_SUCCEEDED,
			ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
			CreateTime:    epoch,
			StartTime:     epoch.Add(time.Minute * 2),
			EndTime:       epoch.Add(time.Minute * 25),
			CLs:           common.CLIDs{1},
			Submission:    nil,
			Mode:          run.DryRun,
		}
		So(datastore.Put(ctx, r), ShouldBeNil)
		So(datastore.Put(ctx, &run.RunCL{
			ID:         1,
			Run:        datastore.MakeKey(ctx, run.RunKind, string(runID)),
			ExternalID: "gerrit/foo-review.googlesource.com/111",
			Detail: &changelist.Snapshot{
				LuciProject:           "lproject",
				Patchset:              2,
				MinEquivalentPatchset: 2,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: "foo-review.googlesource.com",
						Info: &gerrit.ChangeInfo{
							Number:  111,
							Project: "gproject",
							Ref:     "refs/heads/main",
						},
					},
				},
			},
			Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
		}), ShouldBeNil)
		So(nil, ShouldBeNil)

		// Make a task to send that sample Run.
		task := &ExportRunToBQTask{RunId: string(runID)}

		schedule := func() error {
			return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
				return exporter.Schedule(tCtx, task)
			}, nil)
		}

		Convey("A row is sent", func() {
			So(schedule(), ShouldBeNil)
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(exportRunToBQTaskClass))
			rows := ct.BQClient.Rows()
			So(len(rows), ShouldEqual, 1)
			// (Not finished) Assert the Attempt proto in run in the fake client.
			//So(rows[0], ShouldResembleProto, cvbqpb.Attempt{})
		})
	})
}
