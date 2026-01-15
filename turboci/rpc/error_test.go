// Copyright 2026 The LUCI Authors.
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
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestStageAttemptCurrentState(t *testing.T) {
	t.Parallel()

	t.Run("no error", func(t *testing.T) {
		d, err := StageAttemptCurrentState(nil)
		assert.NoErr(t, err)
		assert.Loosely(t, d, should.BeNil)
	})

	t.Run("not grpc error", func(t *testing.T) {
		_, err := StageAttemptCurrentState(errors.New("random error"))
		assert.ErrIsLike(t, err, "err random error is not a grpc error")
	})

	t.Run("not contain StageAttemptCurrentState", func(t *testing.T) {
		d, err := StageAttemptCurrentState(status.Error(codes.Internal, "internal error"))
		assert.NoErr(t, err)
		assert.Loosely(t, d, should.BeNil)
	})

	t.Run("contains StageAttemptCurrentState", func(t *testing.T) {
		s := status.New(codes.FailedPrecondition, "error")
		sacs := orchestratorpb.StageAttemptCurrentState_builder{
			State: orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(),
		}.Build()

		s, err := s.WithDetails(sacs)
		assert.NoErr(t, err)

		d, err := StageAttemptCurrentState(s.Err())
		assert.NoErr(t, err)
		assert.That(t, d, should.Match(sacs))
	})
}
