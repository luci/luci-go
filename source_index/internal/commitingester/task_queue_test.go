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
	"math/rand"
	"testing"

	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/commitingester/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProcessCommitIngestionTask(t *testing.T) {
	Convey(`ProcessCommitIngestionTask`, t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, config.TestCfg)
		So(err, ShouldBeNil)

		rng := rand.New(rand.NewSource(7671861294544766002))
		commits := gitiles.MakeFakeCommits(rng, 1001, nil)
		commits[100].Message = "no-position-2\n"
		commits[200].Message = "no-position-1\n"
		commits[300].Message = "with-position-2\nCr-Commit-Position: refs/heads/main@{#2}\n"
		commits[400].Message = "with-position-1\nCr-Commit-Position: refs/heads/main@{#1}\n"
		commits[500].Message = "with-invalid-2\nCr-Commit-Position: invalid 2\n"
		commits[600].Message = "with-invalid-1\nCr-Commit-Position: invalid 1\n"
		commitsInPage := commits[:1000]

		fakeGitilesClient := &gitiles.Fake{}

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
			So(err, ShouldBeNil)
			expectedSavedCommits = append(expectedSavedCommits, commit.NewFromGitCommit(gitCommit))
		}
		assertSavedCommitsExpected := func() string {
			actualSavedCommits, err := commit.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)

			return commit.ShouldEqualCommitSet(actualSavedCommits, expectedSavedCommits)
		}

		expectedTasks := []*taskspb.IngestCommits{
			{
				Host:       host,
				Repository: repository,
				Commitish:  commits[0].Id,
				PageToken:  commits[999].Id,
				TaskIndex:  1,
			},
		}
		assertOutputTaskExpected := func() string {
			actualTasks := make([]*taskspb.IngestCommits, 0, len(skdr.Tasks().Payloads()))
			for _, payload := range skdr.Tasks().Payloads() {
				actualTasks = append(actualTasks, payload.(*taskspb.IngestCommits))
			}

			return ShouldResembleProto(actualTasks, expectedTasks)
		}

		Convey(`with next page`, func() {
			fakeGitilesClient.SetRepository(repository, nil, commits)

			err := processCommitIngestionTask(ctx, fakeGitilesClient, inputTask)

			So(err, ShouldBeNil)
			So(assertSavedCommitsExpected(), ShouldBeEmpty)
			So(assertOutputTaskExpected(), ShouldBeEmpty)
		})

		Convey(`without next page`, func() {
			commits = commits[:800]
			commits[len(commits)-1].Parents = nil
			commitsInPage = commits[:1000]
			fakeGitilesClient.SetRepository(repository, nil, commits)
			expectedSavedCommits = expectedSavedCommits[:800]
			expectedTasks = []*taskspb.IngestCommits{}

			err := processCommitIngestionTask(ctx, fakeGitilesClient, inputTask)

			So(err, ShouldBeNil)
			So(assertSavedCommitsExpected(), ShouldBeEmpty)
			So(assertOutputTaskExpected(), ShouldBeEmpty)
		})

		Convey(`with already ingested first commit`, func() {
			firstGitCommit, err := commit.NewGitCommit(host, repository, commits[0])
			So(err, ShouldBeNil)
			alreadySavedCommit := commit.NewFromGitCommit(firstGitCommit)
			So(commit.SetForTesting(ctx, alreadySavedCommit), ShouldBeNil)
			fakeGitilesClient.SetRepository(repository, nil, commits)
			expectedSavedCommits = []commit.Commit{alreadySavedCommit}
			expectedTasks = []*taskspb.IngestCommits{}

			err = processCommitIngestionTask(ctx, fakeGitilesClient, inputTask)

			So(err, ShouldBeNil)
			So(assertSavedCommitsExpected(), ShouldBeEmpty)
			So(assertOutputTaskExpected(), ShouldBeEmpty)
		})
	})
}
