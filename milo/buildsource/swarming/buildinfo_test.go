// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"fmt"
	"testing"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/client/coordinator"
	milo "github.com/luci/luci-go/milo/api/proto"

	"github.com/luci/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
