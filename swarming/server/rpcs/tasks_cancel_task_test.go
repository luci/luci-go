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

package rpcs

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestCancelTask(t *testing.T) {
	t.Parallel()

	ftt.Run("CancelTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tqt := tqtasks.TestingContext(ctx)

		srv := &TasksServer{
			TasksManager: tasks.NewManager(tqt.Tasks, "swarming", "version", nil, false),
		}

		state := NewMockedRequestState()
		state.MockPerm("project:visible-realm", acls.PermTasksCancel)
		ctx = MockRequestState(ctx, state)

		taskID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		call := func(ctx context.Context, taskID string, killRunning bool) (*apipb.CancelResponse, error) {
			ctx = MockRequestState(ctx, state)
			return srv.CancelTask(ctx, &apipb.TaskCancelRequest{
				TaskId:      taskID,
				KillRunning: killRunning,
			})
		}

		t.Run("empty taskID", func(t *ftt.Test) {
			_, err := call(ctx, "", false)
			assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("task request not exist", func(t *ftt.Test) {
			_, err := call(ctx, taskID, false)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("acl check fails", func(t *ftt.Test) {
			_ = datastore.Put(ctx,
				&model.TaskRequest{
					Key:   reqKey,
					Realm: "project:unknown-realm",
					TaskSlices: []model.TaskSlice{
						{
							Properties: model.TaskProperties{
								Dimensions: model.TaskDimensions{
									"pool": {"some-pool"},
								},
							},
						},
					},
				},
			)
			_, err = call(ctx, taskID, false)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run("cancel", func(t *ftt.Test) {
			tr := &model.TaskRequest{
				Key:   reqKey,
				Realm: "project:visible-realm",
				TaskSlices: []model.TaskSlice{
					{
						Properties: model.TaskProperties{
							Dimensions: model.TaskDimensions{
								"pool": {"some-pool"},
							},
						},
					},
				},
				PubSubTopic: "pubsub-topic",
				RBEInstance: "rbe-instance",
			}
			_ = datastore.Put(ctx, tr)

			t.Run("fail", func(t *ftt.Test) {
				// No TaskResultSummary entity.
				_, err = call(ctx, taskID, false)
				assert.That(t, err, grpccode.ShouldBe(codes.Internal))
			})

			t.Run("pass", func(t *ftt.Test) {
				now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
				ctx, _ = testclock.UseTime(ctx, now)
				trs := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, reqKey),
					TaskResultCommon: model.TaskResultCommon{
						Modified: now.Add(-time.Minute),
						State:    apipb.TaskState_PENDING,
					},
				}
				toRunKey, _ := model.TaskRequestToToRunKey(ctx, tr, 0)
				ttr := &model.TaskToRun{
					Key:            toRunKey,
					QueueNumber:    datastore.NewIndexedOptional(int64(2)),
					Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
					RBEReservation: "reservation",
				}
				_ = datastore.Put(ctx, trs, ttr)
				rsp, err := call(ctx, taskID, false)
				assert.NoErr(t, err)
				assert.That(t, rsp.Canceled, should.BeTrue)
				assert.That(t, rsp.WasRunning, should.BeFalse)
				assert.That(t, tqt.Pending(tqt.CancelRBE), should.Match([]string{"rbe-instance/reservation"}))
				assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
			})
		})
	})
}
