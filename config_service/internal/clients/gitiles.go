// Copyright 2023 The LUCI Authors.
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

package clients

import (
	"context"
	"net/http"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/api/gitiles"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/server/auth"
)

var MockGitilesClientKey = "mock gitiles clients key for testing only"

// NewGitilesClient returns a Gitiles client with the authority of the given
// luciProject or the current service if luciProject is empty.
func NewGitilesClient(ctx context.Context, host, luciProject string) (gitilespb.GitilesClient, error) {
	if mockClient, ok := ctx.Value(&MockGitilesClientKey).(*mock_gitiles.MockGitilesClient); ok {
		return mockClient, nil
	}

	var t http.RoundTripper
	var err error
	if luciProject != "" {
		t, err = auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(scopes.GerritScopeSet()...))
	} else {
		t, err = auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.GerritScopeSet()...))
	}
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, true)
}
