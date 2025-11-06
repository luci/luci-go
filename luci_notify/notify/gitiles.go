// Copyright 2017 The LUCI Authors.
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

package notify

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
)

// HistoryFunc is a function that gets a list of commits from Gitiles for a
// specific repository, between oldRevision and newRevision, inclusive.
//
// If oldRevision is not reachable from newRevision, returns an empty slice
// and nil error.
type HistoryFunc func(ctx context.Context, luciProject, gerritHost, gerritProject, oldRevision, newRevision string) ([]*gitpb.Commit, error)

// gitilesHistory is an implementation of a HistoryFunc intended to be used
// in production (not for testing).
func gitilesHistory(ctx context.Context, luciProject, gerritHost, gerritProject, oldRevision, newRevision string) ([]*gitpb.Commit, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	opts := []auth.RPCOption{
		auth.WithProject(luciProject),
		auth.WithScopes(scopes.GerritScopeSet()...),
	}
	transport, err := auth.GetRPCTransport(ctx, auth.AsProject, opts...)
	if err != nil {
		return nil, errors.Fmt("getting RPC Transport: %w", err)
	}
	client, err := gitiles.NewRESTClient(&http.Client{Transport: transport}, gerritHost, true)
	if err != nil {
		return nil, errors.Fmt("creating Gitiles client: %w", err)
	}

	req := &gitilespb.LogRequest{
		Project: gerritProject,
		// With ~1, if newCommit.Revision == oldRevision,
		// we'll get one commit,
		// otherwise we will get 0 commits which is indistinguishable from
		// oldRevision not being reachable from newRevision.
		ExcludeAncestorsOf: oldRevision + "~1",
		Committish:         newRevision,
	}
	logging.Infof(ctx, "Gitiles request to host %q: %q", gerritHost, req)
	res, err := gitiles.PagingLog(ctx, client, req, 0)
	switch {
	case err != nil:
		return nil, grpcutil.WrapIfTransient(errors.Fmt("fetching commit from Gitiles: %w", err))
	case len(res) > 0 && res[0].Id != newRevision: // Sanity check.
		return nil, errors.New("gitiles returned inconsistent results")
	default:
		return res, nil
	}
}
