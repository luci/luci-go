// Copyright 2025 The LUCI Authors.
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

package gitsource

import (
	"context"
	"slices"
	"time"
)

func (r *repoCache) isAncestor(ctx context.Context, a, b string) (bool, error) {
	return r.gitTest(ctx, "--no-lazy-fetch", "merge-base", "--is-ancestor", a, b)
}

func (r *repoCache) PickMostRecent(ctx context.Context, ref string, commits []string) (string, error) {
	ctx = r.prepDebugContext(ctx)

	if err := r.prefetchMultiple(ctx, commits, "--filter=blob:none", "--depth=1"); err != nil {
		return "", err
	}

	parsed := make([]*commitObj, 0, len(commits))
	for _, commit := range commits {
		cmt, err := r.batchProc.catFileCommit(ctx, commit)
		if err != nil {
			return "", err
		}
		parsed = append(parsed, cmt)
	}

	slices.SortFunc(parsed, func(a, b *commitObj) int {
		return a.OldestTime().Compare(b.OldestTime())
	})

	for _, cmt := range parsed {
		// skip commits which are already ancestors of ref
		anc, err := r.isAncestor(ctx, cmt.ID, ref)
		if err != nil {
			return "", err
		}
		if anc {
			continue
		}
		err = r.git(ctx, "fetch", "--filter", "blob:none",
			"--shallow-since", cmt.OldestTime().Add(-time.Hour).Format(time.RFC3339),
			"origin", ref)
		if err != nil {
			return "", err
		}
	}

	// sort by ancestry, then by commit time, then by author time
	slices.SortFunc(parsed, func(a, b *commitObj) int {
		if a == b {
			return 0
		}
		if anc, _ := r.isAncestor(ctx, a.ID, b.ID); anc {
			return -1
		}
		if anc, _ := r.isAncestor(ctx, b.ID, a.ID); anc {
			return 1
		}
		return a.OldestTime().Compare(b.OldestTime())
	})

	ret := make([]string, len(commits))
	for i, cmt := range parsed {
		ret[i] = cmt.ID
	}
	return ret[len(ret)-1], nil
}
