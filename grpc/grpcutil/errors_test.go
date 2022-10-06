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

	. "github.com/smartystreets/goconvey/convey"
)

func TestCode(t *testing.T) {
	t.Parallel()

	Convey("Code returns the correct error code", t, func() {
		Convey("For simple errors", func() {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			So(Code(errGRPCNotFound), ShouldEqual, codes.NotFound)
		})

		Convey("For errors missing tags", func() {
			err := errors.New("foobar")
			So(Code(err), ShouldEqual, codes.Unknown)
		})

		Convey("For wrapped errors", func() {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errWrapped := lucierr.Annotate(errGRPCNotFound, "wrapped").Err()
			So(Code(errWrapped), ShouldEqual, codes.NotFound)
		})

		Convey("Multi-errors with multiple different codes return Unknown", func() {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCNotFound, errGRPCInvalidArgument)
			So(Code(errMulti), ShouldEqual, codes.Unknown)
		})

		Convey("Multi-errors with one error return that error's code", func() {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument)
			So(Code(errMulti), ShouldEqual, codes.InvalidArgument)
		})

		Convey("Multi-errors with the same code return that code", func() {
			errGRPCInvalidArgument1 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errGRPCInvalidArgument2 := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCInvalidArgument1, errGRPCInvalidArgument2)
			So(Code(errMulti), ShouldEqual, codes.InvalidArgument)
		})

		Convey("Nested multi-errors work correctly", func() {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti1 := lucierr.NewMultiError(errGRPCInvalidArgument)
			errMulti2 := lucierr.NewMultiError(errMulti1)
			So(Code(errMulti2), ShouldEqual, codes.InvalidArgument)
		})
	})
}

func TestWrapIfTransient(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		newErr := func(code codes.Code) error { return status.Errorf(code, "...") }
		check := func(err error) bool { return transient.Tag.In(err) }

		So(check(WrapIfTransient(newErr(codes.Internal))), ShouldBeTrue)
		So(check(WrapIfTransient(newErr(codes.Unknown))), ShouldBeTrue)
		So(check(WrapIfTransient(newErr(codes.Unavailable))), ShouldBeTrue)

		So(check(WrapIfTransient(nil)), ShouldBeFalse)
		So(check(WrapIfTransient(newErr(codes.FailedPrecondition))), ShouldBeFalse)

		So(check(WrapIfTransientOr(newErr(codes.DeadlineExceeded))), ShouldBeFalse)
		So(check(WrapIfTransientOr(newErr(codes.DeadlineExceeded), codes.DeadlineExceeded)), ShouldBeTrue)
	})
}
