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

package botapi

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestTaskError(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		botID := "bot-id"
		taskID := "65aba3a3e6b99200"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		tr := &model.TaskRequest{
			Key: reqKey,
		}
		assert.NoErr(t, datastore.Put(ctx, tr))

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "bot:bot-id",
		})

		secret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("secret"),
		})

		srv := BotAPIServer{
			cfg:        cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			hmacSecret: secret,
			version:    "server-ver",
			submitUpdate: func(ctx context.Context, u *botinfo.Update) error {
				assert.That(t, u.EventType, should.Equal(model.BotEventTaskError))
				return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					res, err := u.Prepare(ctx, &model.BotInfo{
						Key: model.BotInfoKey(ctx, u.BotID),
					})
					assert.That(t, res.Proceed, should.BeTrue)
					return err
				}, nil)
			},
		}
		call := func(req *TaskErrorRequest, clientError *tasks.ClientError) {
			srv.tasksManager = &tasks.MockedManager{
				CompleteTxnMock: func(ctx context.Context, op *tasks.CompleteOp) (*tasks.CompleteTxnOutcome, error) {
					assert.That(t, op.ClientError, should.Match(clientError))
					return &tasks.CompleteTxnOutcome{
						Updated: true,
					}, nil
				},
			}

			resp, err := srv.TaskError(ctx, req, &botsrv.Request{
				Session: &internalspb.Session{
					BotId: botID,
					BotConfig: &internalspb.BotConfig{
						LogsCloudProject: "logs-cloud-project",
					},
					SessionId: "session-id",
				},
				CurrentTaskID: taskID,
			})
			assert.NoErr(t, err)
			assert.Loosely(t, resp, should.NotBeNil)
		}

		t.Run("with_client_error", func(t *ftt.Test) {
			req := &TaskErrorRequest{
				TaskID:  taskID,
				Message: "boom",
				ClientError: &ClientError{
					MissingCAS: []CASReference{
						{
							Instance: "instance",
							Digest:   "hash/3",
						},
					},
					MissingCIPD: []model.CIPDPackage{
						{
							PackageName: "package",
							Version:     "version",
							Path:        "path",
						},
					},
				},
			}

			clientError := &tasks.ClientError{
				MissingCAS: []model.CASReference{
					{
						CASInstance: "instance",
						Digest: model.CASDigest{
							Hash:      "hash",
							SizeBytes: int64(3),
						},
					},
				},
				MissingCIPD: []model.CIPDPackage{
					{
						PackageName: "package",
						Version:     "version",
						Path:        "path",
					},
				},
			}

			call(req, clientError)
		})

		t.Run("with_empty_client_error", func(t *ftt.Test) {
			req := &TaskErrorRequest{
				TaskID:  taskID,
				Message: "boom",
				ClientError: &ClientError{
					MissingCAS:  []CASReference{},
					MissingCIPD: []model.CIPDPackage{},
				},
			}

			call(req, nil)
		})
	})
}
