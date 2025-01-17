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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
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
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = cryptorand.MockForTest(ctx, 0)

		t.Run("duplicate_with_request_id", func(t *ftt.Test) {
			t.Run("error_fetching_result", func(t *ftt.Test) {
				tri := &model.TaskRequestID{
					Key:    model.TaskRequestIDKey(ctx, "bad_task_id"),
					TaskID: "not_a_task_id",
				}
				assert.NoErr(t, datastore.Put(ctx, tri))

				s := &Creation{
					RequestID: "bad_task_id",
				}
				_, err := s.Run(ctx)
				assert.That(t, err, should.ErrLike("failed to get TaskResultSummary for request id bad_task_id"))
			})
			t.Run("found_duplicate", func(t *ftt.Test) {
				id := "65aba3a3e6b99310"
				reqKey, err := model.TaskIDToRequestKey(ctx, id)
				assert.NoErr(t, err)
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
				assert.NoErr(t, datastore.Put(ctx, tr, trs, tri))

				c := &Creation{
					RequestID: "exist",
				}
				res, err := c.Run(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, res, should.NotBeNil)
				assert.That(t, res.ToProto(), should.Match(trs.ToProto()))
			})
		})

		t.Run("id_collision", func(t *ftt.Test) {
			ctx := cryptorand.MockForTestWithIOReader(ctx, &oneValue{v: "same_id"})
			key := model.NewTaskRequestKey(ctx)
			tr1 := &model.TaskRequest{
				Key: key,
			}
			assert.That(t, datastore.Put(ctx, tr1), should.ErrLike(nil))

			c := &Creation{
				Request: &model.TaskRequest{
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"pool": {"pool"},
								},
							},
						},
					},
				},
			}
			_, err := c.Run(ctx)
			assert.That(t, err, should.ErrLike(ErrAlreadyExists))
		})

		t.Run("OK", func(t *ftt.Test) {
			c := &Creation{
				Request: &model.TaskRequest{
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"pool": {"pool"},
								},
							},
						},
					},
				},
			}
			res, err := c.Run(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, res, should.NotBeNil)
			assert.That(
				t, model.RequestKeyToTaskID(res.TaskRequestKey(), model.AsRequest),
				should.Equal("2cbe1fa55012fa10"))
		})
	})
}

type oneValue struct {
	v string
}

func (o *oneValue) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = byte(o.v[i])
	}
	return len(p), nil
}
