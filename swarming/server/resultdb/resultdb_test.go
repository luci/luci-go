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

package resultdb

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRecorderClient(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	taskID := "65aba3a3e6b99311"
	invID := "task-example.appspot.com-65aba3a3e6b99311"
	invName := "invocations/task-example.appspot.com-65aba3a3e6b99311"

	ftt.Run("CreateInvocation", t, func(t *ftt.Test) {
		realm := "project:bucket"
		deadline := testclock.TestRecentTimeLocal
		req := &rdbpb.CreateInvocationRequest{
			InvocationId: invID,
			Invocation: &rdbpb.Invocation{
				ProducerResource: fmt.Sprintf("//example.appspot.com/tasks/%s", taskID),
				Realm:            realm,
				Deadline:         timestamppb.New(deadline),
			},
		}
		t.Run("OK", func(t *ftt.Test) {
			token := "token for 65aba3a3e6b99310"
			inv := &rdbpb.Invocation{
				Name: invName,
			}
			recorder := newMockedRecorder(ctx, req, inv, nil, token)
			actualInvName, token, err := recorder.CreateInvocation(ctx, taskID, realm, deadline)
			assert.NoErr(t, err)
			assert.That(t, token, should.Equal(token))
			assert.That(t, actualInvName, should.Equal(invName))
		})

		t.Run("no_update_token", func(t *ftt.Test) {
			recorder := newMockedRecorder(ctx, req, nil, nil, "")
			_, _, err := recorder.CreateInvocation(ctx, taskID, realm, deadline)
			assert.That(t, err, should.ErrLike("missing update token"))
		})

		t.Run("fail", func(t *ftt.Test) {
			realm := "project:bucket"
			deadline := testclock.TestRecentTimeLocal

			req := &rdbpb.CreateInvocationRequest{
				InvocationId: invID,
				Invocation: &rdbpb.Invocation{
					ProducerResource: fmt.Sprintf("//example.appspot.com/tasks/%s", taskID),
					Realm:            realm,
					Deadline:         timestamppb.New(deadline),
				},
			}
			recorder := newMockedRecorder(ctx, req, nil, status.Errorf(codes.PermissionDenied, "boom"), "")
			_, _, err := recorder.CreateInvocation(ctx, taskID, realm, deadline)
			assert.That(t, err, should.ErrLike("boom"))
		})
	})

	ftt.Run("FinalizeInvocation", t, func(t *ftt.Test) {
		t.Run("OK", func(t *ftt.Test) {
			updateToken := "token for 65aba3a3e6b99310"
			req := &rdbpb.FinalizeInvocationRequest{
				Name: invName,
			}
			inv := &rdbpb.Invocation{}
			recorder := newMockedRecorder(ctx, req, inv, nil, updateToken)
			assert.NoErr(t, recorder.FinalizeInvocation(ctx, invName, updateToken))
		})

		t.Run("fail", func(t *ftt.Test) {
			updateToken := "token for 65aba3a3e6b99310"
			req := &rdbpb.FinalizeInvocationRequest{
				Name: invName,
			}
			recorder := newMockedRecorder(
				ctx, req, nil, status.Errorf(codes.Internal, "internal"), "")
			err := recorder.FinalizeInvocation(ctx, invName, updateToken)
			assert.That(t, err, should.ErrLike("internal"))
		})
	})
}

func TestExtractHostname(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		h, _ := extractHostname("http://resultdb.example.com")
		assert.That(t, h, should.Equal("resultdb.example.com"))
	})
}

func newMockedRecorder(ctx context.Context, expectedRequest proto.Message, inv *rdbpb.Invocation, err error, updateToken string) *RecorderClient {
	mcf := NewMockRecorderClientFactory(expectedRequest, inv, err, updateToken)
	recorder, _ := mcf.MakeClient(ctx, "rdbhost", "project")
	return recorder
}
