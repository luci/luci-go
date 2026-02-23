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

package appstatus

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStatusFromError(t *testing.T) {
	ctx := t.Context()

	t.Run("Already has appstatus", func(t *testing.T) {
		err := Error(codes.NotFound, "not found")
		st := statusFromError(ctx, err)
		assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
		assert.Loosely(t, st.Message(), should.Equal("not found"))
	})

	t.Run("Context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()

		t.Run("Error unwraps to context.Canceled", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", context.Canceled)
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Canceled))
			assert.Loosely(t, st.Message(), should.Equal(context.Canceled.Error()))
		})

		t.Run("Error unwraps to context.Canceled (legacy)", func(t *testing.T) {
			err := errors.Annotate(context.Canceled, "some reason").Err()
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Canceled))
			assert.Loosely(t, st.Message(), should.Equal(context.Canceled.Error()))
		})

		t.Run("Error is gRPC Canceled", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", status.Error(codes.Canceled, "remote canceled"))
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Canceled))
			assert.Loosely(t, st.Message(), should.Equal(context.Canceled.Error()))
		})

		t.Run("Error is unrelated", func(t *testing.T) {
			err := fmt.Errorf("some generic error")
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Internal))
			assert.Loosely(t, st.Message(), should.Equal("internal server error"))
		})

		t.Run("Context canceled with cause", func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(errors.New("some cause"))

			err := fmt.Errorf("wrapped: %w", context.Canceled)
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Canceled))
			assert.Loosely(t, st.Message(), should.Equal("some cause"))
		})
	})

	t.Run("Context deadline exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 0)
		defer cancel()
		<-ctx.Done()

		t.Run("Error unwraps to context.DeadlineExceeded", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.DeadlineExceeded))
			assert.Loosely(t, st.Message(), should.Equal(context.DeadlineExceeded.Error()))
		})

		t.Run("Error is gRPC DeadlineExceeded", func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", status.Error(codes.DeadlineExceeded, "remote deadline exceeded"))
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.DeadlineExceeded))
			assert.Loosely(t, st.Message(), should.Equal(context.DeadlineExceeded.Error()))
		})

		t.Run("Error is unrelated", func(t *testing.T) {
			err := fmt.Errorf("some generic error")
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Internal))
			assert.Loosely(t, st.Message(), should.Equal("internal server error"))
		})

		t.Run("Deadline exceeded with cause", func(t *testing.T) {
			ctx, cancel := context.WithTimeoutCause(t.Context(), 0, errors.New("some cause"))
			defer cancel()
			<-ctx.Done()

			err := fmt.Errorf("wrapped: %w", context.DeadlineExceeded)
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.DeadlineExceeded))
			assert.Loosely(t, st.Message(), should.Equal("some cause"))
		})
	})

	t.Run("Context active", func(t *testing.T) {
		t.Run("Generic error", func(t *testing.T) {
			err := fmt.Errorf("some generic error")
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Internal))
			assert.Loosely(t, st.Message(), should.Equal("internal server error"))
		})
		t.Run("Error is gRPC Canceled (not mapped because ctx is active)", func(t *testing.T) {
			err := status.Error(codes.Canceled, "remote canceled")
			st := statusFromError(ctx, err)
			assert.Loosely(t, st.Code(), should.Equal(codes.Internal))
			assert.Loosely(t, st.Message(), should.Equal("internal server error"))
		})
	})
}
