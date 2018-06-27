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
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/sync/parallel"
)

// We use commitHeap to merge slices of commits using max-heap algorithm below.
type refCommit struct {
	ref    string
	commit *gitpb.Commit
}
type commitHeap []refCommit

func (h commitHeap) Len() int      { return len(h) }
func (h commitHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h commitHeap) Less(i, j int) bool {
	// To make heap behave as max-heap, we consider later time to be smaller than
	// earlier timer, i.e. latest commit will be the at the root of the heap.
	return google.TimeFromProto(h[i].commit.Committer.Time).After(
		google.TimeFromProto(h[j].commit.Committer.Time))
}
func (h *commitHeap) Push(x interface{}) { *h = append(*h, x.(refCommit)) }
func (h *commitHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// FetchRefs implements Client interface.
func (i *implementation) FetchRefs(c context.Context, host, project, startRef string, refConfigs []string, limit int) (commits []*gitpb.Commit, err error) {
	defer func() { err = errors.Annotate(tagError(c, err), "gitiles.FetchRefs").Err() }()

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

	// Request all refs and their tips that match refConfigs.
	rs := gitilesapi.NewRefSet(refConfigs)
	refTips := map[string]string{}
	lock := sync.Mutex{} // for concurrent writes to the maps
	if err = parallel.FanOutIn(func(work chan<- func() error) {
		rs.ForEachPrefix(func(prefix string) {
			work <- func() error {
				resp, err := client.Refs(
					c, &gitilespb.RefsRequest{Project: project, RefsPath: prefix})
				if err != nil {
					return err
				}
				lock.Lock()
				defer lock.Unlock()
				for ref, tip := range resp.Revisions {
					if rs.Has(ref) {
						refTips[ref] = tip
					}
				}
				return nil
			}
		})
	}); err != nil {
		return
	}

	// Fetch commits from all matching refs.
	logs := map[string][]*gitpb.Commit{}
	if err = parallel.FanOutIn(func(ch chan<- func() error) {
		for ref, commit := range refTips {
			ref, commit := ref, commit
			ch <- func() (err error) {
				lock.Lock()
				defer lock.Unlock()
				logs[ref], err = i.log(
					c, host, project, commit, startRef, &LogOptions{Limit: limit})
				return
			}
		}
	}); err != nil {
		return
	}

	// Merge commits from all refs sorted by time into a single list up to a
	// limit. We use max-heap based merging algorithm. This assumes that commits
	// in each ref are already sorted by time.

	// Initialize heap with first commits from each ref.
	ch := commitHeap{}
	for ref, commits := range logs {
		if len(commits) > 0 {
			ch = append(ch, refCommit{ref, commits[0]})
			logs[ref] = logs[ref][1:]
		}
	}
	heap.Init(&ch)

	// Keep adding commits to the merged list until we reach the limit or run out
	// of commits on all refs.
	commits = make([]*gitpb.Commit, 0, limit)
	for len(commits) != limit && len(ch) != 0 {
		// Add latest commit to the merged list and replace it with another commit
		// from the same ref in the heap (unless ref has no more commits).
		ref, commit := ch[0].ref, ch[0].commit
		commits = append(commits, commit)
		if len(logs[ref]) > 0 {
			ch[0].commit = logs[ref][0]
			logs[ref] = logs[ref][1:]
			heap.Fix(&ch, 0)
		} else {
			heap.Remove(&ch, 0)
		}
	}

	return
}
