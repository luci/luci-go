// Copyright 2021 The LUCI Authors.
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
	"net/http/httptest"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

type BuildbucketFake struct {
	ScheduleBuild func(*bbpb.ScheduleBuildRequest) (*bbpb.Build, error)
	GetBuild      func(*bbpb.GetBuildRequest) (*bbpb.Build, error)
	CancelBuild   func(*bbpb.CancelBuildRequest) (*bbpb.Build, error)

	srv *httptest.Server
}

type fakeBuildsServer struct {
	bbgrpcpb.UnimplementedBuildsServer

	fake *BuildbucketFake
}

func (f *fakeBuildsServer) ScheduleBuild(ctx context.Context, req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
	return f.fake.ScheduleBuild(req)
}

func (f *fakeBuildsServer) GetBuild(ctx context.Context, req *bbpb.GetBuildRequest) (*bbpb.Build, error) {
	return f.fake.GetBuild(req)
}

func (f *fakeBuildsServer) CancelBuild(ctx context.Context, req *bbpb.CancelBuildRequest) (*bbpb.Build, error) {
	return f.fake.CancelBuild(req)
}

func (bb *BuildbucketFake) Start() {
	r := router.New()

	rpcs := &prpc.Server{}
	rpcs.InstallHandlers(r, nil)
	bbgrpcpb.RegisterBuildsServer(rpcs, &fakeBuildsServer{fake: bb})

	bb.srv = httptest.NewServer(r)
}

func (bb *BuildbucketFake) Stop() {
	bb.srv.Close()
}

func (bb *BuildbucketFake) URL() string {
	return bb.srv.URL
}
