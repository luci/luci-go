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

package git

import (
	"regexp"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
)

// Log implements Client interface.
func (p *implementation) FetchRefs(c context.Context, host, project, startRef, refNamespace string, refSuffix *regexp.Regexp, limit int) (commits []*gitpb.Commit, err error) {
	defer func() { err = errors.Annotate(tagError(c, err), "gitiles.FetchRefs").Err() }()

	if refSuffix == nil {
		err = errors.Reason("refSuffix is required").Err()
		return
	}

	allowed, err := p.acls.IsAllowed(c, host, project)
	switch {
	case err != nil:
		return
	case !allowed:
		err = status.Errorf(codes.NotFound, "not found")
		return
	}

	// Step 1. Request all refs in refNamespace.
	client, err := p.gitilesClient(c, host)
	if err != nil {
		return
	}

	req := gitilespb.RefsRequest{Project: project, RefsPath: refNamespace}
	res, err := client.Refs(c, &req)
	if err != nil {
		return
	}

	// Step 2. Filter out those that match refSuffix.
	matchingRefs := map[string]string{}
	for ref, commit := range res.Revisions {
		suffix := strings.TrimPrefix(ref, refNamespace)
		if refSuffix.MatchString(suffix) {
			matchingRefs[ref] = commit
		}
	}

	// Step 3. Fetch commits from all matching refs using p.log(c, host, project,
	//         ref, startRef, &LogOptins{Limit: limit})
	// TODO(sergiyb): Implement caching in memcache, i.e. store (ref+commit) ->
	// (list of commits) mapping and call Gitiles only if ref moves.
	logs := map[string][]*gitpb.Commit{}
	if err = parallel.FanOutIn(func(ch chan<- func() error) {
		for ref, commit := range matchingRefs {
			ch <- func() (err error) {
				logs[ref], err = p.log(
					c, host, project, commit, startRef, &LogOptions{Limit: limit})
				return
			}
		}
	}); err != nil {
		return
	}

	// Step 4. Merge commits from all refs sorted by time into a single
	//         mergedCommits list up to a limit.
	commits = make([]*gitpb.Commit, limit)
	for {
		if len(commits) == limit {
			return
		}

		commitTime := func(commit *gitpb.Commit) uint64 {
			if commit == nil {
				return 0
			}

			// This should not overflow for the next 500 years or so.
			return uint64(commit.Committer.Time.Seconds)*1e9 + uint64(commit.Committer.Time.Nanos)
		}

		var c *gitpb.Commit = nil
		var refName string
		for ref, commits := range logs {
			if len(commits) > 0 && commitTime(commits[0]) > commitTime(c) {
				c = commits[0]
				refName = ref
			}
		}

		// We ran out of commits on all refs.
		if c == nil {
			return
		}

		logs[refName] = logs[refName][1:]
		commits = append(commits, c)
	}
}
