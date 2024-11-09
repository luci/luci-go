// Copyright 2022 The LUCI Authors.
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

// Package buildbucket contains logic of interacting with Buildbucket.
package buildbucket

import (
	"context"
	"net/http"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
)

const (
	bbHost = "cr-buildbucket.appspot.com"
)

// mockedBBClientKey is the context key indicates using mocked buildbucket client in tests.
var mockedBBClientKey = "used in tests only for setting the mock buildbucket client"

func newBuildsClient(ctx context.Context, host string) (bbpb.BuildsClient, error) {
	if mockClient, ok := ctx.Value(&mockedBBClientKey).(*bbpb.MockBuildsClient); ok {
		return mockClient, nil
	}

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return bbpb.NewBuildsPRPCClient(
		&prpc.Client{
			C:       &http.Client{Transport: t},
			Host:    host,
			Options: prpc.DefaultOptions(),
		}), nil
}

// Client is the client to communicate with Buildbucket.
// It wraps a bbpb.BuildsClient.
type Client struct {
	Client bbpb.BuildsClient
}

// NewClient creates a client to communicate with Buildbucket.
func NewClient(ctx context.Context, host string) (*Client, error) {
	client, err := newBuildsClient(ctx, host)
	if err != nil {
		return nil, err
	}

	return &Client{
		Client: client,
	}, nil
}

// GetBuild returns bbpb.Build for the requested build.
func (c *Client) GetBuild(ctx context.Context, req *bbpb.GetBuildRequest) (*bbpb.Build, error) {
	return c.Client.GetBuild(ctx, req)
}

func (c *Client) SearchBuild(ctx context.Context, req *bbpb.SearchBuildsRequest) (*bbpb.SearchBuildsResponse, error) {
	return c.Client.SearchBuilds(ctx, req)
}

func (c *Client) ScheduleBuild(ctx context.Context, req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
	return c.Client.ScheduleBuild(ctx, req)
}

func (c *Client) CancelBuild(ctx context.Context, req *bbpb.CancelBuildRequest) (*bbpb.Build, error) {
	return c.Client.CancelBuild(ctx, req)
}

func GetBuild(c context.Context, bbid int64, mask *bbpb.BuildMask) (*bbpb.Build, error) {
	q := &bbpb.GetBuildRequest{
		Id:   bbid,
		Mask: mask,
	}

	cl, err := NewClient(c, bbHost)
	if err != nil {
		logging.Errorf(c, "Cannot create Buildbucket client")
		return nil, err
	}
	return cl.GetBuild(c, q)
}

// SearchOlderBuilds searches for builds in the same builder and are older than a reference Build.
// More recent builds appear first. The token for the next page of builds is also returned.
func SearchOlderBuilds(c context.Context, refBuild *bbpb.Build, mask *bbpb.BuildMask, maxResultSize int32, pageToken string) ([]*bbpb.Build, string, error) {
	req := &bbpb.SearchBuildsRequest{
		Predicate: &bbpb.BuildPredicate{
			Builder: refBuild.Builder,
			Build: &bbpb.BuildRange{
				EndBuildId: refBuild.Id,
			},
		},
		Mask:      mask,
		PageSize:  maxResultSize,
		PageToken: pageToken,
	}

	// Create a new buildbucket client
	cl, err := NewClient(c, bbHost)
	if err != nil {
		logging.Errorf(c, "Cannot create Buildbucket client")
		return nil, "", err
	}

	// Execute query for older builds
	res, err := cl.SearchBuild(c, req)
	if err != nil {
		return nil, "", err
	}

	return res.Builds, res.NextPageToken, nil
}

func ScheduleBuild(c context.Context, req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
	// Create a new buildbucket client
	cl, err := NewClient(c, bbHost)
	if err != nil {
		logging.Errorf(c, "Cannot create Buildbucket client")
		return nil, err
	}
	return cl.ScheduleBuild(c, req)
}

func CancelBuild(c context.Context, bbid int64, reason string) (*bbpb.Build, error) {
	// Create a new buildbucket client
	cl, err := NewClient(c, bbHost)
	if err != nil {
		logging.Errorf(c, "Cannot create Buildbucket client")
		return nil, err
	}
	req := &bbpb.CancelBuildRequest{
		Id:              bbid,
		SummaryMarkdown: reason,
	}
	return cl.Client.CancelBuild(c, req)
}

func GetBuildTaskDimension(ctx context.Context, bbid int64) (*pb.Dimensions, error) {
	build, err := GetBuild(ctx, bbid, &bbpb.BuildMask{
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"infra.swarming.task_dimensions", "infra.backend.task_dimensions"},
		},
	})
	if err != nil {
		return nil, err
	}

	return util.ToDimensionsPB(util.GetTaskDimensions(build)), nil
}
