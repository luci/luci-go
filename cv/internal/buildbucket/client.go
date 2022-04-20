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

package buildbucket

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
)

// Client defines a subset of Buildbucket API used by CV.
//
// For full definition, see:
// https://pkg.go.dev/go.chromium.org/luci/buildbucket/proto#BuildsClient
type Client interface {
	// GetBuild gets a build.
	GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
	// SearchBuilds searches for builds.
	SearchBuilds(ctx context.Context, in *bbpb.SearchBuildsRequest, opts ...grpc.CallOption) (*bbpb.SearchBuildsResponse, error)
	// ScheduleBuild schedules a new build.
	ScheduleBuild(ctx context.Context, in *bbpb.ScheduleBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
	// CancelBuild cancels a build.
	CancelBuild(ctx context.Context, in *bbpb.CancelBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error)
}

// ClientFactory creates Client tied to Buildbucket host and LUCI project.
type ClientFactory interface {
	MakeClient(ctx context.Context, host, luciProject string) (Client, error)
}

// NewClientFactory returns Factory for use in production.
func NewClientFactory() ClientFactory {
	return prpcClientFactory{}
}

type prpcClientFactory struct{}

func (prpcClientFactory) MakeClient(ctx context.Context, host, luciProject string) (Client, error) {
	rt, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject))
	if err != nil {
		return nil, err
	}
	return bbpb.NewBuildsPRPCClient(&prpc.Client{
		C:    &http.Client{Transport: rt},
		Host: host,
	}), nil
}
