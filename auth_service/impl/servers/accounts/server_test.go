// Copyright 2021 The LUCI Authors.
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

package accounts

import (
	"context"
	"net"
	"testing"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/auth_service/api/rpcpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAccountsServer(t *testing.T) {
	t.Parallel()

	Convey("With server", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{})
		srv := Server{}

		Convey("GetSelf anonymous", func() {
			resp, err := srv.GetSelf(ctx, &emptypb.Empty{})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &rpcpb.SelfInfo{
				Identity: "anonymous:anonymous",
				Ip:       "127.0.0.1",
			})
		})

		Convey("GetSelf authenticated", func() {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity:       "user:someone@example.com",
				PeerIPOverride: net.ParseIP("192.168.0.1"),
			})
			resp, err := srv.GetSelf(ctx, &emptypb.Empty{})
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, &rpcpb.SelfInfo{
				Identity: "user:someone@example.com",
				Ip:       "192.168.0.1",
			})
		})
	})
}
