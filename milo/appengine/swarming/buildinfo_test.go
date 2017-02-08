// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"testing"

	swarming "github.com/luci/luci-go/common/api/swarming/swarming/v1"
	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	milo "github.com/luci/luci-go/milo/api/proto"

	"github.com/luci/gae/impl/memory"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

// testLogDogClient is a minimal functional LogsClient implementation.
//
// It retains its latest input parameter and returns its configured err (if not
// nil) or resp.
type testLogDogClient struct {
	logdog.LogsClient

	req  interface{}
	resp interface{}
	err  error
}

func (tc *testLogDogClient) Tail(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (
	*logdog.GetResponse, error) {

	tc.req = in
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.resp.(*logdog.GetResponse), nil
}

type testSwarmingService struct {
	swarmingService

	host string
	req  swarming.SwarmingRpcsTaskRequest
	res  swarming.SwarmingRpcsTaskResult
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

func datagramGetResponse(project, prefix string, msg proto.Message) *logdog.GetResponse {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return &logdog.GetResponse{
		Project: project,
		Desc: &logpb.LogStreamDescriptor{
			Prefix:      prefix,
			ContentType: miloProto.ContentTypeAnnotations,
			StreamType:  logpb.StreamType_DATAGRAM,
		},
		State: &logdog.LogStreamState{},
		Logs: []*logpb.LogEntry{
			{
				Content: &logpb.LogEntry_Datagram{
					Datagram: &logpb.Datagram{
						Data: data,
					},
				},
			},
		},
	}
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
				TaskId: "12345",
				State:  TaskRunning,
				Tags: []string{
					"allow_milo:1",
					"foo:1",
					"bar:2",
				},
			},
		}
		bip := BuildInfoProvider{
			LogdogClientFunc: func(context.Context) (*coordinator.Client, error) {
				return &coordinator.Client{
					C:    &testClient,
					Host: "example.com",
				}, nil
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
					Task: "12345",
				},
			},
		}

		Convey("Will fail to load a non-Kitchen build.", func() {
			testSvc.req.Properties.Command = []string{"not", "kitchen"}

			_, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeRPCNotFound)
		})

		Convey("Can load a build, inferring project from Kitchen CLI.", func() {
			testClient.resp = datagramGetResponse("testproject", "swarm/swarming.example.com/12345", &logdogStep)

			resp, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, &logdog.TailRequest{
				Project: "testproject",
				Path:    "swarm/swarming.example.com/12345/+/annotations",
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
						Label: "Task 12345",
						Value: &miloProto.Link_Url{
							Url: "https://swarming.example.com/user/task/12345",
						},
					},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "swarm/swarming.example.com/12345",
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
			testClient.resp = datagramGetResponse("testproject", "swarm/swarming.example.com/12345", &logdogStep)

			resp, err := bip.GetBuildInfo(c, biReq.GetSwarming(), "testproject")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, &logdog.TailRequest{
				Project: "testproject",
				Path:    "swarm/swarming.example.com/12345/+/annotations",
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
						Label: "Task 12345",
						Value: &miloProto.Link_Url{
							Url: "https://swarming.example.com/user/task/12345",
						},
					},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "swarm/swarming.example.com/12345",
					Name:   "annotations",
				},
			})
		})
	})
}
