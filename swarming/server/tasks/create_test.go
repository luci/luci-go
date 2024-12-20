// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func TestCreation(t *testing.T) {
	t.Parallel()

	ftt.Run("Creation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("duplicate_with_request_id", func(t *ftt.Test) {
			t.Run("error_fetching_result", func(t *ftt.Test) {
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "bad_task_id"),
					TaskID: "not_a_task_id",
				}
				assert.That(t, datastore.Put(ctx, tri), should.ErrLike(nil))

				s := &Creation{
					RequestID: "bad_task_id",
				}
				_, err := s.Run(ctx)
				assert.That(t, err, should.ErrLike("datastore error fetching the task"))
			})
			t.Run("found_duplicate", func(t *ftt.Test) {
				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.That(t, err, should.ErrLike(nil))
				tr := &model.TaskRequest{
					Key: reqKey,
				}
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_COMPLETED,
					},
				}
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "exist"),
					TaskID: id,
				}
				assert.That(t, datastore.Put(ctx, tr, trs, tri), should.ErrLike(nil))

				s := &Creation{
					RequestID: "exist",
				}
				res, err := s.Run(ctx)
				assert.That(t, err, should.ErrLike(nil))
				assert.Loosely(t, res, should.NotBeNil)
				assert.That(t, res.ToProto(), should.Match(trs.ToProto()))
			})
		})
	})

}
