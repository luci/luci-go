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

package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	executorpb "go.chromium.org/turboci/proto/go/graph/executor/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

const trustedBackend identity.Identity = "user:trusted@example.com"

func init() {
	testAllowFakeTestStage = true
}

func TestTurboCIInterceptor(t *testing.T) {
	t.Parallel()

	t.Run("OK simple", func(t *testing.T) {
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: trustedBackend,
		})

		stage := fakeTestStage("enduser@example.com", "some-project")

		for _, req := range []proto.Message{
			executorpb.RunStageRequest_builder{Stage: stage}.Build(),
			executorpb.ValidateStageRequest_builder{Stage: stage}.Build(),
			executorpb.CancelStageRequest_builder{Stage: stage}.Build(),
		} {
			callInterceptor(ctx, t, req, func(ctx context.Context) {
				assert.That(t, auth.CurrentIdentity(ctx).Email(), should.Equal("enduser@example.com"))
			})
		}
	})

	t.Run("OK project-scoped, same project", func(t *testing.T) {
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: trustedBackend,
			FakeDB: authtest.NewFakeDB(
				authtest.MockProjectScopedAccount("some-project", "some-project-scoped@example.com"),
			),
		})

		req := executorpb.RunStageRequest_builder{
			Stage: fakeTestStage("some-project-scoped@example.com", "some-project"),
		}.Build()

		callInterceptor(ctx, t, req, func(ctx context.Context) {
			assert.That(t, string(auth.CurrentIdentity(ctx)), should.Equal("project:some-project"))
		})
	})

	t.Run("OK project-scoped, different project", func(t *testing.T) {
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: trustedBackend,
			FakeDB: authtest.NewFakeDB(
				authtest.MockProjectScopedAccount("some-project", "some-project-scoped@example.com"),
				authtest.MockProjectScopedAccount("another-project", "another-project-scoped@example.com"),
			),
		})

		req := executorpb.RunStageRequest_builder{
			Stage: fakeTestStage("another-project-scoped@example.com", "some-project"),
		}.Build()

		callInterceptor(ctx, t, req, func(ctx context.Context) {
			assert.That(t, string(auth.CurrentIdentity(ctx)), should.Equal("user:another-project-scoped@example.com"))
		})
	})

	t.Run("Untrusted backend", func(t *testing.T) {
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: "user:unknown@example.com",
		})

		req := executorpb.RunStageRequest_builder{
			Stage: fakeTestStage("enduser@example.com", "some-project"),
		}.Build()

		assert.That(t, callInterceptor(ctx, t, req, nil), should.ErrLike("is not allowed to call TurboCI Stage Executor API"))
	})

	t.Run("Unexpected request type", func(t *testing.T) {
		ctx := auth.WithState(t.Context(), &authtest.FakeState{
			Identity: trustedBackend,
		})

		req := &emptypb.Empty{}

		assert.That(t, callInterceptor(ctx, t, req, nil), should.ErrLike("no `stage` field in the request"))
	})
}

func callInterceptor(ctx context.Context, t *testing.T, req proto.Message, handler func(context.Context)) error {
	t.Helper()

	expectFail := handler == nil
	called := false

	interceptor := TurboCIInterceptor(context.Background(), trustedBackend.Email())
	_, err := interceptor(ctx, req, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		called = true
		if handler == nil {
			t.Fatalf("The call was unexpectedly passed through")
		} else {
			handler(ctx)
		}
		return nil, nil
	})

	if expectFail && err == nil {
		t.Fatalf("The call unexpectedly succeeded")
	}
	if !expectFail && err != nil {
		t.Fatalf("The call unexpectedly failed: %s", err)
	}
	if !expectFail && !called {
		t.Fatalf("The call unexpectedly wasn't passed through")
	}

	return err
}

func fakeTestStage(submitter, project string) *orchestratorpb.Stage {
	strct, err := structpb.NewValue(map[string]any{
		"submitter": submitter,
		"project":   project,
	})
	if err != nil {
		panic(err)
	}
	testArgs, err := anypb.New(strct)
	if err != nil {
		panic(err)
	}
	return orchestratorpb.Stage_builder{
		Args: orchestratorpb.Value_builder{
			Value: testArgs,
		}.Build(),
	}.Build()
}
