// Copyright 2016 The LUCI Authors.
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

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/milo/api/buildbot"
	milo "go.chromium.org/luci/milo/api/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGRPC(t *testing.T) {
	c := memory.Use(context.Background())
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

	Convey(`A test environment`, t, func() {
		name := "testmaster"
		bname := "testbuilder"
		master := &buildbot.Master{
			Name:     name,
			Builders: map[string]*buildbot.Builder{"fake": {}},
			Slaves: map[string]*buildbot.Slave{
				"foo": {
					RunningbuildsMap: map[string][]int{
						"fake": {1},
					},
				},
			},
		}

		So(putDSMasterJSON(c, master, false), ShouldBeNil)
		So(ds.Put(c, &buildbot.Build{
			Master:      name,
			Buildername: "fake",
			Number:      1,
		}), ShouldBeNil)
		ds.GetTestable(c).Consistent(true)
		ds.GetTestable(c).AutoIndex(true)
		svc := Service{}

		Convey(`Get finished builds`, func() {
			// Add in some builds.
			for i := 0; i < 5; i++ {
				ds.Put(c, &buildbot.Build{
					Master:      name,
					Buildername: bname,
					Number:      i,
					Finished:    true,
				})
			}
			ds.Put(c, &buildbot.Build{
				Master:      name,
				Buildername: bname,
				Number:      6,
				Finished:    false,
			})
			ds.GetTestable(c).CatchupIndexes()

			r := &milo.BuildbotBuildsRequest{
				Master:  name,
				Builder: bname,
			}
			result, err := svc.GetBuildbotBuildsJSON(c, r)
			So(err, ShouldBeNil)
			So(len(result.Builds), ShouldEqual, 5)

			Convey(`Also get incomplete builds`, func() {
				r := &milo.BuildbotBuildsRequest{
					Master:         name,
					Builder:        bname,
					IncludeCurrent: true,
				}
				result, err := svc.GetBuildbotBuildsJSON(c, r)
				So(err, ShouldBeNil)
				So(len(result.Builds), ShouldEqual, 6)
			})

			Convey(`Good cursor`, func() {
				r.Cursor = result.GetCursor()
				_, err := svc.GetBuildbotBuildsJSON(c, r)
				So(err, ShouldBeNil)
			})
			Convey(`Bad cursor`, func() {
				r.Cursor = "foobar"
				_, err := svc.GetBuildbotBuildsJSON(c, r)
				So(err, ShouldResemble,
					grpc.Errorf(codes.InvalidArgument,
						"Invalid cursor: Failed to Base64-decode cursor: illegal base64 data at input byte 4"))
			})
			Convey(`Bad request`, func() {
				_, err := svc.GetBuildbotBuildsJSON(c, &milo.BuildbotBuildsRequest{})
				So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "No master specified"))
				_, err = svc.GetBuildbotBuildsJSON(c, &milo.BuildbotBuildsRequest{Master: name})
				So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "No builder specified"))
			})
		})

		Convey(`Get Master`, func() {
			Convey(`Bad request`, func() {
				_, err := svc.GetCompressedMasterJSON(c, &milo.MasterRequest{})
				So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "No master specified"))
			})
			_, err := svc.GetCompressedMasterJSON(c, &milo.MasterRequest{Name: name})
			So(err, ShouldBeNil)
		})

		Convey(`Get Build`, func() {
			Convey(`Invalid input`, func() {
				_, err := svc.GetBuildbotBuildJSON(c, &milo.BuildbotBuildRequest{})
				So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "No master specified"))
				_, err = svc.GetBuildbotBuildJSON(c, &milo.BuildbotBuildRequest{
					Master: "foo",
				})
				So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "No builder specified"))
			})
			Convey(`Basic`, func() {
				_, err := svc.GetBuildbotBuildJSON(c, &milo.BuildbotBuildRequest{
					Master:   name,
					Builder:  "fake",
					BuildNum: 1,
				})
				So(err, ShouldBeNil)
			})
			Convey(`Basic Not found`, func() {
				_, err := svc.GetBuildbotBuildJSON(c, &milo.BuildbotBuildRequest{
					Master:   name,
					Builder:  "fake",
					BuildNum: 2,
				})
				So(err, ShouldResemble, grpc.Errorf(codes.NotFound, "Build not found"))
			})
		})
	})
}
