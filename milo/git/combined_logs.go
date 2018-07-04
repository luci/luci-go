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
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/gae/service/datastore"
	gitilesapi "go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

// logCache stores a cached list of commits (log) for a given ref at a given
// commit position return by Gerrit. They Key decribes the query that was used
// to retrieve the log and follows the following format:
//
//   host|project|ref|exclude_ref|limit
//
// When the ref moves, entity is updated with the new CommitID and updated Log.
// The Log field is an encoded list of commits, which is a created by encoding a
// varint for the number of commits in the list followed by the corresponding
// number of serialized gitpb.Commit messages.
type logCache struct {
	Key      string `gae:"$id"`
	CommitID string `gae:"commit"`
	Log      []byte `gae:"log"`
}

func logCacheFor(host, project, ref, excludeRef string, limit int) logCache {
	return logCache{Key: fmt.Sprintf("%s|%s|%s|%s|%d", host, project, ref, excludeRef, limit)}
}

func loadDSCache(c context.Context, host, project, excludeRef string, limit int, refTips map[string]string) (cachedLogs map[string][]*gitpb.Commit, missingRefs []string) {
	refs := make([]string, 0, len(refTips))
	caches := make([]logCache, 0, len(refTips))
	for ref := range refTips {
		refs = append(refs, ref)
		caches = append(caches, logCacheFor(host, project, ref, excludeRef, limit))
	}

	cachedLogs = make(map[string][]*gitpb.Commit)
	missingRefs = make([]string, 0)
	err := datastore.Get(c, caches)

	var merr errors.MultiError
	if err == nil {
		merr = make([]error, len(caches))
	} else {
		var ok bool
		if merr, ok = err.(errors.MultiError); !ok {
			return
		}
	}

	for i, cache := range caches {
		ref := refs[i]
		if merr[i] != nil || cache.CommitID != refTips[ref] {
			missingRefs = append(missingRefs, ref)
			continue
		}

		buf := proto.NewBuffer(cache.Log)
		numCommits, err := buf.DecodeVarint()
		if err != nil {
			missingRefs = append(missingRefs, ref)
			continue
		}

		log := make([]*gitpb.Commit, 0, numCommits)
		for i := uint64(0); i < numCommits; i++ {
			var commit gitpb.Commit
			if err = buf.DecodeMessage(&commit); err != nil {
				missingRefs = append(missingRefs, ref)
				continue
			}

			log = append(log, &commit)
		}

		cachedLogs[ref] = log
	}

	return
}

func saveDSCache(c context.Context, host, project, excludeRef string, limit int, refLogs map[string][]*gitpb.Commit) error {
	caches := make([]logCache, 0, len(refLogs))
	for ref, log := range refLogs {
		buf := proto.NewBuffer([]byte{})
		if err := buf.EncodeVarint(uint64(len(log))); err != nil {
			return err
		}

		for _, commit := range log {
			if err := buf.EncodeMessage(commit); err != nil {
				return err
			}
		}

		cache := logCacheFor(host, project, ref, excludeRef, limit)
		cache.CommitID = log[0].Id
		cache.Log = buf.Bytes()
		caches = append(caches, cache)
	}

	return datastore.Put(c, caches)
}

func (impl *implementation) loadLogsForRefs(c context.Context, host, project, excludeRef string, limit int, refTips map[string]string) (logs [][]*gitpb.Commit, err error) {
	cachedLogs, missingRefs := loadDSCache(c, host, project, excludeRef, limit, refTips)

	// Load missing logs from Gitiles.
	newLogs := make(map[string][]*gitpb.Commit)
	lock := sync.Mutex{} // for concurrent writes to the map
	err = parallel.FanOutIn(func(ch chan<- func() error) {
		for _, ref := range missingRefs {
			ref := ref
			ch <- func() error {
				log, err := impl.log(c, host, project, refTips[ref], excludeRef, &LogOptions{Limit: limit})
				if err != nil {
					return err
				}

				lock.Lock()
				defer lock.Unlock()
				newLogs[ref] = log
				return nil
			}
		}
	})

	if err := saveDSCache(c, host, project, excludeRef, limit, newLogs); err != nil {
		logging.WithError(err).Warningf(c, "Failed to cache logs fetched from Gitiles")
	}

	// Drop ref names and create a list containing all logs.
	logs = make([][]*gitpb.Commit, 0, len(cachedLogs)+len(newLogs))
	for _, log := range cachedLogs {
		logs = append(logs, log)
	}
	for _, log := range newLogs {
		logs = append(logs, log)
	}

	return
}

// CombinedLogs implements Client interface.
func (impl *implementation) CombinedLogs(c context.Context, host, project, excludeRef string, refs []string, limit int) (commits []*gitpb.Commit, err error) {
	defer func() { err = errors.Annotate(tagError(c, err), "gitiles.CombinedLogs").Err() }()

	// Check if the user is allowed to access this project.
	allowed, err := impl.acls.IsAllowed(c, host, project)
	switch {
	case err != nil:
		return
	case !allowed:
		err = status.Errorf(codes.NotFound, "not found")
		return
	}

	// Prepare Gitiles client.
	client, err := impl.gitilesClient(c, host)
	if err != nil {
		return
	}

	// Resolve all refs and commits they are pointing at.
	refTips, err := gitilesapi.NewRefSet(refs).Resolve(c, client, project)
	if err != nil {
		return
	}

	var logs [][]*gitpb.Commit
	if logs, err = impl.loadLogsForRefs(c, host, project, excludeRef, limit, refTips); err != nil {
		return
	}

	// We merge commits from all refs sorted by time into a single list up to a
	// limit. We use max-heap based merging algorithm below.
	var h commitHeap
	for _, log := range logs {
		if len(log) > 0 {
			h = append(h, refCommits{log})
		}
	}

	// Keep adding commits to the merged list until we reach the limit or run out
	// of commits on all refs.
	heap.Init(&h)
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
