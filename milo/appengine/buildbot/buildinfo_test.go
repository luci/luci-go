// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"testing"

	miloProto "github.com/luci/luci-go/common/proto/milo"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/coordinator"
	milo "github.com/luci/luci-go/milo/api/proto"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"

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

	req  []interface{}
	resp []interface{}
	err  error
}

func (tc *testLogDogClient) popResp() (resp interface{}) {
	resp, tc.resp = tc.resp[0], tc.resp[1:]
	return
}

func (tc *testLogDogClient) Tail(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (
	*logdog.GetResponse, error) {

	tc.req = append(tc.req, in)
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.popResp().(*logdog.GetResponse), nil
}

// Query returns log stream paths that match the requested query.
func (tc *testLogDogClient) Query(ctx context.Context, in *logdog.QueryRequest, opts ...grpc.CallOption) (
	*logdog.QueryResponse, error) {

	tc.req = append(tc.req, in)
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.popResp().(*logdog.QueryResponse), nil
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
		bip := BuildInfoProvider{
			LogdogClientFunc: func(context.Context) (*coordinator.Client, error) {
				return &coordinator.Client{
					C:    &testClient,
					Host: "example.com",
				}, nil
			},
		}

		build := buildbotBuild{
			Master:      "foo master",
			Buildername: "bar builder",
			Number:      1337,
			Properties: []*buildbotProperty{
				{Name: "foo", Value: "build-foo"},
				{Name: "bar", Value: "build-bar"},
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
			Build: &milo.BuildInfoRequest_Buildbot{
				Buildbot: &milo.BuildInfoRequest_BuildBot{
					MasterName:  "foo master",
					BuilderName: "bar builder",
					BuildNumber: 1337,
				},
			},
		}

		Convey("Can load a BuildBot build by annotation URL.", func() {
			build.Properties = append(build.Properties, []*buildbotProperty{
				{Name: "logdog_annotation_url", Value: "logdog://example.com/testproject/foo/bar/+/baz/annotations"},
			}...)
			So(ds.Put(c, &build), ShouldBeNil)
			testClient.resp = []interface{}{
				datagramGetResponse("testproject", "foo/bar", &logdogStep),
			}

			resp, err := bip.GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, []interface{}{
				&logdog.TailRequest{
					Project: "testproject",
					Path:    "foo/bar/+/baz/annotations",
					State:   true,
				},
			})
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
						{Name: "logdog_annotation_url", Value: "logdog://example.com/testproject/foo/bar/+/baz/annotations"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "foo/bar",
					Name:   "baz/annotations",
				},
			})
		})

		Convey("Can load a BuildBot build by tag.", func() {
			build.Properties = append(build.Properties, []*buildbotProperty{
				{Name: "logdog_prefix", Value: "foo/bar"},
				{Name: "logdog_project", Value: "testproject"},
			}...)
			So(ds.Put(c, &build), ShouldBeNil)
			testClient.resp = []interface{}{
				datagramGetResponse("testproject", "foo/bar", &logdogStep),
			}

			resp, err := bip.GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, []interface{}{
				&logdog.TailRequest{
					Project: "testproject",
					Path:    "foo/bar/+/annotations",
					State:   true,
				},
			})
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
						{Name: "logdog_prefix", Value: "foo/bar"},
						{Name: "logdog_project", Value: "testproject"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "foo/bar",
					Name:   "annotations",
				},
			})
		})

		Convey("Fails to load a BuildBot build by query if no project hint is provided.", func() {
			So(ds.Put(c, &build), ShouldBeNil)

			_, err := bip.GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldErrLike, "annotation stream not found")
		})

		Convey("Can load a BuildBot build by query with a project hint.", func() {
			So(ds.Put(c, &build), ShouldBeNil)
			testClient.resp = []interface{}{
				&logdog.QueryResponse{
					Streams: []*logdog.QueryResponse_Stream{
						{
							Path: "foo/bar/+/annotations",
						},
						{
							Path: "other/ignore/+/me",
						},
					},
				},
				datagramGetResponse("testproject", "foo/bar/+/annotations", &logdogStep),
			}

			resp, err := bip.GetBuildInfo(c, biReq.GetBuildbot(), "testproject")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, []interface{}{
				&logdog.QueryRequest{
					Project:     "testproject",
					ContentType: miloProto.ContentTypeAnnotations,
					Tags: map[string]string{
						"buildbot.master":      "foo master",
						"buildbot.builder":     "bar builder",
						"buildbot.buildnumber": "1337",
					},
				},
				&logdog.TailRequest{
					Project: "testproject",
					Path:    "foo/bar/+/annotations",
					State:   true,
				},
			})

			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "foo/bar",
					Name:   "annotations",
				},
			})
		})

		Convey("Can load a BuildBot build by inferred name.", func() {
			So(ds.Put(c, &build), ShouldBeNil)
			testClient.resp = []interface{}{
				&logdog.QueryResponse{},
				datagramGetResponse("testproject", "foo/bar/+/annotations", &logdogStep),
			}

			resp, err := bip.GetBuildInfo(c, biReq.GetBuildbot(), "testproject")
			So(err, ShouldBeNil)
			So(testClient.req, ShouldResemble, []interface{}{
				&logdog.QueryRequest{
					Project:     "testproject",
					ContentType: miloProto.ContentTypeAnnotations,
					Tags: map[string]string{
						"buildbot.master":      "foo master",
						"buildbot.builder":     "bar builder",
						"buildbot.buildnumber": "1337",
					},
				},
				&logdog.TailRequest{
					Project: "testproject",
					Path:    "bb/foo_master/bar_builder/1337/+/annotations",
					State:   true,
				},
			})

			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "testproject",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "bb/foo_master/bar_builder/1337",
					Name:   "annotations",
				},
			})
		})
	})
}
