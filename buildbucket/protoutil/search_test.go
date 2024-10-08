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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
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

	ftt.Run("Search", t, func(c *ftt.Test) {
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

		c.Run("One page", func(c *ftt.Test) {
			expectedBuilds := []*pb.Build{{Id: 1}, {Id: 2}}
			client.simpleMock(&pb.SearchBuildsResponse{Builds: expectedBuilds}, nil)
			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actualBuilds, should.Resemble(expectedBuilds))
		})

		c.Run("Two pages", func(c *ftt.Test) {
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
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actualBuilds, should.Resemble([]*pb.Build{{Id: 1}, {Id: 2}, {Id: 3}}))
		})

		c.Run("Response error", func(c *ftt.Test) {
			client.simpleMock(nil, fmt.Errorf("request failed"))
			actualBuilds, err := search(&pb.SearchBuildsRequest{})
			assert.Loosely(c, err, should.ErrLike("request failed"))
			assert.Loosely(c, actualBuilds, should.BeEmpty)
		})

		c.Run("Ensure Required fields", func(c *ftt.Test) {
			client.simpleMock(&pb.SearchBuildsResponse{}, nil)
			actualBuilds, err := search(&pb.SearchBuildsRequest{
				Fields: &field_mask.FieldMask{Paths: []string{"builds.*.created_by"}},
			})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actualBuilds, should.BeEmpty)
			assert.Loosely(c, client.reqs, should.HaveLength(1))
			assert.Loosely(c, client.reqs[0].Fields, should.Resemble(&field_mask.FieldMask{Paths: []string{"builds.*.created_by", "builds.*.id", "next_page_token"}}))
		})

		c.Run("Interrupt", func(c *ftt.Test) {
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

			assert.Loosely(c, <-builds, should.Resemble(&pb.Build{Id: 1}))
			cancel()
			err := <-errC
			assert.Loosely(c, err, should.NotBeNil)
			assert.Loosely(c, err == context.Canceled, should.BeTrue)
		})

		c.Run("Multiple requests", func(c *ftt.Test) {
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
			assert.Loosely(c, err, should.BeNil)

			assert.Loosely(c, builds, should.Resemble([]*pb.Build{
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
			}))
		})

		c.Run("Duplicate build", func(c *ftt.Test) {
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

			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, builds, should.Resemble([]*pb.Build{
				{Id: 1},
				{Id: 2},
				{Id: 11},
				{Id: 21},
				{Id: 22},
			}))
		})

		c.Run("Empty request slice", func(c *ftt.Test) {
			actualBuilds, err := search()
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, actualBuilds, should.BeEmpty)
		})
	})
}
