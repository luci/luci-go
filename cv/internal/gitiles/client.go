// Copyright 2026 The LUCI Authors.
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

package gitiles

import (
	"context"
	"net/http"

	"google.golang.org/grpc"

	"go.chromium.org/luci/auth/scopes"
	gitilesapi "go.chromium.org/luci/common/api/gitiles"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
)

// Client defines a subset of the Gitiles API used by CV.
type Client interface {
	// Log retrieves commit log.
	Log(ctx context.Context, in *gitilespb.LogRequest, opts ...grpc.CallOption) (*gitilespb.LogResponse, error)
}

// Factory creates Client tied to Gitiles host and LUCI project.
type Factory interface {
	MakeClient(ctx context.Context, host, luciProject string) (Client, error)
}

// NewFactory returns a Factory for use in production.
func NewFactory() Factory {
	return prodFactory{}
}

type prodFactory struct{}

// MakeClient implements Factory.
func (prodFactory) MakeClient(ctx context.Context, host, luciProject string) (Client, error) {
	// Use AsProject to act on behalf of the LUCI project.
	rt, err := auth.GetRPCTransport(ctx, auth.AsProject,
		auth.WithProject(luciProject),
		auth.WithScopes(scopes.GerritScopeSet()...),
	)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: rt}
	// We pass true for auth because we want to make authenticated calls.
	return gitilesapi.NewRESTClient(httpClient, host, true)
}
