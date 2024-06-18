// Copyright 2024 The LUCI Authors.
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

package experiments

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestEcho(t *testing.T) {
	Convey("Given an experiments server", t, func() {
		ctx := context.Background()

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"googlers"},
		}
		ctx = auth.WithState(ctx, authState)
		// ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewExperimentsServer()

		Convey("Unauthorised requests are rejected", func() {
			// Not a member of allowedGroup.
			authState.IdentityGroups = []string{"other-group"}

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.EchoRequest{
				Message: "hello",
			}

			rsp, err := server.Echo(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of googlers")
			So(rsp, ShouldBeNil)
		})

		// Valid baseline request.
		request := &pb.EchoRequest{
			Message: "hello",
		}

		Convey("Invalid requests are rejected", func() {
			Convey("Message", func() {
				Convey("Empty", func() {
					request.Message = ""

					rsp, err := server.Echo(ctx, request)
					So(err, ShouldBeRPCInvalidArgument, "message: unspecified")
					So(rsp, ShouldBeNil)
				})
				Convey("Non-printable", func() {
					request.Message = "\u0001"

					rsp, err := server.Echo(ctx, request)
					So(err, ShouldBeRPCInvalidArgument, "message: does not match")
					So(rsp, ShouldBeNil)
				})
				Convey("Too long", func() {
					request.Message = strings.Repeat("a", 1025)

					rsp, err := server.Echo(ctx, request)
					So(err, ShouldBeRPCInvalidArgument, "message: exceeds 1024 bytes")
					So(rsp, ShouldBeNil)
				})
			})
		})
		Convey("Valid requests", func() {
			rsp, err := server.Echo(ctx, request)
			So(err, ShouldBeNil)
			So(rsp, ShouldResembleProto, &pb.EchoResponse{
				Message: "hello",
			})
		})
	})
}
