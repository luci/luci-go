// Copyright 2020 The LUCI Authors.
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

package grpcutil

import (
	"errors"
	"fmt"
	"testing"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lucierr "go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCode(t *testing.T) {
	t.Parallel()

	t.Run("Code returns the correct error code", func(t *testing.T) {
		t.Run("For simple errors", func(t *testing.T) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			assert.That(t, Code(errGRPCNotFound), should.Equal(codes.NotFound))
		})

		t.Run("For errors missing tags", func(t *testing.T) {
			err := errors.New("foobar")
			assert.That(t, Code(err), should.Equal(codes.Unknown))
		})

		t.Run("For wrapped errors", func(t *testing.T) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errWrapped := lucierr.Fmt("wrapped: %w", errGRPCNotFound)
			assert.That(t, Code(errWrapped), should.Equal(codes.NotFound))
		})

		t.Run("Multi-errors with multiple different codes return Unknown", func(t *testing.T) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCNotFound, errGRPCInvalidArgument)
			assert.That(t, Code(errMulti), should.Equal(codes.Unknown))
		})

		t.Run("Multi-errors with one error return that error's code", func(t *testing.T) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument)
			assert.That(t, Code(errMulti), should.Equal(codes.InvalidArgument))
		})

		t.Run("Multi-errors with the same code return that code", func(t *testing.T) {
			errGRPCInvalidArgument1 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errGRPCInvalidArgument2 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument1, errGRPCInvalidArgument2)
			assert.That(t, Code(errMulti), should.Equal(codes.InvalidArgument))
		})

		t.Run("Nested multi-errors work correctly", func(t *testing.T) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti1 := lucierr.NewMultiError(errGRPCInvalidArgument)
			errMulti2 := lucierr.NewMultiError(errMulti1)
			assert.That(t, Code(errMulti2), should.Equal(codes.InvalidArgument))
		})
	})
}

func TestWrapIfTransient(t *testing.T) {
	t.Parallel()

	t.Run("Works", func(t *testing.T) {
		newErr := func(code codes.Code) error { return status.Errorf(code, "...") }
		check := func(err error) bool { return transient.Tag.In(err) }

		assert.That(t, check(WrapIfTransient(newErr(codes.Internal))), should.BeTrue)
		assert.That(t, check(WrapIfTransient(newErr(codes.Unknown))), should.BeTrue)
		assert.That(t, check(WrapIfTransient(newErr(codes.Unavailable))), should.BeTrue)

		assert.That(t, check(WrapIfTransient(nil)), should.BeFalse)
		assert.That(t, check(WrapIfTransient(newErr(codes.FailedPrecondition))), should.BeFalse)

		assert.That(t, check(WrapIfTransientOr(newErr(codes.DeadlineExceeded))), should.BeFalse)
		assert.That(t, check(WrapIfTransientOr(newErr(codes.DeadlineExceeded), codes.DeadlineExceeded)), should.BeTrue)
	})
}

func TestGRPCifyAndLogErr(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("Pass through native", func(t *testing.T) {
		origErr := status.Errorf(codes.Aborted, "Aborted")
		ifiedErr := GRPCifyAndLogErr(ctx, origErr)

		// The exact same error object.
		assert.That(t, origErr == ifiedErr, should.BeTrue)
	})

	t.Run("Pass through native wrapped", func(t *testing.T) {
		origErr := fmt.Errorf("bzz: %w", status.Errorf(codes.Aborted, "Aborted"))
		ifiedErr := GRPCifyAndLogErr(ctx, origErr)

		// The exact same error.
		assert.That(t, origErr == ifiedErr, should.BeTrue)
	})

	t.Run("Collapses code-tagged", func(t *testing.T) {
		origErr := fmt.Errorf("bzz: %w", status.Errorf(codes.Aborted, "Aborted"))
		origErr = InvalidArgumentTag.Apply(origErr)
		ifiedErr := GRPCifyAndLogErr(ctx, origErr)

		// Replaced the code and stringified the error in an awkward way.
		st, ok := status.FromError(ifiedErr)
		assert.That(t, ok, should.BeTrue)
		assert.That(t, st.Proto(), should.Match(&statuspb.Status{
			Code:    int32(codes.InvalidArgument),
			Message: "bzz: rpc error: code = Aborted desc = Aborted",
		}))
	})
}
