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

// Package git provides high level API for Git/Gerrit data.
//
// Features:
//
//	It enforces read ACLs on Git/Gerrit hosts/projects configured in
//	  settings.cfg in source_acls blocks.
//	Transparently caches results respecting ACLs above.
//	  That's why no caching of returned data should be done by callers.
//
// Limitations:
//
//	currently, only works with *.googlesource.com hosted Git/Gerrit
//	repositories, but could be extended to work with other providers.
package git

import (
	"context"
	"net/http"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/milo/internal/git/gitacls"
)

// Client provides high level API for Git/Gerrit data.
//
// Methods may return grpc errors returned by the underlying Gitiles service.
// These errors will be annotated with Milo's error tags whenever reasonable.
type Client interface {

	// Log returns ancestors commits of the given repository host
	// (e.g. "chromium.googlesource.com"), project (e.g. "chromium/src")
	// and descendant committish (e.g. "refs/heads/main" or commit hash).
	//
	// Limit specifies the maximum number of commits to return.
	// If limit<=0, 50 is used.
	// Setting a lower value increases cache hit probability.
	//
	// May return gRPC errors returned by the underlying Gitiles service.
	Log(c context.Context, host, project, commitish string, inputOptions *LogOptions) ([]*gitpb.Commit, error)

	// CombinedLogs returns latest commits reachable from several refs.
	//
	// Returns a slice of up to a limit (defaults to 50 when <= 0) commits that:
	//  * for each source ref, subsequence of returned slice formed by
	//    its corresponding commits will be in the same order,
	//  * across refs, commits will be ordered by timestamp, and
	//  * identical commits from multiple commits will be deduped.
	//
	// For example, for two refs with commits (C_5 means C was committed at time
	// 5) and limit 5, the function will first resolve each ref to sequence of
	// commits:
	//
	//    ref1: A_1 -> B_5 -> C_9
	//    ref2: X_2 -> Y_7 -> Z_4    (note timestamp inversion)
	//
	// and then return combined list of [C_9, B_5, Z_4, Y_7, X_2].
	//
	// refs must be a list of ref specs as described in the proto config (see
	// doc for refs field in the Console message in api/config/project.proto)
	//
	// excludeRef can be set to non-empty value to exclude commits from a specific
	// ref, e.g. this is useful when requesting commits from branches that branch
	// off a single main branch, commits from which should not be returned even
	// though they are present in the history of each requested branch
	CombinedLogs(c context.Context, host, project, excludeRef string,
		refs []string, limit int) (commits []*gitpb.Commit, err error)

	// CLEmail fetches the CL owner email.
	//
	// Returns empty string if either:
	//   CL doesn't exist, or
	//   current user has no access to CL's project.
	//
	// May return gRPC errors returned by the underlying Gerrit service.
	CLEmail(c context.Context, host string, changeNumber int64) (string, error)
}

// UseACLs returns context with production implementation installed.
func UseACLs(c context.Context, acls *gitacls.ACLs) context.Context {
	return Use(c, NewClient(acls))
}

// NewClient returns a new production Client.
func NewClient(acls *gitacls.ACLs) Client {
	return &implementation{acls: acls}
}

// Use returns context with provided Client implementation.
func Use(c context.Context, s Client) context.Context {
	return context.WithValue(c, &contextKey, s)
}

// Get returns Client set in supplied context.
//
// panics if not set.
func Get(c context.Context) Client {
	s, ok := c.Value(&contextKey).(Client)
	if !ok {
		panic(errors.New("git.Client not installed in context"))
	}
	return s
}

// private implementation

var contextKey = "client factory key"

// implementation implements Client.
type implementation struct {
	acls *gitacls.ACLs
	// If a mockGerrit or mockGitiles is provided, then the mocks will be used.
	// Otherwise, the production client will be used.
	mockGitiles gitilespb.GitilesClient
	mockGerrit  gerritpb.GerritClient
}

var _ Client = (*implementation)(nil)

// transport returns an authenticated RoundTripper for Gerrit or Gitiles RPCs.
func (p *implementation) transport(c context.Context) (transport http.RoundTripper, err error) {
	luciProject, ok := ProjectFromContext(c)
	if ok {
		return auth.GetRPCTransport(c, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(scopes.GerritScopeSet()...))
	}
	return auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(scopes.GerritScopeSet()...))
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
