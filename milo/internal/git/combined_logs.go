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
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gitilesapi "go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/milo/internal/utils"
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
	iTime := h[i].commits[0].Committer.Time.AsTime()
	jTime := h[j].commits[0].Committer.Time.AsTime()

	// Ensure consistent ordering based on commit hash when times are identical.
	if iTime == jTime {
		return h[i].commits[0].Id > h[j].commits[0].Id
	}

	// To make heap behave as max-heap, we consider later time to be smaller than
	// earlier timer, i.e. latest commit will be the at the root of the heap.
	return iTime.After(jTime)
}

func (h *commitHeap) Push(x any) {
	*h = append(*h, x.(refCommits))
}

func (h *commitHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// logCache stores a cached list of commits (log) for a given ref at a given
// commit position return by Gerrit. The Key describes the query that was used
// to retrieve the log and follows the following format:
//
//	host|project|ref|exclude_ref|limit
//
// When the ref moves, entity is updated with the new CommitID and updated Log.
// The Log field is an encoded list of commits, which is a created by encoding a
// varint for the number of commits in the list followed by the corresponding
// number of serialized gitpb.Commit messages.
type logCache struct {
	Key      string `gae:"$id"`
	CommitID string `gae:"commit,noindex"`
	Log      []byte `gae:"log,noindex"`

	ref string `gae:"-"`
}

func logCacheFor(host, project, ref, excludeRef string, limit int) logCache {
	return logCache{
		Key: fmt.Sprintf("%s|%s|%s|%s|%d", host, project, ref, excludeRef, limit),
		ref: ref,
	}
}

func loadCacheFromDS(c context.Context, host, project, excludeRef string, limit int, refTips map[string]string) (cachedLogs map[string][]*gitpb.Commit) {
	items := make([]logCache, 0, len(refTips))
	for ref := range refTips {
		items = append(items, logCacheFor(host, project, ref, excludeRef, limit))
	}

	cachedLogs = map[string][]*gitpb.Commit{}
	var merr errors.MultiError
	switch err := datastore.Get(c, items).(type) {
	case errors.MultiError:
		merr = err
	case nil:
		merr = nil
	default:
		return
	}

	for i, item := range items {
		if (merr != nil && merr[i] != nil) || item.CommitID != refTips[item.ref] {
			continue
		}

		buf := proto.NewBuffer(item.Log)
		numCommits, err := buf.DecodeVarint()
		if err != nil {
			continue
		}

		log := make([]*gitpb.Commit, 0, numCommits)
		for range numCommits {
			var commit gitpb.Commit
			if err = buf.DecodeMessage(&commit); err != nil {
				continue
			}

			log = append(log, &commit)
		}

		cachedLogs[item.ref] = log
	}

	return
}

func saveCacheToDS(c context.Context, host, project, excludeRef string, limit int, refLogs map[string][]*gitpb.Commit, refTips map[string]string) error {
	items := make([]logCache, 0, len(refLogs))
	totalBytes := 0
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

		item := logCacheFor(host, project, ref, excludeRef, limit)
		item.CommitID = refTips[ref]
		item.Log = buf.Bytes()
		items = append(items, item)

		// This logic breaks storing caches into datastore into smaller requests to
		// avoid exceeding 1MB limit on datastore requests set by AppEngine.
		totalBytes += len(item.Log)
		if totalBytes > 512*1024 { // 0.5 MiB
			if err := datastore.Put(c, items); err != nil {
				return err
			}
			totalBytes = 0
			items = items[:0]
		}
	}

	return datastore.Put(c, items)
}

// maxGitilesLogRPCsPerRequest is the max number of Gitiles requests allowed per
// user request to avoid exceeding Gitiles quota.
const maxGitilesLogRPCsPerRequest = 50

func (impl *implementation) loadLogsForRefs(c context.Context, host, project, excludeRef string, limit int, refTips map[string]string) ([][]*gitpb.Commit, error) {
	cachedLogs := loadCacheFromDS(c, host, project, excludeRef, limit, refTips)
	logging.Infof(c, "Fetched %d logs from cache, will fetch remaining %d logs from Gitiles", len(cachedLogs), len(refTips)-len(cachedLogs))

	// Load missing logs from Gitiles.
	newLogs := make(map[string][]*gitpb.Commit)
	lock := sync.Mutex{} // for concurrent writes to the map
	err := parallel.WorkPool(8, func(ch chan<- func() error) {
		numRequests := 0
		for ref := range refTips {
			if _, ok := cachedLogs[ref]; ok {
				continue
			}

			if numRequests++; numRequests > maxGitilesLogRPCsPerRequest {
				ch <- func() error {
					// TODO(sergiyb,tandrii): if you have genuine need for this many refs
					// at once, implement a cron job that runs this very function
					// continuously to avoid bursts of gitiles traffic that will make Milo
					// not functional for the other projects.
					return errors.Fmt("too many refs are new or changed to be "+
						"fetched at once, stopping after %d. Check your config and/or "+
						"reload the page", maxGitilesLogRPCsPerRequest)
				}
				break
			}

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

	// Try to cache what we've fetched even if some requests failed.
	if derr := saveCacheToDS(c, host, project, excludeRef, limit, newLogs, refTips); derr != nil {
		logging.WithError(derr).Warningf(c, "Failed to cache logs fetched from Gitiles")
	}

	if err != nil {
		return nil, errors.Fmt("failed to fetch %d logs from Gitiles: %w", len(refTips)-len(cachedLogs)-len(newLogs), err)
	}

	// Drop ref names and create a list containing all logs.
	logs := make([][]*gitpb.Commit, 0, len(cachedLogs)+len(newLogs))
	for _, log := range cachedLogs {
		logs = append(logs, log)
	}
	for _, log := range newLogs {
		logs = append(logs, log)
	}

	return logs, nil
}

// CombinedLogs implements Client interface.
func (impl *implementation) CombinedLogs(c context.Context, host, project, excludeRef string, refs []string, limit int) (commits []*gitpb.Commit, err error) {
	defer func() { err = errors.WrapIf(utils.TagGRPC(c, err), "gitiles.CombinedLogs") }()

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
	refTips, missingRefs, err := gitilesapi.NewRefSet(refs).Resolve(c, client, project)
	if err != nil {
		return
	}
	if len(missingRefs) > 0 {
		logging.Warningf(c, "configured refs %s weren't resolved to any ref; either incorrect ACLs or redudant refs", missingRefs)
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
