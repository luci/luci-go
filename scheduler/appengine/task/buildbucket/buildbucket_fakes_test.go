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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

type BuildbucketFake struct {
	ScheduleBuild func(*bbpb.ScheduleBuildRequest) (*bbpb.Build, error)
	GetBuildV1    func(bid int64) (*bbv1.LegacyApiBuildResponseMessage, error)
	CancelBuildV1 func(bid int64) (*bbv1.LegacyApiBuildResponseMessage, error)

	srv *httptest.Server
}

type fakeBuildsServer struct {
	bbpb.UnimplementedBuildsServer

	fake *BuildbucketFake
}

func (f *fakeBuildsServer) ScheduleBuild(ctx context.Context, req *bbpb.ScheduleBuildRequest) (*bbpb.Build, error) {
	return f.fake.ScheduleBuild(req)
}

func (bb *BuildbucketFake) Start() {
	r := router.New()

	// Buildbucket v2 API fake.
	rpcs := &prpc.Server{Authenticator: prpc.NoAuthentication}
	rpcs.InstallHandlers(r, router.MiddlewareChain{})
	bbpb.RegisterBuildsServer(rpcs, &fakeBuildsServer{fake: bb})

	// Buildbucket v1 API fake.
	r.GET("/_ah/api/buildbucket/v1/builds/:BuildID", router.MiddlewareChain{}, func(ctx *router.Context) {
		doV1(ctx, bb.GetBuildV1)
	})
	r.POST("/_ah/api/buildbucket/v1/builds/:BuildID/cancel", router.MiddlewareChain{}, func(ctx *router.Context) {
		doV1(ctx, bb.CancelBuildV1)
	})

	bb.srv = httptest.NewServer(r)
}

func (bb *BuildbucketFake) Stop() {
	bb.srv.Close()
}

func (bb *BuildbucketFake) URL() string {
	return bb.srv.URL
}

func doV1(ctx *router.Context, cb func(bid int64) (*bbv1.LegacyApiBuildResponseMessage, error)) {
	bid, err := strconv.ParseInt(ctx.Params.ByName("BuildID"), 10, 64)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), 400)
		return
	}
	if cb == nil {
		http.Error(ctx.Writer, "not implemented", 404)
		return
	}
	resp, err := cb(bid)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), 500)
		return
	}
	blob, _ := json.Marshal(resp)
	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.Write(blob)
}
