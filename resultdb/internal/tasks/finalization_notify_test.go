// Copyright 2022 The LUCI Authors.
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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestNotifyInvocationFinalized(t *testing.T) {
	t.Parallel()

	ftt.Run("With fake task queue scheduler", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, tq := tq.TestingContext(ctx, nil)

		t.Run("Enqueues a pub/sub notification", func(t *ftt.Test) {
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				msg := &pb.InvocationFinalizedNotification{
					Invocation: "invocations/x",
					Realm:      "myproject:myrealm",
				}
				NotifyInvocationFinalized(ctx, msg)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			tasks := tq.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(1))
			task := tasks[0]

			attrs := task.Message.GetAttributes()
			assert.Loosely(t, attrs, should.ContainKey("luci_project"))
			assert.Loosely(t, attrs["luci_project"], should.Equal("myproject"))

			var msg pb.InvocationFinalizedNotification
			assert.Loosely(t, protojson.Unmarshal(task.Message.GetData(), &msg), should.BeNil)
			assert.Loosely(t, &msg, should.Match(&pb.InvocationFinalizedNotification{
				Invocation: "invocations/x",
				Realm:      "myproject:myrealm",
			}))
		})
	})
}

func TestNotifyRootInvocationFinalized(t *testing.T) {
	t.Parallel()

	ftt.Run("With fake task queue scheduler", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, tq := tq.TestingContext(ctx, nil)

		t.Run("Enqueues a pub/sub notification", func(t *ftt.Test) {
			properties := rootInvProperties()
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				msg := &pb.RootInvocationFinalizedNotification{
					RootInvocation: &pb.RootInvocation{
						Name:       "rootInvocations/x",
						Realm:      "myproject:myrealm",
						State:      pb.RootInvocation_SUCCEEDED,
						Properties: properties,
					},
				}
				NotifyRootInvocationFinalized(ctx, msg)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			tasks := tq.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(1))
			task := tasks[0]

			attrs := task.Message.GetAttributes()
			assert.Loosely(t, attrs, should.ContainKey(luciProjectFilter))
			assert.Loosely(t, attrs[luciProjectFilter], should.Equal("myproject"))
			assert.Loosely(t, attrs, should.ContainKey(stateFilter))
			assert.Loosely(t, attrs[stateFilter], should.Equal(pb.RootInvocation_SUCCEEDED.String()))
			assert.Loosely(t, attrs, should.ContainKey(androidRunnerFilter))
			assert.Loosely(t, attrs[androidRunnerFilter], should.Equal("Bazel"))
			assert.Loosely(t, attrs, should.ContainKey(androidBranchFilter))
			assert.Loosely(t, attrs[androidBranchFilter], should.Equal("main"))
			assert.Loosely(t, attrs, should.ContainKey(androidTargetFilter))
			assert.Loosely(t, attrs[androidTargetFilter], should.Equal("brya-trunk_staging-userdebug_coverage"))

			var msg pb.RootInvocationFinalizedNotification
			assert.Loosely(t, protojson.Unmarshal(task.Message.GetData(), &msg), should.BeNil)
			assert.Loosely(t, &msg, should.Match(&pb.RootInvocationFinalizedNotification{
				RootInvocation: &pb.RootInvocation{
					Name:       "rootInvocations/x",
					Realm:      "myproject:myrealm",
					State:      pb.RootInvocation_SUCCEEDED,
					Properties: properties,
				},
			}))
		})
	})
}

func rootInvProperties() *structpb.Struct {
	properties := make(map[string]*structpb.Value)
	properties["@type"] = structpb.NewStringValue("type.googleapis.com/google.protobuf.Struct")
	properties["runner"] = structpb.NewStringValue("Bazel")
	properties["primary_build"] = structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
		"branch":       structpb.NewStringValue("main"),
		"build_target": structpb.NewStringValue("brya-trunk_staging-userdebug_coverage"),
	}})
	return &structpb.Struct{Fields: properties}
}

func TestNotifyTestResults(t *testing.T) {
	t.Parallel()

	ftt.Run("With fake task queue scheduler", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, tq := tq.TestingContext(ctx, nil)

		t.Run("Enqueues a pub/sub notification", func(t *ftt.Test) {
			msg := &pb.TestResultsNotification{
				ResultdbHost: "test-host",
			}
			attrs := map[string]string{"branch": "main"}
			NotifyTestResults(ctx, msg, attrs)

			tasks := tq.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(1))
			task := tasks[0]

			assert.Loosely(t, task.Message.GetAttributes(), should.Match(map[string]string{"branch": "main"}))

			var nMsg pb.TestResultsNotification
			assert.Loosely(t, proto.Unmarshal(task.Message.GetData(), &nMsg), should.BeNil)
			assert.Loosely(t, &nMsg, should.Match(&pb.TestResultsNotification{
				ResultdbHost: "test-host",
			}))
		})
	})
}
