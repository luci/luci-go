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

package bbfake

import (
	"context"

	"google.golang.org/grpc"

	bbpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/cv/internal/buildbucket"
)

type clientFactory struct {
	fake *Fake
}

// MakeClient implements buildbucket.ClientFactory.
func (factory clientFactory) MakeClient(ctx context.Context, host, luciProject string) (buildbucket.Client, error) {
	return &Client{
		fake:        factory.fake,
		luciProject: luciProject,
		bbHost:      host,
	}, nil
}

// Client connects a Buildbucket Fake and scope to a certain LUCI Project +
// Buildbucket host.
type Client struct {
	fake        *Fake
	luciProject string
	bbHost      string
}

// GetBuild implements buildbucket.Client.
func (c *Client) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	panic("not implemented")
}

// SearchBuilds implements buildbucket.Client.
func (c *Client) SearchBuilds(ctx context.Context, in *bbpb.SearchBuildsRequest, opts ...grpc.CallOption) (*bbpb.SearchBuildsResponse, error) {
	panic("not implemented")
}

// ScheduleBuild implements buildbucket.Client.
func (c *Client) ScheduleBuild(ctx context.Context, in *bbpb.ScheduleBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	panic("not implemented")
}

// CancelBuild implements buildbucket.Client.
func (c *Client) CancelBuild(ctx context.Context, in *bbpb.CancelBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	panic("not implemented")
}
