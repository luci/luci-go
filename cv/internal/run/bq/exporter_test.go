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

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestExportRunToBQ(t *testing.T) {
	t.Parallel()

	ftt.Run("Exporting a Run to BQ works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		exporter := NewExporter(ct.TQDispatcher, ct.BQFake, ct.Env)

		// Set up datastore by putting a sample Run + RunCLs.
		epoch := ct.Clock.Now().UTC()
		runID := common.MakeRunID("lproject", epoch, 1, []byte("aaa"))
		r := run.Run{
			ID:            runID,
			Status:        run.Status_SUCCEEDED,
			ConfigGroupID: "sha256:deadbeefdeadbeef/cgroup",
			CreateTime:    epoch,
			StartTime:     epoch.Add(time.Minute * 2),
			EndTime:       epoch.Add(time.Minute * 25),
			CLs:           common.CLIDs{1},
			Submission:    nil,
			Mode:          run.DryRun,
		}
		assert.Loosely(t, datastore.Put(ctx, &r), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, &run.RunCL{
			ID:         1,
			Run:        datastore.MakeKey(ctx, common.RunKind, string(runID)),
			ExternalID: "gerrit/foo-review.googlesource.com/111",
			Detail: &changelist.Snapshot{
				LuciProject:           "lproject",
				Patchset:              2,
				MinEquivalentPatchset: 2,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: "foo-review.googlesource.com",
						Info: &gerritpb.ChangeInfo{
							Number:  111,
							Project: "gproject",
							Ref:     "refs/heads/main",
						},
					},
				},
			},
			Trigger: &run.Trigger{Time: timestamppb.New(epoch)},
		}), should.BeNil)
		assert.Loosely(t, nil, should.BeNil)

		// BQ Export tasks must be scheduled in a transaction.
		schedule := func() error {
			return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
				return exporter.Schedule(tCtx, runID)
			}, nil)
		}

		t.Run("A row is sent", func(t *ftt.Test) {
			t.Run("in prod", func(t *ftt.Test) {
				assert.Loosely(t, schedule(), should.BeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(exportRunToBQTaskClass))
				rows := ct.BQFake.Rows("", CVDataset, CVTable)
				assert.That(t, rows, should.Match([]protoreflect.ProtoMessage{&cvbqpb.Attempt{
					Key:                  runID.AttemptKey(),
					LuciProject:          runID.LUCIProject(),
					RunId:                string(runID),
					ConfigGroup:          "cgroup",
					ClGroupKey:           "331ea2a6a5d5f3b3",
					EquivalentClGroupKey: "47337d4707144297",
					StartTime:            timestamppb.New(epoch),
					ActualStartTime:      timestamppb.New(epoch.Add(time.Minute * 2)),
					EndTime:              timestamppb.New(epoch.Add(25 * time.Minute)),
					GerritChanges: []*cvbqpb.GerritChange{
						{
							Host:                       "foo-review.googlesource.com",
							Project:                    "gproject",
							Change:                     111,
							Patchset:                   2,
							EarliestEquivalentPatchset: 2,
							TriggerTime:                timestamppb.New(epoch),
							Mode:                       cvbqpb.Mode_DRY_RUN,
							SubmitStatus:               cvbqpb.GerritChange_PENDING,
						},
					},
					Status:    cvbqpb.AttemptStatus_SUCCESS,
					Substatus: cvbqpb.AttemptSubstatus_NO_SUBSTATUS,
				}}))

				// The same rows must be sent to legacy CQ table.
				cqRows := ct.BQFake.Rows(legacyProject, legacyDataset, legacyTable)
				assert.That(t, cqRows, should.Match(rows))
				// And only these 2 rows have been sent.
				assert.Loosely(t, ct.BQFake.TotalSent(), should.Equal(2))
			})

			t.Run("in dev", func(t *ftt.Test) {
				assert.Loosely(t, schedule(), should.BeNil)
				ct.Env.IsGAEDev = true
				ct.TQ.Run(ctx, tqtesting.StopAfterTask(exportRunToBQTaskClass))
				assert.Loosely(t, ct.BQFake.Rows("", CVDataset, CVTable), should.HaveLength(1))
				// Must not send to production legacy.
				assert.Loosely(t, ct.BQFake.Rows(legacyProject, legacyDataset, legacyTable), should.HaveLength(0))
				assert.Loosely(t, ct.BQFake.Rows(legacyProjectDev, legacyDataset, legacyTable), should.HaveLength(1))
			})
		})
	})
}
