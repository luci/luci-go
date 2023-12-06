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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/authtest"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSwarmingServer(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		ctx := memory.Use(context.Background())
		ctx = MockRequestState(ctx, MockedRequestState{
			Caller:  "user:caller@example.com",
			AuthDB:  authtest.NewFakeDB(),
			Configs: MockedConfigs{},
		})

		srv := SwarmingServer{}

		Convey("GetPermissions OK", func() {
			resp, err := srv.GetPermissions(ctx, &apipb.PermissionsRequest{
				BotId:  "bot-id",
				TaskId: "task-id",
				Tags:   []string{"pool:pool-0", "pool:pool-1", "other:tag"},
			})
			So(err, ShouldBeNil)
			// TODO(vadimsh): Implement.
			So(resp, ShouldResembleProto, &apipb.ClientPermissions{
				DeleteBot:         true,
				DeleteBots:        true,
				TerminateBot:      true,
				GetConfigs:        false,
				PutConfigs:        false,
				CancelTask:        true,
				GetBootstrapToken: false,
				CancelTasks:       true,
				ListBots:          []string{},
				ListTasks:         []string{},
			})
		})
	})
}
