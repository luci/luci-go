// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"context"
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"

	"github.com/golang/mock/gomock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSearch(t *testing.T) {
	t.Parallel()

	Convey("Search", t, func(c C) {
		ctx := context.Background()

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		client := pb.NewMockBuildsClient(ctl)

		params := func(limit int) SearchParams {
			return SearchParams{
				Client:  client,
				BaseReq: &pb.SearchBuildsRequest{},
				Limit:   limit,
			}
		}

		Convey("One page", func() {
			expectedBuilds := []*pb.Build{{Id: 1}, {Id: 2}}
			client.EXPECT().
				SearchBuilds(ctx, &pb.SearchBuildsRequest{PageSize: defaultPageSize}).
				Return(&pb.SearchBuildsResponse{Builds: expectedBuilds}, nil)

			actualBuilds, nextPageToken, err := Search(ctx, params(0))
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldEqual, "")
			So(actualBuilds, ShouldResembleProto, expectedBuilds)
		})

		Convey("One page with limit", func() {
			client.EXPECT().
				SearchBuilds(ctx, &pb.SearchBuildsRequest{
					PageSize: 10,
				}).
				Return(&pb.SearchBuildsResponse{
					NextPageToken: "cursor",
				}, nil)

			_, nextPageToken, err := Search(ctx, params(10))
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldEqual, "cursor")
		})

		Convey("Two pages and limit", func() {
			firstPage := client.EXPECT().
				SearchBuilds(ctx, &pb.SearchBuildsRequest{
					PageSize: 10,
				}).
				Return(&pb.SearchBuildsResponse{
					Builds:        []*pb.Build{{Id: 1}, {Id: 2}},
					NextPageToken: "cursor",
				}, nil)

			client.EXPECT().
				SearchBuilds(ctx, &pb.SearchBuildsRequest{
					PageSize:  8,
					PageToken: "cursor",
				}).
				After(firstPage).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 3}},
				}, nil)

			actualBuilds, nextPageToken, err := Search(ctx, params(10))
			So(err, ShouldBeNil)
			So(nextPageToken, ShouldEqual, "")
			So(actualBuilds, ShouldResembleProto, []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}})
		})

		Convey("Interrupt", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			builds := make(chan *pb.Build)
			client.EXPECT().
				SearchBuilds(ctx, &pb.SearchBuildsRequest{PageSize: defaultPageSize}).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}},
				}, nil)

			var err error
			var nextPageToken string
			done := make(chan struct{})
			go func() {
				nextPageToken, err = SearchCont(ctx, builds, params(0))
				close(done)
			}()

			So(<-builds, ShouldResembleProto, &pb.Build{Id: 1})
			So(<-builds, ShouldResembleProto, &pb.Build{Id: 2})
			cancel()
			<-done
			So(err == context.Canceled, ShouldBeTrue)
			So(nextPageToken, ShouldEqual, "")
		})
	})
}
