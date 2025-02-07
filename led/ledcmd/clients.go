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

package ledcmd

import (
	"net/http"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/grpc/prpc"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func newSwarmTasksClient(authClient *http.Client, host string) swarmingpb.TasksClient {
	return swarmingpb.NewTasksClient(&prpc.Client{
		C:    authClient,
		Host: host,
	})
}

func newBuildbucketClient(authClient *http.Client, host string) bbpb.BuildsClient {
	return bbpb.NewBuildsPRPCClient(&prpc.Client{
		C:    authClient,
		Host: host,
	})
}
