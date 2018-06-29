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
	"container/heap"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gitilesapi "go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/sync/parallel"
)

// A structure to keep a list of commits for some ref.
type refCommits struct {
	commits []*gitpb.Commit
}

// The pop method removes and returns first commit. Second return value is true
// if this was the last commit. Caller must ensure refCommits has commits when
// calling the method.
func (rc *refCommits) pop() (commit *gitpb.Commit, empty bool) {
	commit, rc.commits = rc.commits[0], rc.commits[1:]
	return commit, len(rc.commits) == 0
}

// We use commitHeap to merge slices of commits using max-heap algorithm below.
// Only first commit in each slice is used for comparisons.
type commitHeap []refCommits

func (h commitHeap) Len() int {
	return len(h)
}

func (h commitHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h commitHeap) Less(i, j int) bool {
	iTime := google.TimeFromProto(h[i].commits[0].Committer.Time)
	jTime := google.TimeFromProto(h[j].commits[0].Committer.Time)

	// Ensure consistent ordering based on commmit hash when times are identical.
	if iTime == jTime {
		return h[i].commits[0].Id > h[j].commits[0].Id
	}

	// To make heap behave as max-heap, we consider later time to be smaller than
	// earlier timer, i.e. latest commit will be the at the root of the heap.
	return iTime.After(jTime)
}

func (h *commitHeap) Push(x interface{}) {
	*h = append(*h, x.(refCommits))
}

func (h *commitHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// CombinedLogs implements Client interface.
func (i *implementation) CombinedLogs(c context.Context, host, project, excludeRef string, refs []string, limit int) (commits []*gitpb.Commit, err error) {
	defer func() { err = errors.Annotate(tagError(c, err), "gitiles.CombinedLogs").Err() }()

	// Check if the user is allowed to access this project.
	allowed, err := i.acls.IsAllowed(c, host, project)
	switch {
	case err != nil:
		return
	case !allowed:
		err = status.Errorf(codes.NotFound, "not found")
		return
	}

	// Prepare Gitiles client.
	client, err := i.gitilesClient(c, host)
	if err != nil {
		return
	}

	// Resolve all refs and commits they are pointing at.
	refTips, err := gitilesapi.NewRefSet(refs).Resolve(c, client, project)
	if err != nil {
		return
	}

	// We merge commits from all refs sorted by time into a single list up to a
	// limit. We use max-heap based merging algorithm below.
	h := make(commitHeap, 0, len(refTips))

	// Fetch commits from all matching refs and add lists of commits to the heap.
	lock := sync.Mutex{} // for concurrent writes to the heap
	if err = parallel.FanOutIn(func(ch chan<- func() error) {
		for _, commit := range refTips {
			commit := commit
			ch <- func() error {
				log, err := i.log(c, host, project, commit, excludeRef, &LogOptions{Limit: limit})
				if err != nil {
					return err
				}

				lock.Lock()
				defer lock.Unlock()
				if len(log) > 0 {
					heap.Push(&h, refCommits{log})
				}
				return nil
			}
		}
	}); err != nil {
		return
	}

	// Keep adding commits to the merged list until we reach the limit or run out
	// of commits on all refs.
	commits = make([]*gitpb.Commit, 0, limit)
	for len(commits) < limit && len(h) != 0 {
		commit, empty := h[0].pop()
		// Do not add duplicate commits that come from different refs.
		if len(commits) == 0 || commits[len(commits)-1].Id != commit.Id {
			commits = append(commits, commit)
		}
		if empty {
			heap.Remove(&h, 0)
		} else {
			heap.Fix(&h, 0)
		}
	}

	return
}
