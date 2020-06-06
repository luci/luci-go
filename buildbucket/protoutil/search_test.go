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
	"sync"
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type searchStub struct {
	pb.BuildsClient
	reqs         []*pb.SearchBuildsRequest
	mu           sync.Mutex
	searchBuilds func(context.Context, *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error)
}

func (c *searchStub) SearchBuilds(ctx context.Context, in *pb.SearchBuildsRequest, opts ...grpc.CallOption) (*pb.SearchBuildsResponse, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, in)
	c.mu.Unlock()
	return c.searchBuilds(ctx, in)
}

func (c *searchStub) simpleMock(res *pb.SearchBuildsResponse, err error) {
	c.searchBuilds = func(context.Context, *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
		return res, err
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	Convey("Search", t, func(c C) {
		ctx := context.Background()

		client := &searchStub{}

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
			expectedBuilds := []*pb.Build{{Id: 1}, {Id: 2}}
			client.simpleMock(&pb.SearchBuildsResponse{Builds: expectedBuilds}, nil)
			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldResembleProto, expectedBuilds)
		})

		Convey("Two pages", func() {
			client.searchBuilds = func(ctx context.Context, in *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
				switch in.PageToken {
				case "":
					return &pb.SearchBuildsResponse{
						Builds:        []*pb.Build{{Id: 1}, {Id: 2}},
						NextPageToken: "token",
					}, nil
				case "token":
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{{Id: 3}},
					}, nil
				default:
					return nil, errors.Reason("unexpected request").Err()
				}
			}

			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldResembleProto, []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}})
		})

		Convey("Response error", func() {
			client.simpleMock(nil, fmt.Errorf("request failed"))
			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			So(err, ShouldErrLike, "request failed")
			So(actualBuilds, ShouldBeEmpty)
		})

		Convey("Ensure Required fields", func() {
			client.simpleMock(&pb.SearchBuildsResponse{}, nil)
			actualBuilds, err := search(&pb.SearchBuildsRequest{
				Fields: &field_mask.FieldMask{Paths: []string{"builds.*.created_by"}},
			})
			So(err, ShouldBeNil)
			So(actualBuilds, ShouldBeEmpty)
			So(client.reqs, ShouldHaveLength, 1)
			So(client.reqs[0].Fields, ShouldResembleProto, &field_mask.FieldMask{Paths: []string{"builds.*.created_by", "builds.*.id", "next_page_token"}})
		})

		Convey("Interrupt", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			client.simpleMock(&pb.SearchBuildsResponse{
				Builds: []*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}},
			}, nil)

			builds := make(chan *pb.Build)
			errC := make(chan error)
			go func() {
				errC <- Search(ctx, builds, client, &pb.SearchBuildsRequest{})
			}()

			So(<-builds, ShouldResembleProto, &pb.Build{Id: 1})
			cancel()
			err := <-errC
			So(err, ShouldNotBeNil)
			So(err == context.Canceled, ShouldBeTrue)
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
			client.searchBuilds = func(ctx context.Context, in *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
				for _, r := range requests {
					if in.PageToken != r.pageToken || in.Predicate.GetStatus() != r.status {
						continue
					}
					builds := make([]*pb.Build, len(r.buildIDs))
					for i, id := range r.buildIDs {
						builds[i] = &pb.Build{Id: id}
					}
					return &pb.SearchBuildsResponse{
						Builds:        builds,
						NextPageToken: r.nextPageToken,
					}, nil
				}

				return nil, errors.Reason("unexpected request").Err()
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
			client.searchBuilds = func(ctx context.Context, in *pb.SearchBuildsRequest) (*pb.SearchBuildsResponse, error) {
				switch {
				case in.Predicate.Status == pb.Status_SUCCESS:
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{{Id: 1}, {Id: 11}, {Id: 21}},
					}, nil

				case in.Predicate.Status == pb.Status_FAILURE:
					return &pb.SearchBuildsResponse{
						Builds: []*pb.Build{{Id: 2}, {Id: 11}, {Id: 22}},
					}, nil

				default:
					return nil, errors.Reason("unexpected request").Err()
				}
			}

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
