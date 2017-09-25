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

package buildbot

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/milo/api/buildbot"
	milo "go.chromium.org/luci/milo/api/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

		build := buildbot.Build{
			Master:      "foo master",
			Buildername: "bar builder",
			Number:      1337,
			Properties: []*buildbot.Property{
				{Name: "foo", Value: "build-foo"},
				{Name: "bar", Value: "build-bar"},
			},
		}

		// mark foo master as public
		So(ds.Put(c, &buildbotMasterPublic{"foo master"}), ShouldBeNil)

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

		Convey("Load an invalid build", func() {
			_, err := bip.GetBuildInfo(c,
				&milo.BuildInfoRequest_BuildBot{
					MasterName:  "foo master",
					BuilderName: "bar builder",
					BuildNumber: 1334,
				}, "")
			So(err, ShouldErrLike,
				"rpc error: code = NotFound desc = Build #1334 for master \"foo master\", builder \"bar builder\" was not found")
		})

		Convey("Can load a BuildBot build by log location.", func() {
			build.Properties = append(build.Properties, []*buildbot.Property{
				{Name: "log_location", Value: "logdog://example.com/testproject/foo/bar/+/baz/annotations"},
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
						{Name: "log_location", Value: "logdog://example.com/testproject/foo/bar/+/baz/annotations"},
					},
				},
				AnnotationStream: &miloProto.LogdogStream{
					Server: "example.com",
					Prefix: "foo/bar",
					Name:   "baz/annotations",
				},
			})
		})

		Convey("Can load a BuildBot build by annotation URL.", func() {
			build.Properties = append(build.Properties, []*buildbot.Property{
				{Name: "log_location", Value: "protocol://not/a/logdog/url"},
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
						{Name: "log_location", Value: "protocol://not/a/logdog/url"},
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
			build.Properties = append(build.Properties, []*buildbot.Property{
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
