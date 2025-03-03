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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cvpb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestExportRunToBQ(t *testing.T) {
	t.Parallel()

	ftt.Run("Publisher", t, func(t *ftt.Test) {
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

		t.Run("RunEnded enqueues a task", func(t *ftt.Test) {
			// A RunEnded task must be scheduled in a transaction.
			runEnded := func() error {
				return datastore.RunInTransaction(ctx, func(tCtx context.Context) error {
					return publisher.RunEnded(tCtx, r.ID, r.Status, r.EVersion)
				}, nil)
			}
			assert.Loosely(t, runEnded(), should.BeNil)
			tsk := ct.TQ.Tasks()[0]

			t.Run("with attributes", func(t *ftt.Test) {
				attrs := tsk.Message.GetAttributes()
				assert.Loosely(t, attrs, should.ContainKey("luci_project"))
				assert.Loosely(t, attrs["luci_project"], should.Equal(r.ID.LUCIProject()))
				assert.Loosely(t, attrs, should.ContainKey("status"))
				assert.Loosely(t, attrs["status"], should.Equal(r.Status.String()))
			})

			t.Run("with JSONPB encoded message", func(t *ftt.Test) {
				var msg cvpb.PubSubRun
				assert.Loosely(t, protojson.Unmarshal(tsk.Message.GetData(), &msg), should.BeNil)
				assert.That(t, &msg, should.Match(&cvpb.PubSubRun{
					Id:       r.ID.PublicID(),
					Status:   cvpb.Run_SUCCEEDED,
					Eversion: int64(r.EVersion),
					Hostname: ct.Env.LogicalHostname,
				}))
			})
		})
	})
}
