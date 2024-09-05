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
	"fmt"
	"math/rand"
	"testing"

	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commitingester/internal/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/gitilesutil"
	"go.chromium.org/luci/source_index/internal/testutil"
)

func TestSyncCommitsHandler(t *testing.T) {
	ftt.Run(`SyncCommitsHandler`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, config.TestCfg)
		assert.Loosely(t, err, should.BeNil)

		// Which commit a ref points to doesn't matter.
		// Create some random commits and point refs to them.
		rng := rand.New(rand.NewSource(9016518488350932606))
		commits := gitilespb.MakeFakeCommits(rng, 3456, nil)
		refs := map[string]string{
			"refs/branch-heads/release-101": commits[0].Id,
			"refs/branch-heads/release-102": commits[1].Id,
			"refs/heads/main":               commits[2].Id,
			"refs/heads/staging-snapshot":   commits[3].Id,
			"refs/arbitrary/my-branch":      commits[4].Id,
			"refs/heads/another-branch":     commits[5].Id,
			"refs/tags/a-tag":               commits[6].Id,
		}

		fakeChromiumGitilesClient := &gitilespb.Fake{}
		fakeWebRTCGitilesClient := &gitilespb.Fake{}

		ctx = gitilesutil.UseFakeClientFactory(ctx, func(ctx context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (gitilespb.GitilesClient, error) {
			switch host {
			case "chromium.googlesource.com":
				return fakeChromiumGitilesClient, nil
			case "webrtc.googlesource.com":
				return fakeWebRTCGitilesClient, nil
			default:
				return nil, errors.Reason("unexpected host %q", host).Err()
			}
		})

		type taskKey struct {
			host      string
			repo      string
			commitish string
		}
		expectedTasks := map[taskKey]*taskspb.IngestCommits{
			{host: "chromium.googlesource.com", repo: "chromium/src", commitish: "refs/branch-heads/release-101"}: {
				Host:       "chromium.googlesource.com",
				Repository: "chromium/src",
				Commitish:  "refs/branch-heads/release-101",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
			{host: "chromium.googlesource.com", repo: "chromium/src", commitish: "refs/branch-heads/release-102"}: {
				Host:       "chromium.googlesource.com",
				Repository: "chromium/src",
				Commitish:  "refs/branch-heads/release-102",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
			{host: "chromium.googlesource.com", repo: "chromium/src", commitish: "refs/heads/main"}: {
				Host:       "chromium.googlesource.com",
				Repository: "chromium/src",
				Commitish:  "refs/heads/main",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
			{host: "chromium.googlesource.com", repo: "chromiumos/manifest", commitish: "refs/heads/staging-snapshot"}: {
				Host:       "chromium.googlesource.com",
				Repository: "chromiumos/manifest",
				Commitish:  "refs/heads/staging-snapshot",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
			{host: "chromium.googlesource.com", repo: "v8/v8", commitish: "refs/heads/main"}: {
				Host:       "chromium.googlesource.com",
				Repository: "v8/v8",
				Commitish:  "refs/heads/main",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
			{host: "webrtc.googlesource.com", repo: "src", commitish: "refs/heads/main"}: {
				Host:       "webrtc.googlesource.com",
				Repository: "src",
				Commitish:  "refs/heads/main",
				PageToken:  "",
				TaskIndex:  0,
				Backfill:   true,
			},
		}

		getOutputTasks := func() map[taskKey]*taskspb.IngestCommits {
			actualTasks := make(map[taskKey]*taskspb.IngestCommits, len(skdr.Tasks().Payloads()))
			for _, payload := range skdr.Tasks().Payloads() {
				t := payload.(*taskspb.IngestCommits)
				actualTasks[taskKey{host: t.Host, repo: t.Repository, commitish: t.Commitish}] = t
			}

			return actualTasks
		}

		t.Run(`e2e`, func(t *ftt.Test) {
			fakeChromiumGitilesClient.SetRepository("chromium/src", refs, commits)
			fakeChromiumGitilesClient.SetRepository("chromiumos/manifest", refs, commits)
			fakeChromiumGitilesClient.SetRepository("v8/v8", refs, commits)
			fakeWebRTCGitilesClient.SetRepository("src", refs, commits)

			err := syncCommitsHandler(ctx)

			assert.Loosely(t, err, should.BeNil)
			assert.That(t, getOutputTasks(), should.Match(expectedTasks))
		})

		t.Run(`when there are RPC failures`, func(t *ftt.Test) {
			fakeChromiumGitilesClient.SetRepository("chromium/src", refs, commits)
			fakeChromiumGitilesClient.SetRepository("v8/v8", refs, commits)
			delete(expectedTasks, taskKey{host: "chromium.googlesource.com", repo: "chromiumos/manifest", commitish: "refs/heads/staging-snapshot"})
			delete(expectedTasks, taskKey{host: "webrtc.googlesource.com", repo: "src", commitish: "refs/heads/main"})

			err := syncCommitsHandler(ctx)

			assert.Loosely(t, err, should.NotBeNil)
			assert.That(t, getOutputTasks(), should.Match(expectedTasks))
		})

		t.Run(`when there are many tasks to be scheduled`, func(t *ftt.Test) {
			assert.That(t, len(commits), should.BeGreaterThan(2000))
			refs = make(map[string]string, len(commits))
			expectedTasks = make(map[taskKey]*taskspb.IngestCommits, len(commits))
			for i, commit := range commits {
				ref := fmt.Sprintf("refs/branch-heads/release-%d", i)
				refs[ref] = commit.Id
				expectedTasks[taskKey{host: "chromium.googlesource.com", repo: "chromium/src", commitish: ref}] = &taskspb.IngestCommits{
					Host:       "chromium.googlesource.com",
					Repository: "chromium/src",
					Commitish:  ref,
					PageToken:  "",
					TaskIndex:  0,
					Backfill:   true,
				}
			}
			fakeChromiumGitilesClient.SetRepository("chromium/src", refs, commits)

			err := syncCommitsHandler(ctx)

			assert.Loosely(t, err, should.NotBeNil)
			assert.That(t, getOutputTasks(), should.Match(expectedTasks))
		})
	})
}
