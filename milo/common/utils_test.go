// Copyright 2019 The LUCI Authors.
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

package common

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"
)

var errGRPCNotFound = status.Errorf(codes.NotFound, "not found")

func testTagGRPC(t *testing.T) {
	t.Parallel()

	Convey("GRPC Tagging Works", t, func() {
		c := memory.Use(context.Background())
		cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:user@example.com"})
		cAnon := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		So(grpcutil.Code(TagGRPC(cAnon, errGRPCNotFound)), ShouldEqual, codes.Unauthenticated)
		So(grpcutil.Code(TagGRPC(cUser, errGRPCNotFound)), ShouldEqual, codes.NotFound)
	})
}
