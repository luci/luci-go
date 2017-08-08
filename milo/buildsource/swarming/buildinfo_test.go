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
	"fmt"
	"testing"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/client/coordinator"
	milo "go.chromium.org/luci/milo/api/proto"

	"go.chromium.org/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testSwarmingService struct {
	swarmingService

	host string
	req  swarming.SwarmingRpcsTaskRequest
	res  swarming.SwarmingRpcsTaskResult
	out  string
}

func (sf *testSwarmingService) getHost() string { return sf.host }

func (sf *testSwarmingService) getSwarmingResult(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskResult, error) {

	return &sf.res, nil
}

func (sf *testSwarmingService) getSwarmingRequest(c context.Context, taskID string) (
	*swarming.SwarmingRpcsTaskRequest, error) {

	return &sf.req, nil
}

func (sf *testSwarmingService) getTaskOutput(c context.Context, taskID string) (string, error) {
	return sf.out, nil
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()

	Convey("A testing BuildInfoProvider", t, func() {
		c := context.Background()
		c = memory.Use(c)

		testClient := testLogDogClient{}
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
			bl: BuildLoader{
				logDogClientFunc: func(c context.Context, host string) (*coordinator.Client, error) {
					if host == "" {
						host = "example.com"
					}
					return &coordinator.Client{
						C:    &testClient,
						Host: host,
					}, nil
				},
			},
			swarmingServiceFunc: func(context.Context, string) (swarmingService, error) {
				return &testSvc, nil
			},
		}

		logdogStep := miloProto.Step{
			Command: &miloProto.Step_Command{
				CommandLine: []string{"foo", "bar", "baz"},
			},
			Text: []string{"test step"},
			Property: []*miloProto.Step_Property{
				{Name: "bar", Value: "log-bar"},
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

		Convey("Can load a build, inferring project from Kitchen CLI.", func() {
			testClient.resp = datagramGetResponse("testproject", "swarm/swarming.example.com/12341", &logdogStep)

			resp, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, &logdog.TailRequest{
				Project: "testproject",
				Path:    "swarm/swarming.example.com/12341/+/annotations",
				State:   true,
			})
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Link: &miloProto.Link{
						Label: "Task 12340",
						Value: &miloProto.Link_Url{
							Url: "https://swarming.example.com/task?id=12340&show_raw=1&wide_logs=true",
						},
					},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "swarm/swarming.example.com/12341",
					Name:   "annotations",
				},
			})
		})

		Convey("Will fail to load Kitchen without LogDog and no project hint.", func() {
			testSvc.req.Properties.Command = []string{"kitchen"}

			_, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeRPCNotFound)
		})

		Convey("Will load Kitchen without LogDog if there is a project hint.", func() {
			biReq.ProjectHint = "testproject"
			testSvc.req.Properties.Command = []string{"kitchen"}
			testClient.resp = datagramGetResponse("testproject", "swarm/swarming.example.com/12341", &logdogStep)

			resp, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "testproject")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, &logdog.TailRequest{
				Project: "testproject",
				Path:    "swarm/swarming.example.com/12341/+/annotations",
				State:   true,
			})
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Link: &miloProto.Link{
						Label: "Task 12340",
						Value: &miloProto.Link_Url{
							Url: "https://swarming.example.com/task?id=12340&show_raw=1&wide_logs=true",
						},
					},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "swarm/swarming.example.com/12341",
					Name:   "annotations",
				},
			})

			Convey("Will return NotFound if the build is internal", func() {
				testSvc.res.Tags = testSvc.res.Tags[1:]
				testSvc.req.Tags = testSvc.req.Tags[1:]
				_, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "testproject")
				So(err, ShouldResemble, grpcutil.NotFound)
			})
		})
	})
}

func TestGetRunID(t *testing.T) {
	t.Parallel()

	successes := []struct {
		taskID    string
		tryNumber int64
		runID     string
	}{
		{"3442825749e6e110", 1, "3442825749e6e111"},
	}

	failures := []struct {
		taskID    string
		tryNumber int64
		err       string
	}{
		{"", 1, "swarming task ID is empty"},
		{"3442825749e6e110", 16, "exceeds 4 bits"},
		{"3442825749e6e11M", 1, "failed to parse hex from rune"},
	}

	Convey("Testing BuildInfo's getRunID", t, func() {
		for _, tc := range successes {
			Convey(fmt.Sprintf("Successfully processes %q / %q => %q", tc.taskID, tc.tryNumber, tc.runID), func() {
				v, err := getRunID(tc.taskID, tc.tryNumber)
				So(err, ShouldBeNil)
				So(v, ShouldEqual, tc.runID)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf("Failes to parse %q / %q (%s)", tc.taskID, tc.tryNumber, tc.err), func() {
				_, err := getRunID(tc.taskID, tc.tryNumber)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})
}
