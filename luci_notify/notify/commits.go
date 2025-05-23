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
	"context"
	"sort"
	"strings"
	"sync"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/sync/parallel"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

// commitIndex finds the index of the given revision inside the list of
// Git commits, or -1 if the revision is not found.
func commitIndex(commits []*gitpb.Commit, revision string) int {
	for i, commit := range commits {
		if commit.Id == revision {
			return i
		}
	}
	return -1
}

// commitsBlamelist computes EmailNotify consisting of the blamelist (commit author
// emails) for a list of commits.
func commitsBlamelist(commits []*gitpb.Commit, template string) []EmailNotify {
	blamelist := stringset.New(len(commits))
	for _, commit := range commits {
		blamelist.Add(commit.Author.Email)
	}
	recipients := make([]EmailNotify, 0, len(blamelist))
	for recipient := range blamelist {
		recipients = append(recipients, EmailNotify{
			Email:    recipient,
			Template: template,
		})
	}
	sortEmailNotify(recipients)
	return recipients
}

// Checkout represents a Git checkout of multiple repositories. It is a
// mapping of repository URLs to Git revisions.
type Checkout map[string]string

// NewCheckout creates a new Checkout populated with the repositories and revision
// found in the GitilesCommits object.
func NewCheckout(commits *notifypb.GitilesCommits) Checkout {
	results := make(Checkout, len(commits.GetCommits()))
	for _, gitilesCommit := range commits.GetCommits() {
		results[protoutil.GitilesRepoURL(gitilesCommit)] = gitilesCommit.Id
	}
	return results
}

// ToGitilesCommits converts the Checkout into a set of GitilesCommits which may
// be stored as part of a config.Builder.
func (c Checkout) ToGitilesCommits() *notifypb.GitilesCommits {
	if len(c) == 0 {
		return nil
	}
	result := &notifypb.GitilesCommits{
		Commits: make([]*buildbucketpb.GitilesCommit, 0, len(c)),
	}
	for repo, commit := range c {
		host, project, _ := gitiles.ParseRepoURL(repo)
		result.Commits = append(result.Commits, &buildbucketpb.GitilesCommit{
			Host:    host,
			Project: project,
			Id:      commit,
		})
	}
	// Sort commits, first by host, then by project.
	sort.Slice(result.Commits, func(i, j int) bool {
		first := result.Commits[i]
		second := result.Commits[j]
		hostResult := strings.Compare(first.Host, second.Host)
		if hostResult == 0 {
			return strings.Compare(first.Project, second.Project) < 0
		}
		return hostResult < 0
	})
	return result
}

// Filter filters out repositories from the Checkout which are not in the allowlist
// and returns a new Checkout.
func (c Checkout) Filter(allowset stringset.Set) Checkout {
	newCheckout := make(Checkout)
	for repo, commit := range c {
		if allowset.Has(repo) {
			newCheckout[repo] = commit
		}
	}
	return newCheckout
}

// Logs represents a set of Git diffs between two Checkouts.
//
// It is a mapping of repository URLs to a list of Git commits, representing
// the Git log for that repository.
type Logs map[string][]*gitpb.Commit

// ComputeLogs produces a set of Git diffs between oldCheckout and newCheckout,
// using the repositories in the newCheckout. historyFunc is used to grab
// the Git history.
func ComputeLogs(c context.Context, luciProject string, oldCheckout, newCheckout Checkout, history HistoryFunc) (Logs, error) {
	var resultMu sync.Mutex
	result := make(Logs)
	err := parallel.WorkPool(8, func(ch chan<- func() error) {
		for repo, revision := range newCheckout {
			newRev := revision
			oldRev, ok := oldCheckout[repo]
			if !ok {
				continue
			}
			host, project, _ := gitiles.ParseRepoURL(repo)
			ch <- func() error {
				log, err := history(c, luciProject, host, project, oldRev, newRev)
				if err != nil {
					return err
				}
				if len(log) <= 1 {
					return nil
				}
				resultMu.Lock()
				result[repo] = log[:len(log)-1]
				resultMu.Unlock()
				return nil
			}
		}
	})
	return result, err
}

// Filter filters out repositories from the Logs which are not in the allowlist
// and returns a new Logs.
func (l Logs) Filter(allowset stringset.Set) Logs {
	newLogs := make(Logs)
	for repo, commits := range l {
		if allowset.Has(repo) {
			newLogs[repo] = commits
		}
	}
	return newLogs
}

// Blamelist computes a set of email notifications from the Logs.
func (l Logs) Blamelist(template string) []EmailNotify {
	blamelist := stringset.New(0)
	for _, log := range l {
		for _, commit := range log {
			blamelist.Add(commit.Author.Email)
		}
	}
	recipients := make([]EmailNotify, 0, len(blamelist))
	for recipient := range blamelist {
		recipients = append(recipients, EmailNotify{
			Email:    recipient,
			Template: template,
		})
	}
	sortEmailNotify(recipients)
	return recipients
}
