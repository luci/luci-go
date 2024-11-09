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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestEcho(t *testing.T) {
	ftt.Run("Given an experiments server", t, func(t *ftt.Test) {
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

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			// Not a member of allowedGroup.
			authState.IdentityGroups = []string{"other-group"}

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.EchoRequest{
				Message: "hello",
			}

			rsp, err := server.Echo(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not a member of googlers"))
			assert.Loosely(t, rsp, should.BeNil)
		})

		// Valid baseline request.
		request := &pb.EchoRequest{
			Message: "hello",
		}

		t.Run("Invalid requests are rejected", func(t *ftt.Test) {
			t.Run("Message", func(t *ftt.Test) {
				t.Run("Empty", func(t *ftt.Test) {
					request.Message = ""

					rsp, err := server.Echo(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("message: unspecified"))
					assert.Loosely(t, rsp, should.BeNil)
				})
				t.Run("Non-printable", func(t *ftt.Test) {
					request.Message = "\u0001"

					rsp, err := server.Echo(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("message: does not match"))
					assert.Loosely(t, rsp, should.BeNil)
				})
				t.Run("Too long", func(t *ftt.Test) {
					request.Message = strings.Repeat("a", 1025)

					rsp, err := server.Echo(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("message: exceeds 1024 bytes"))
					assert.Loosely(t, rsp, should.BeNil)
				})
			})
		})
		t.Run("Valid requests", func(t *ftt.Test) {
			rsp, err := server.Echo(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Resemble(&pb.EchoResponse{
				Message: "hello",
			}))
		})
	})
}
