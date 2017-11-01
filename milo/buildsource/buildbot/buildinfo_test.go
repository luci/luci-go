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
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	miloProto "go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/fakelogs"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/api/buildbot"
	milo "go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func writeDatagram(c *fakelogs.Client, prefix, path types.StreamName, msg proto.Message, tags ...map[string]string) {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	var tagMap streamproto.TagMap
	if len(tags) > 0 {
		tagMap = streamproto.TagMap(tags[0])
	}

	s, err := c.OpenDatagramStream(prefix, path, &streamproto.Flags{
		ContentType: miloProto.ContentTypeAnnotations,
		Tags:        tagMap,
	})
	if err != nil {
		panic(err)
	}
	if _, err := s.Write(data); err != nil {
		panic(err)
	}
	if err := s.Close(); err != nil {
		panic(err)
	}
}

func TestBuildInfo(t *testing.T) {
	t.Parallel()

	Convey("A testing BuildInfoProvider", t, func() {
		c := context.Background()
		c = memory.Use(c)

		testClient := fakelogs.NewClient()
		c = rawpresentation.InjectFakeLogdogClient(c, testClient)

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
		err := buildstore.SaveMaster(c, &buildbot.Master{Name: "foo master"}, false, nil)
		So(err, ShouldBeNil)

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
			_, err := GetBuildInfo(c,
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
				{Name: "log_location", Value: "logdog://example.com/proj-foo/foo/bar/+/baz/annotations"},
			}...)
			importBuild(c, &build)
			writeDatagram(testClient, "foo/bar", "baz/annotations", &logdogStep)

			resp, err := GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "proj-foo",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
						{Name: "log_location", Value: "logdog://example.com/proj-foo/foo/bar/+/baz/annotations"},
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
				{Name: "logdog_annotation_url", Value: "logdog://example.com/proj-foo/foo/bar/+/baz/annotations"},
			}...)
			importBuild(c, &build)
			writeDatagram(testClient, "foo/bar", "baz/annotations", &logdogStep)

			resp, err := GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "proj-foo",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
						{Name: "log_location", Value: "protocol://not/a/logdog/url"},
						{Name: "logdog_annotation_url", Value: "logdog://example.com/proj-foo/foo/bar/+/baz/annotations"},
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
				{Name: "logdog_project", Value: "proj-foo"},
			}...)
			importBuild(c, &build)
			writeDatagram(testClient, "foo/bar", "annotations", &logdogStep)

			resp, err := GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "proj-foo",
				Step: &miloProto.Step{
					Command: &miloProto.Step_Command{
						CommandLine: []string{"foo", "bar", "baz"},
					},
					Text: []string{"test step"},
					Property: []*miloProto.Step_Property{
						{Name: "bar", Value: "log-bar"},
						{Name: "foo", Value: "build-foo"},
						{Name: "logdog_prefix", Value: "foo/bar"},
						{Name: "logdog_project", Value: "proj-foo"},
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
			importBuild(c, &build)

			_, err = GetBuildInfo(c, biReq.GetBuildbot(), "")
			So(err, ShouldErrLike, "annotation stream not found")
		})

		Convey("Can load a BuildBot build by query with a project hint.", func() {
			importBuild(c, &build)
			writeDatagram(testClient, "foo/bar", "annotations", &logdogStep, map[string]string{
				"buildbot.master":      build.Master,
				"buildbot.builder":     build.Buildername,
				"buildbot.buildnumber": strconv.Itoa(build.Number),
			})
			s, err := testClient.OpenTextStream("other/ignore", "me")
			So(err, ShouldBeNil)
			_, err = s.Write([]byte("Some stuff\nor\nomething"))
			So(err, ShouldBeNil)
			_, err = s.Write([]byte("some more stuff"))
			So(err, ShouldBeNil)
			So(s.Close(), ShouldBeNil)

			resp, err := GetBuildInfo(c, biReq.GetBuildbot(), "proj-foo")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "proj-foo",
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
			importBuild(c, &build)
			writeDatagram(testClient, "bb/foo_master/bar_builder/1337", "annotations", &logdogStep)

			resp, err := GetBuildInfo(c, biReq.GetBuildbot(), "proj-foo")
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &milo.BuildInfoResponse{
				Project: "proj-foo",
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
