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

package swarming

import (
	"testing"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/fakelogs"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"

	"go.chromium.org/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testSwarmingService struct {
	host string
	req  swarming.SwarmingRpcsTaskRequest
	res  swarming.SwarmingRpcsTaskResult
	out  string
}

func (sf *testSwarmingService) GetHost() string { return sf.host }

func (sf *testSwarmingService) GetSwarmingResult(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskResult, error) {

	return &sf.res, nil
}

func (sf *testSwarmingService) GetSwarmingRequest(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskRequest, error) {

	return &sf.req, nil
}

func (sf *testSwarmingService) GetTaskOutput(c context.Context, taskID string) (string, error) {
	return sf.out, nil
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()

	Convey("A testing BuildInfoProvider", t, func() {
		c := context.Background()
		c = memory.Use(c)
		c = rawpresentation.InjectFakeLogdogClient(c, fakelogs.NewClient())

		testSvc := testSwarmingService{
			host: "swarming.example.com",
			req: swarming.SwarmingRpcsTaskRequest{
				Properties: &swarming.SwarmingRpcsTaskProperties{
					Command: []string{"kitchen", "foo", "bar", "-logdog-project", "testproject", "baz"},
				},
				Tags: []string{
					"allow_milo:1",
				},
			},
			res: swarming.SwarmingRpcsTaskResult{
				TaskId: "12340",
				State:  TaskRunning,
				Tags: []string{
					"allow_milo:1",
					"foo:1",
					"bar:2",
				},
				TryNumber: 1,
			},
		}
		bip := BuildInfoProvider{
			swarmingServiceFunc: func(context.Context, string) (SwarmingService, error) {
				return &testSvc, nil
			},
		}

		biReq := milo.BuildInfoRequest{
			Build: &milo.BuildInfoRequest_Swarming_{
				Swarming: &milo.BuildInfoRequest_Swarming{
					Task: "12340",
				},
			},
		}

		Convey("Will fail to load a non-Kitchen build.", func() {
			testSvc.req.Properties.Command = []string{"not", "kitchen"}

			_, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeRPCNotFound)
		})

		Convey("Will fail to load Kitchen without LogDog and no project hint.", func() {
			testSvc.req.Properties.Command = []string{"kitchen"}

			_, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeRPCNotFound)
		})

	})
}
