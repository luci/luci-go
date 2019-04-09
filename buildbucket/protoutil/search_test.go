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
	"fmt"
	"testing"

	pb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/genproto/protobuf/field_mask"

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

		search := func(requests ...*pb.SearchBuildsRequest) ([]*pb.Build, error) {
			buildC := make(chan *pb.Build)
			errC := make(chan error)
			var builds []*pb.Build
			go func() {
				err := Search(ctx, buildC, client, requests...)
				close(buildC)
				errC <- err
			}()
			for b := range buildC {
				builds = append(builds, b)
			}
			return builds, <-errC
		}

		Convey("One page", func() {
			req := &pb.SearchBuildsRequest{}
			expectedBuilds := []*pb.Build{{Id: 1}, {Id: 2}}
			client.EXPECT().
				SearchBuilds(gomock.Any(), req).
				Return(&pb.SearchBuildsResponse{Builds: expectedBuilds}, nil)

			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldResembleProto, expectedBuilds)
		})

		Convey("Two pages", func() {
			firstPage := client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{}).
				Return(&pb.SearchBuildsResponse{
					Builds:        []*pb.Build{{Id: 1}, {Id: 2}},
					NextPageToken: "cursor",
				}, nil)

			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{
					PageToken: "cursor",
				}).
				After(firstPage).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 3}},
				}, nil)

			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldResembleProto, []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}})
		})

		Convey("Response error", func() {
			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{}).
				Return(nil, fmt.Errorf("request failed"))

			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldErrLike, "request failed")
			So(actualBuilds, ShouldBeEmpty)
		})

		Convey("Ensure NextPageToken", func() {
			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{
					Fields: &field_mask.FieldMask{Paths: []string{"builds.*.status", "next_page_token", "builds.*.id"}},
				}).
				Return(&pb.SearchBuildsResponse{}, nil)

			actualBuilds, err := search(&pb.SearchBuildsRequest{
				Fields: &field_mask.FieldMask{Paths: []string{"builds.*.status"}},
			})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldBeEmpty)
		})

		Convey("Interrupt", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{}).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}},
				}, nil)

			builds := make(chan *pb.Build)
			errC := make(chan error)
			go func() {
				errC <- Search(ctx, builds, client, &pb.SearchBuildsRequest{})
			}()

			So(<-builds, ShouldResembleProto, &pb.Build{Id: 1})
			cancel()
			So(<-errC == context.Canceled, ShouldBeTrue)
		})

		Convey("Multiple requests", func() {
			// First stream, with status filter SUCCESS and two pages.
			requests := []struct {
				status    pb.Status
				pageToken string

				buildIDs      []int64
				nextPageToken string
			}{
				// First stream.
				{
					status:        pb.Status_SUCCESS,
					buildIDs:      []int64{1, 11, 21},
					nextPageToken: "1",
				},
				{
					status:    pb.Status_SUCCESS,
					buildIDs:  []int64{31},
					pageToken: "1",
				},
				// Second stream.
				{
					status:        pb.Status_FAILURE,
					buildIDs:      []int64{2, 12, 22},
					nextPageToken: "2",
				},
				{
					status:    pb.Status_FAILURE,
					buildIDs:  []int64{32, 42},
					pageToken: "2",
				},
				// Third stream.
				{
					status:   pb.Status_INFRA_FAILURE,
					buildIDs: []int64{3},
				},
			}
			for _, r := range requests {
				builds := make([]*pb.Build, len(r.buildIDs))
				for i, id := range r.buildIDs {
					builds[i] = &pb.Build{Id: id}
				}
				client.EXPECT().
					SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{
						Predicate: &pb.BuildPredicate{Status: r.status},
						PageToken: r.pageToken,
					}).
					Return(&pb.SearchBuildsResponse{
						Builds:        builds,
						NextPageToken: r.nextPageToken,
					}, nil)
			}

			builds, err := search([]*pb.SearchBuildsRequest{
				{Predicate: &pb.BuildPredicate{Status: pb.Status_SUCCESS}},
				{Predicate: &pb.BuildPredicate{Status: pb.Status_FAILURE}},
				{Predicate: &pb.BuildPredicate{Status: pb.Status_INFRA_FAILURE}},
			}...)
			So(err, ShouldBeNil)
			So(builds, ShouldResembleProto, []*pb.Build{
				{Id: 1},
				{Id: 2},
				{Id: 3},
				{Id: 11},
				{Id: 12},
				{Id: 21},
				{Id: 22},
				{Id: 31},
				{Id: 32},
				{Id: 42},
			})
		})

		Convey("Duplicate build", func() {
			// First stream, with status filter SUCCESS and two pages.
			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{Status: pb.Status_SUCCESS},
				}).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 1}, {Id: 11}, {Id: 21}},
				}, nil)
			// Second stream, with status filter FAILURE and two pages.
			client.EXPECT().
				SearchBuilds(gomock.Any(), &pb.SearchBuildsRequest{
					Predicate: &pb.BuildPredicate{Status: pb.Status_FAILURE},
				}).
				Return(&pb.SearchBuildsResponse{
					Builds: []*pb.Build{{Id: 2}, {Id: 11}, {Id: 22}},
				}, nil)

			builds, err := search([]*pb.SearchBuildsRequest{
				{Predicate: &pb.BuildPredicate{Status: pb.Status_SUCCESS}},
				{Predicate: &pb.BuildPredicate{Status: pb.Status_FAILURE}},
			}...)

			So(err, ShouldBeNil)
			So(builds, ShouldResembleProto, []*pb.Build{
				{Id: 1},
				{Id: 2},
				{Id: 11},
				{Id: 21},
				{Id: 22},
			})
		})

		Convey("Empty request slice", func() {
			actualBuilds, err := search()
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldBeEmpty)
		})
	})
}
