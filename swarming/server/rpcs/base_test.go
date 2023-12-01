// Copyright 2023 The LUCI Authors.
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

package rpcs

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/gae/impl/memory"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServerInterceptor(t *testing.T) {
	t.Parallel()

	Convey("With config in datastore", t, func() {
		ctx := memory.Use(context.Background())
		cfg := MockConfigs(ctx, MockedConfigs{})

		interceptor := ServerInterceptor(cfg, []*grpc.ServiceDesc{
			&apipb.Swarming_ServiceDesc,
		})

		Convey("Sets up state", func() {
			var state *RequestState
			err := interceptor(ctx, "/swarming.v2.Swarming/GetPermissions", func(ctx context.Context) error {
				state = State(ctx)
				return nil
			})
			So(err, ShouldBeNil)
			So(state, ShouldNotBeNil)
			So(state.Config, ShouldNotBeNil)
			So(state.ACL, ShouldNotBeNil)
		})

		Convey("Skips unrelated APIs", func() {
			var called bool
			err := interceptor(ctx, "/another.Service/GetPermissions", func(ctx context.Context) error {
				called = true
				defer func() { So(recover(), ShouldNotBeNil) }()
				State(ctx) // panics
				return nil
			})
			So(err, ShouldBeNil)
			So(called, ShouldBeTrue)
		})
	})
}
