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
	"net/http"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
)

// HistoryFunc is a function that gets a list of commits from Gitiles for a
// specific repository, between oldRevision and newRevision, inclusive.
//
// If oldRevision is not reachable from newRevision, returns an empty slice
// and nil error.
type HistoryFunc func(c context.Context, host, project, oldRevision, newRevision string) ([]*gitpb.Commit, error)

// gitilesHistory is an implementation of a HistoryFunc intended to be used
// in production (not for testing).
func gitilesHistory(c context.Context, host, project, oldRevision, newRevision string) ([]*gitpb.Commit, error) {
	c, _ = context.WithTimeout(c, 30*time.Second)

	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	client, err := gitiles.NewRESTClient(&http.Client{Transport: transport}, host, true)
	if err != nil {
		return nil, errors.Annotate(err, "creating Gitiles client").Err()
	}

	req := &gitilespb.LogRequest{
		Project: project,
		// With ~1, if newCommit.Revision == oldRevision,
		// we'll get one commit,
		// otherwise we will get 0 commits which is indistinguishable from
		// oldRevision not being reachable from newRevision.
		Ancestor: oldRevision + "~1",
		Treeish:  newRevision,
	}
	logging.Infof(c, "Gitiles request to host %q: %q", host, req)
	res, err := client.Log(c, req)
	switch {
	case err != nil:
		return nil, errors.Annotate(err, "fetching commit from Gitiles").Err()
	case len(res.Log) > 0 && res.Log[0].Id != newRevision: // Sanity check.
		return nil, errors.Reason("gitiles returned inconsistent results").Err()
	default:
		return res.Log, nil
	}
}
