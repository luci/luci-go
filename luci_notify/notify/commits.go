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

package notify

import (
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/sync/parallel"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

func commitIndex(commits []*gitpb.Commit, revision string) int {
	for i, commit := range commits {
		if commit.Id == revision {
			return i
		}
	}
	return -1
}

func commitsToGitilesCommits(commits map[string]string) notifypb.GitilesCommits {
	result := notifypb.GitilesCommits{
		Commits: make([]*buildbucketpb.GitilesCommit, 0, len(commits)),
	}
	for repo, commit := range commits {
		project, host, _ := gitiles.ParseRepoURL(repo)
		result.Commits = append(result.Commits, &buildbucketpb.GitilesCommit{
			Host: host,
			Project: project,
			Id: commit,
		})
	}
	return result
}

func filterRepositories(commits map[string]string, whitelist []string) {
	whiteset := stringset.NewFromSlice(whitelist...)
	for repo, _ := range commits {
		if !whiteset.Has(repo) {
			delete(commits, repo)
		}
	}
}

func computeGitLog(c context.Context, oldCommits map[string]string, newCommits map[string]string, history HistoryFunc) (map[string][]*gitpb.Commit, error) {
	var resultMu sync.Mutex
	result := make(map[string][]*gitpb.Commit)
	err := parallel.RunMulti(c, 8, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(ch chan<- func() error) {
			for repo, revision := range newCommits {
				repo := repo
				project, host, _ := gitiles.ParseRepoURL(repo)
				newRev := revision
				oldRev, ok := oldCommits[repo]
				if !ok {
					continue
				}
				ch <- func() error {
					log, err := history(c, host, project, oldRev, newRev)
					if err != nil {
						return err
					}
					resultMu.Lock()
					result[repo] = log
					resultMu.Unlock()
					return nil
				}
			}
		})
	})
	return result, err
}

func blamelistFromLogs(c context.Context, logs map[string][]*gitpb.Commit, commits map[string]string) stringset.Set {
	blamelist := stringset.New(0)
	for repo, commit := range commits {
		log, ok := logs[repo]
		if !ok {
			continue
		}
		index := commitIndex(log, commit)
		if index <= 0 {
			continue
		}
		for _, commit := range log[:index] {
			blamelist.Add(commit.Author.Email)
		}
	}
	return blamelist
}
