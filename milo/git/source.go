// Copyright 2018 The LUCI Authors.
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

// package git provides high level API for Git/Gerrit data.
//
// Features:
//   It enforces read ACLs on Git/Gerrit hosts/projects configured in
//     settings.cfg in source_acls blocks.
//   Transparently caches results respecting ACLs above.
//     That's why no caching of returned data should be done by callers.
//
// Limitations:
//   currently, only works with *.googlesource.com hosted Git/Gerrit
//   repositories, but could be extended to work with other providers.
package git

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"
)

// Source provides high level API for Git/Gerrit data.
type Source interface {

	// Log returns ancestors commits of the given repository host
	// (e.g. "chromium.googlesource.com"), project (e.g. "chromium/src")
	// and descendant committish (e.g. "refs/heads/master" or commit hash).
	//
	// Limit specifies the number of commits to return.
	// If limit<=0, 50 is used.
	// Setting a lower value increases cache hit probability.
	//
	// May return gRPC errors returned by the underlying Gitiles service.
	Log(c context.Context, host, project, commitish string, inputOptions *LogOptions) ([]*gitpb.Commit, error)

	// FetchGerritChangeEmail fetches the CL owner email.
	//
	// Returns empty string if either:
	//   CL doesn't exist, or
	//   current user has no access to CL's project.
	FetchGerritChangeEmail(c context.Context, host string, changeNumber int64) (string, error)
}

// UseACLs returns context with production implementation installed.
func UseACLs(c context.Context, acls *ACLs) (context.Context, error) {
	return Use(c, &implementation{acls: acls}), nil
}

// Use returns context with provided Source implementation.
//
// Useful in tests, see also gittest.MockSource.
func Use(c context.Context, s Source) context.Context {
	return context.WithValue(c, &contextKey, s)
}

// Get returns Source set in supplied context.
//
// panics if not set.
func Get(c context.Context) Source {
	s, ok := c.Value(&contextKey).(Source)
	if !ok {
		panic(errors.New("git Source not installed in context"))
	}
	return s
}

// private implementation

var contextKey = "client factory key"

// implementation implements Source.
type implementation struct {
	acls        *ACLs
	mockGitiles gitilespb.GitilesClient
	mockGerrit  gerritpb.GerritClient
}

var _ Source = (*implementation)(nil)

// transport returns authenticated RoundTripper for Gerrit or Gitiles RPCs.
func (p *implementation) transport(c context.Context) (transport http.RoundTripper, err error) {
	// TODO(tandrii): if current context has OAuth 2.0 bearer token, use it.
	// TODO(tandrii): instead of auth.Self, use service accounts configured per
	//   LUCI project ( != Git/Gerrit project ).
	return auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
}

func (p *implementation) gitilesClient(c context.Context, host string) (gitilespb.GitilesClient, error) {
	if p.mockGitiles != nil {
		return p.mockGitiles, nil
	}
	t, err := p.transport(c)
	if err != nil {
		return nil, err
	}
	return gitiles.NewRESTClient(&http.Client{Transport: t}, host, true)
}

func (p *implementation) gerritClient(c context.Context, host string) (gerritpb.GerritClient, error) {
	if p.mockGerrit != nil {
		return p.mockGerrit, nil
	}
	t, err := p.transport(c)
	if err != nil {
		return nil, err
	}
	return gerrit.NewRESTClient(&http.Client{Transport: t}, host, true)
}
