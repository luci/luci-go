// Copyright 2025 The LUCI Authors.
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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(
		datastore.Nullable[string, datastore.Indexed]{},
	))
}

func TestTerminateBot(t *testing.T) {
	t.Parallel()

	ftt.Run("TerminateBot", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		srv := &BotsServer{}
		pool := "pool"
		botID := "bot-id"
		state := NewMockedRequestState()
		poolCfg := state.Configs.MockPool(pool, "project:pool-realm")
		poolCfg.RbeMigration = &configpb.Pool_RBEMigration{
			RbeInstance:    "rbe-instance",
			RbeModePercent: int32(100),
			BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
				{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
			},
		}
		state.Configs.MockBot(botID, pool)
		state.MockPerm("project:pool-realm", acls.PermPoolsTerminateBot)

		call := func(srv *BotsServer, state *MockedRequestState, botID, reason string) (*apipb.TerminateResponse, error) {
			req := &apipb.TerminateRequest{
				BotId:  botID,
				Reason: reason,
			}

			return srv.TerminateBot(MockRequestState(ctx, state), req)
		}
		t.Run("validation", func(t *ftt.Test) {
			t.Run("missing_bot_id", func(t *ftt.Test) {
				_, err := call(srv, state, "", "")
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("bot_id is required"))
			})
			t.Run("invalid_reason", func(t *ftt.Test) {
				_, err := call(srv, state, botID, strings.Repeat("a", 1001))
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("reason is too long: 1001 > 1000"))
			})
		})

		t.Run("perm_check_failures", func(t *ftt.Test) {
			t.Run("unknown_bot", func(t *ftt.Test) {
				_, err := call(srv, state, "unknown-bot", "")
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike("swarming.pools.terminateBot"))
			})
			t.Run("pool_perm_check_fails", func(t *ftt.Test) {
				state := state.SetCaller(identity.AnonymousIdentity)
				_, err := call(srv, state, botID, "")
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike("swarming.pools.terminateBot"))
			})
		})

		t.Run("existing_found", func(t *ftt.Test) {
			existingTaskID := "65aba3a3e6b99100"
			reqKey, err := model.TaskIDToRequestKey(ctx, existingTaskID)
			assert.NoErr(t, err)
			trs := &model.TaskResultSummary{
				Key:     model.TaskResultSummaryKey(ctx, reqKey),
				Created: now.Add(-5 * time.Second),
				TaskResultCommon: model.TaskResultCommon{
					State: apipb.TaskState_PENDING,
				},
				Tags: []string{
					"id:bot-id",
					"swarming.terminate:1",
				},
			}
			assert.NoErr(t, datastore.Put(ctx, trs))
			res, err := call(srv, state, botID, "")
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&apipb.TerminateResponse{
				TaskId: existingTaskID,
			}))
		})

		t.Run("create_new", func(t *ftt.Test) {
			taskID := "65aba3a3e6b99200"

			srv := &BotsServer{
				TasksManager: &tasks.MockedManager{
					CreateTaskMock: func(ctx context.Context, op *tasks.CreationOp) (*tasks.CreatedTask, error) {
						expected := &model.TaskRequest{
							Created:       now,
							Authenticated: DefaultFakeCaller,
							Name:          "Terminate bot-id: reason",
							Priority:      0,
							Expiration:    now.Add(5 * 24 * time.Hour),
							RBEInstance:   "rbe-instance",
							ManualTags: []string{
								"swarming.terminate:1",
								fmt.Sprintf("rbe:%s", "rbe-instance"),
							},
							ServiceAccount: "none",
							TaskSlices: []model.TaskSlice{
								{
									ExpirationSecs: int64(5 * 24 * time.Hour.Seconds()),
									Properties: model.TaskProperties{
										Dimensions: model.TaskDimensions{
											"id": []string{botID},
										},
									},
								},
							},
							Tags: []string{
								"authenticated:user:test@example.com",
								"id:bot-id",
								"priority:0",
								"rbe:rbe-instance",
								"realm:none",
								"service_account:none",
								"swarming.pool.template:no_pool",
								"swarming.terminate:1",
								"user:none",
							},
						}
						assert.That(t, op.Request, should.Match(expected))
						req := op.Request
						req.Key, _ = model.TaskIDToRequestKey(ctx, taskID)

						trs := &model.TaskResultSummary{
							Key: model.TaskResultSummaryKey(ctx, req.Key),
						}
						return &tasks.CreatedTask{Request: req, Result: trs}, nil
					},
				},
			}
			res, err := call(srv, state, botID, "reason")
			assert.NoErr(t, err)
			assert.That(t, res, should.Match(&apipb.TerminateResponse{
				TaskId: taskID,
			}))
		})
	})
}
