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

	lucierr "go.chromium.org/luci/common/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

		Convey("For multi-errors", func() {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := lucierr.NewMultiError(errGRPCNotFound, errGRPCInvalidArgument)
			So(Code(errMulti), ShouldEqual, codes.Unknown)
		})
	})
}
