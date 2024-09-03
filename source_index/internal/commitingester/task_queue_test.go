// Copyright 2024 The LUCI Authors.
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

package commitingester

import (
	"context"
	"math/rand"
	"testing"

	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/commitingester/internal/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/gitilesutil"
	"go.chromium.org/luci/source_index/internal/testutil"
)

func TestProcessCommitIngestionTask(t *testing.T) {
	ftt.Run(`ProcessCommitIngestionTask`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, config.TestCfg)
		assert.Loosely(t, err, should.BeNil)

		rng := rand.New(rand.NewSource(7671861294544766002))
		commits := gitilespb.MakeFakeCommits(rng, 1001, nil)
		commits[100].Message = "no-position-2\n"
		commits[200].Message = "no-position-1\n"
		commits[300].Message = "with-position-2\nCr-Commit-Position: refs/heads/main@{#2}\n"
		commits[400].Message = "with-position-1\nCr-Commit-Position: refs/heads/main@{#1}\n"
		commits[500].Message = "with-invalid-2\nCr-Commit-Position: invalid 2\n"
		commits[600].Message = "with-invalid-1\nCr-Commit-Position: invalid 1\n"
		commitsInPage := commits[:1000]

		fakeGitilesClient := &gitilespb.Fake{}
		ctx = gitilesutil.UseFakeClientFactory(ctx, func(
			ctx context.Context,
			host string,
			as auth.RPCAuthorityKind,
			opts ...auth.RPCOption,
		) (gitilespb.GitilesClient, error) {
			return fakeGitilesClient, nil
		})

		host := "chromium.googlesource.com"
		repository := "chromium/src"

		inputTask := &taskspb.IngestCommits{
			Host:       host,
			Repository: repository,
			Commitish:  commits[0].Id,
			PageToken:  "",
			TaskIndex:  0,
		}

		expectedSavedCommits := make([]commit.Commit, 0, len(commitsInPage))
		for _, c := range commitsInPage {
			gitCommit, err := commit.NewGitCommit(host, repository, c)
			assert.Loosely(t, err, should.BeNil)
			expectedSavedCommits = append(expectedSavedCommits, commit.NewFromGitCommit(gitCommit))
		}

		t.Run("with normal queue", func(t *ftt.Test) {
			expectedTasks := []*taskspb.IngestCommits{
				{
					Host:       host,
					Repository: repository,
					Commitish:  commits[0].Id,
					PageToken:  commits[999].Id,
					TaskIndex:  1,
				},
			}
			getOutputTasks := func() []*taskspb.IngestCommits {
				actualTasks := make([]*taskspb.IngestCommits, 0, len(skdr.Tasks().Payloads()))
				for _, payload := range skdr.Tasks().Payloads() {
					actualTasks = append(actualTasks, payload.(*taskspb.IngestCommits))
				}

				return actualTasks
			}

			t.Run(`with next page`, func(t *ftt.Test) {
				fakeGitilesClient.SetRepository(repository, nil, commits)

				err := processCommitIngestionTask(ctx, inputTask, false)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.MustReadAllForTesting(span.Single(ctx)), commit.ShouldMatchCommits(expectedSavedCommits))
				assert.That(t, getOutputTasks(), should.Match(expectedTasks))
			})

			t.Run(`without next page`, func(t *ftt.Test) {
				commits = commits[:800]
				commits[len(commits)-1].Parents = nil
				commitsInPage = commits[:1000]
				fakeGitilesClient.SetRepository(repository, nil, commits)
				expectedSavedCommits = expectedSavedCommits[:800]
				expectedTasks = []*taskspb.IngestCommits{}

				err := processCommitIngestionTask(ctx, inputTask, false)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.MustReadAllForTesting(span.Single(ctx)), commit.ShouldMatchCommits(expectedSavedCommits))
				assert.That(t, getOutputTasks(), should.Match(expectedTasks))
			})

			t.Run(`with already ingested first commit`, func(t *ftt.Test) {
				firstGitCommit, err := commit.NewGitCommit(host, repository, commits[0])
				assert.Loosely(t, err, should.BeNil)
				alreadySavedCommit := commit.NewFromGitCommit(firstGitCommit)
				commit.MustSetForTesting(ctx, alreadySavedCommit)
				fakeGitilesClient.SetRepository(repository, nil, commits)
				expectedSavedCommits = []commit.Commit{alreadySavedCommit}
				expectedTasks = []*taskspb.IngestCommits{}

				err = processCommitIngestionTask(ctx, inputTask, false)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.MustReadAllForTesting(span.Single(ctx)), commit.ShouldMatchCommits(expectedSavedCommits))
				assert.That(t, getOutputTasks(), should.Match(expectedTasks))
			})
		})

		t.Run("with backfill queue", func(t *ftt.Test) {
			expectedTasks := []*taskspb.IngestCommitsBackfill{
				{
					Host:       host,
					Repository: repository,
					Commitish:  commits[0].Id,
					PageToken:  commits[999].Id,
					TaskIndex:  1,
				},
			}
			getOutputTasks := func() []*taskspb.IngestCommitsBackfill {
				actualTasks := make([]*taskspb.IngestCommitsBackfill, 0, len(skdr.Tasks().Payloads()))
				for _, payload := range skdr.Tasks().Payloads() {
					actualTasks = append(actualTasks, payload.(*taskspb.IngestCommitsBackfill))
				}

				return actualTasks
			}

			t.Run(`with next page`, func(t *ftt.Test) {
				fakeGitilesClient.SetRepository(repository, nil, commits)

				err := processCommitIngestionTask(ctx, inputTask, true)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, commit.MustReadAllForTesting(span.Single(ctx)), commit.ShouldMatchCommits(expectedSavedCommits))
				assert.That(t, getOutputTasks(), should.Match(expectedTasks))
			})
		})
	})
}
