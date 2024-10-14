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
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lucierr "go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCode(t *testing.T) {
	t.Parallel()

	ftt.Run("Code returns the correct error code", t, func(t *ftt.Test) {
		t.Run("For simple errors", func(t *ftt.Test) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			assert.Loosely(t, Code(errGRPCNotFound), should.Equal(codes.NotFound))
		})

		t.Run("For errors missing tags", func(t *ftt.Test) {
			err := errors.New("foobar")
			assert.Loosely(t, Code(err), should.Equal(codes.Unknown))
		})

		t.Run("For wrapped errors", func(t *ftt.Test) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errWrapped := lucierr.Annotate(errGRPCNotFound, "wrapped").Err()
			assert.Loosely(t, Code(errWrapped), should.Equal(codes.NotFound))
		})

		t.Run("Multi-errors with multiple different codes return Unknown", func(t *ftt.Test) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCNotFound, errGRPCInvalidArgument)
			assert.Loosely(t, Code(errMulti), should.Equal(codes.Unknown))
		})

		t.Run("Multi-errors with one error return that error's code", func(t *ftt.Test) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument)
			assert.Loosely(t, Code(errMulti), should.Equal(codes.InvalidArgument))
		})

		t.Run("Multi-errors with the same code return that code", func(t *ftt.Test) {
			errGRPCInvalidArgument1 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errGRPCInvalidArgument2 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument1, errGRPCInvalidArgument2)
			assert.Loosely(t, Code(errMulti), should.Equal(codes.InvalidArgument))
		})

		t.Run("Nested multi-errors work correctly", func(t *ftt.Test) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti1 := lucierr.NewMultiError(errGRPCInvalidArgument)
			errMulti2 := lucierr.NewMultiError(errMulti1)
			assert.Loosely(t, Code(errMulti2), should.Equal(codes.InvalidArgument))
		})
	})
}

func TestWrapIfTransient(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		newErr := func(code codes.Code) error { return status.Errorf(code, "...") }
		check := func(err error) bool { return transient.Tag.In(err) }

		assert.Loosely(t, check(WrapIfTransient(newErr(codes.Internal))), should.BeTrue)
		assert.Loosely(t, check(WrapIfTransient(newErr(codes.Unknown))), should.BeTrue)
		assert.Loosely(t, check(WrapIfTransient(newErr(codes.Unavailable))), should.BeTrue)

		assert.Loosely(t, check(WrapIfTransient(nil)), should.BeFalse)
		assert.Loosely(t, check(WrapIfTransient(newErr(codes.FailedPrecondition))), should.BeFalse)

		assert.Loosely(t, check(WrapIfTransientOr(newErr(codes.DeadlineExceeded))), should.BeFalse)
		assert.Loosely(t, check(WrapIfTransientOr(newErr(codes.DeadlineExceeded), codes.DeadlineExceeded)), should.BeTrue)
	})
}
