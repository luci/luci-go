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
	"encoding/json"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/swarming/server/tasks"
)

func TestTaskBackendRunTask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lt := tasks.MockTQTasks()
	srv := &TaskBackend{
		BuildbucketTarget:       "swarming://target",
		BuildbucketAccount:      "ignored-in-the-test",
		DisableBuildbucketCheck: true,
		TasksServer:             &TasksServer{TaskLifecycleTasks: lt},
	}

	ftt.Run("validate", t, func(t *ftt.Test) {
		t.Run("no_id", func(t *ftt.Test) {
			_, err := srv.RunTask(ctx, &bbpb.RunTaskRequest{})
			assert.That(t, err, should.ErrLike("build_id is required"))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("no_pubsub_topic", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId: "12345678",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("pubsub_topic is required"))
		})

		t.Run("no_target", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId:     "12345678",
				PubsubTopic: "pubsub_topic",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike("target: required"))
		})

		t.Run("wrong_target", func(t *ftt.Test) {
			req := &bbpb.RunTaskRequest{
				BuildId:     "12345678",
				PubsubTopic: "pubsub_topic",
				Target:      "wrong",
			}
			_, err := srv.RunTask(ctx, req)
			assert.That(t, err, should.ErrLike(`Expected "swarming://target", got "wrong"`))
		})

		t.Run("backend_config", func(t *ftt.Test) {
			call := func(cfgMap any) (*bbpb.RunTaskResponse, error) {
				req := &bbpb.RunTaskRequest{
					BuildId:       "12345678",
					PubsubTopic:   "pubsub_topic",
					Target:        srv.BuildbucketTarget,
					BackendConfig: toBackendCfg(cfgMap),
				}
				return srv.RunTask(ctx, req)
			}

			cases := []struct {
				name string
				cfg  any
				errs []string
			}{
				{
					name: "fail_ingest_config",
					cfg: map[string]any{
						"random": 500,
					},
					errs: []string{"unknown field"},
				},
				{
					name: "missing_fields",
					cfg:  nil,
					errs: []string{
						"agent_binary_cipd_filename: required",
					},
				},
			}

			for _, c := range cases {
				t.Run(c.name, func(t *ftt.Test) {
					_, err := call(c.cfg)
					for _, e := range c.errs {
						assert.That(t, err, should.ErrLike(e))
					}
				})
			}
		})
	})
}

func toBackendCfg(raw any) *structpb.Struct {
	cfgJSON, _ := json.Marshal(raw)
	cfgStruct := &structpb.Struct{}
	_ = cfgStruct.UnmarshalJSON(cfgJSON)
	return cfgStruct
}
