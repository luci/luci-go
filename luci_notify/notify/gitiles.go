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

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
)

// GitilesClientFactoryKey is the key by which a GitilesClientFactory is stored in a context.Context.
var GitilesClientFactoryKey = "store a gitiles client factory"

// GitilesClient is an interface intended to abstract away the ability to
// query Gitiles for commits within a specific range of revisions. The
// interface exists so that it can be mocked for testing.
type GitilesClient interface {
	// History queries Gitiles and returns a list of commits.
	//
	// The call is equivalent to running `git log oldRevision..newRevision`.
	History(c context.Context, newRevision, oldRevision string) ([]gitiles.Commit, error)
}

// GitilesClientFactory is any function which creates a GitilesClient.
type GitilesClientFactory func(c context.Context, repoURL string) (GitilesClient, error)

// WithGitilesClientFactory returns a context.Context embedded with a GitilesClientFactory.
func WithGitilesClientFactory(c context.Context, fac GitilesClientFactory) context.Context {
        return context.WithValue(c, &GitilesClientFactoryKey, fac)
} 

// NewGitilesClient generates a new GitilesClient from a factory in the context.
func NewGitilesClient(c context.Context, repoURL string) (GitilesClient, error) {                                          
	if fac, ok := c.Value(&GitilesClientFactoryKey).(GitilesClientFactory); !ok {
		panic("no gitiles client factory installed")
	} else {
		return fac(c, repoURL)
	}
}

// GitilesClientImpl wraps a gitiles.Client and implements the GitilesClient interface
// and is intended to be used in production (outside of testing).
type GitilesClientImpl struct {
	gitiles.Client

	// RepoURL is the URL of the repository in Gitiles for which this client will
	// query information.
	RepoURL string
}
 
// History implements the GitilesClient interface for GitilesClientImpl.
func (g *GitilesClientImpl) History(c context.Context, newRevision, oldRevision string) ([]gitiles.Commit, error) {
	// With this range, if newCommit.Revision == oldRevision we'll get nothing, but
	// this was checked earlier already.
	treeish := fmt.Sprintf("%s..%s", oldRevision, newRevision)
	commits, err := g.Client.Log(c, g.RepoURL, treeish)
	if err != nil {
		return nil, errors.Annotate(err, "fetching commit from Gitiles").Err()
	}
	// Sanity check.
	if len(commits) > 0 && commits[0].Commit != newRevision {
		return nil, errors.Reason("gitiles returned inconsistent results").Err()
	}
	return commits, nil
}

// GitilesClientFactoryImpl is a GitilesClientFactory intended to be used in production
// (i.e. outside of testing).
func GitilesClientFactoryImpl(c context.Context, repoURL string) (GitilesClient, error) {
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(
		"https://www.googleapis.com/auth/gerritcodereview",
	))
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	return GitilesClient(&GitilesClientImpl{
		Client: gitiles.Client{Client: &http.Client{Transport: transport}},
		RepoURL: repoURL,
	}), nil
}
