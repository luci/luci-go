// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestACLDecorator(t *testing.T) {
	t.Parallel()

	anon := identity.AnonymousIdentity
	someone := identity.Identity("user:someone@example.com")
	admin := identity.Identity("user:admin@example.com")

	state := &authtest.FakeState{
		FakeDB: authtest.FakeDB{
			admin: {"administrators"},
		},
	}
	ctx := auth.WithState(context.Background(), state)

	var cases = []struct {
		method  string
		caller  identity.Identity
		request proto.Message
		allowed bool
	}{
		{"GetObjectURL", anon, nil, false},
		{"GetObjectURL", someone, nil, false},
		{"GetObjectURL", admin, nil, true},
	}

	for _, cs := range cases {
		Convey(fmt.Sprintf("%s by %s", cs.method, cs.caller), t, func() {
			state.Identity = cs.caller
			_, err := aclPrelude(ctx, cs.method, cs.request)
			if cs.allowed {
				So(err, ShouldBeNil)
			} else {
				So(grpc.Code(err), ShouldEqual, codes.PermissionDenied)
			}
		})
	}
}
