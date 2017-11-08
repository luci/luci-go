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
	"fmt"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// gitilesHistory is an implementation of a testutil.HistoryFunc intended to be used
// in production (not for testing).
func gitilesHistory(c context.Context, repoURL, oldRevision, newRevision string) ([]gitiles.Commit, error) {
	c, _ = context.WithTimeout(c, 30*time.Second)
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	client := gitiles.Client{Client: &http.Client{Transport: transport}, Auth: true}
	// With this range, if newCommit.Revision == oldRevision we'll get one commit.
	treeish := fmt.Sprintf("%s~1..%s", oldRevision, newRevision)
	commits, err := client.Log(c, repoURL, treeish)
	if err != nil {
		return nil, errors.Annotate(err, "fetching commit from Gitiles").Err()
	}
	// Sanity check.
	if len(commits) > 0 && commits[0].Commit != newRevision {
		return nil, errors.Reason("gitiles returned inconsistent results").Err()
	}
	return commits, nil
}
