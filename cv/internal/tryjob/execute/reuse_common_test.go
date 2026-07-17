// Copyright 2022 The LUCI Authors.
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

package execute

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	recipe "go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gitiles"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestCanReuse(t *testing.T) {
	t.Parallel()

	ftt.Run("canReuseTryjob works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		t.Run("reuse allowed", func(t *ftt.Test) {
			t.Run("empty mode allowlist", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
						Status:     tryjob.Result_SUCCEEDED,
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseAllowed))
			})

			t.Run("explicitly allowed in mode allowlist", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
						Status:     tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Reusability: &recipe.Output_Reusability{
								ModeAllowlist: []string{string(run.DryRun), string(run.FullRun)},
							},
						},
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseAllowed))
			})
		})

		t.Run("reuse maybe", func(t *ftt.Test) {
			t.Run("triggered fresh tryjob", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_TRIGGERED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge / 2)),
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseMaybe))
			})

			t.Run("pending tryjob", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_PENDING,
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseMaybe))
			})
		})

		t.Run("reuse denied", func(t *ftt.Test) {
			t.Run("triggered stale tryjob", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_TRIGGERED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseDenied))
			})

			t.Run("successfully ended tryjob but stale", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_SUCCEEDED,
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseDenied))
			})

			t.Run("failed tryjob", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_FAILED_PERMANENTLY,
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseDenied))
			})

			t.Run("not in the mode allowlist", func(t *ftt.Test) {
				tj := &tryjob.Tryjob{
					Status: tryjob.Status_ENDED,
					Result: &tryjob.Result{
						CreateTime: timestamppb.New(ct.Clock.Now().Add(-staleTryjobAge * 2)),
						Status:     tryjob.Result_SUCCEEDED,
						Output: &recipe.Output{
							Reusability: &recipe.Output_Reusability{
								ModeAllowlist: []string{string(run.DryRun)},
							},
						},
					},
				}
				assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseDenied))
			})

			for _, st := range []tryjob.Status{tryjob.Status_CANCELLED, tryjob.Status_UNTRIGGERED} {
				t.Run(fmt.Sprintf("status is %s", st), func(t *ftt.Test) {
					tj := &tryjob.Tryjob{Status: st}
					assert.Loosely(t, canReuseTryjob(ctx, tj, run.FullRun), should.Equal(reuseDenied))
				})
			}
		})
	})
}

func TestComputeReuseKey(t *testing.T) {
	t.Parallel()

	ftt.Run("computeReuseKey works", t, func(t *ftt.Test) {
		cls := []*run.RunCL{
			{
				ID: 22222,
				Detail: &changelist.Snapshot{
					MinEquivalentPatchset: 22,
				},
			},
			{
				ID: 11111,
				Detail: &changelist.Snapshot{
					MinEquivalentPatchset: 11,
				},
			},
		}
		// Should yield the same result as
		// > python3 -c 'import base64;from hashlib import sha256;print(base64.b64encode(sha256(b"\0".join(sorted(b"%d/%d"%(x[0], x[1]) for x in [[22222,22],[11111,11]]))).digest()))'
		assert.Loosely(t, computeReuseKey(cls, nil), should.Equal("2Yh+hI8zJZFe8ac1TrrFjATWGjhiV9aXsKjNJIhzATk="))
	})

	ftt.Run("computeReuseKey includes footers", t, func(t *ftt.Test) {
		disableReuseFooters := []string{"Special-Footer1", "Special-Footer2"}

		tests := []struct {
			name        string
			original    *run.RunCL
			mutate      func(*run.RunCL)
			expectEqual bool
		}{
			{
				name:        "no-op",
				mutate:      func(*run.RunCL) {},
				expectEqual: true,
			},
			{
				name: "add non-disable-reuse footer",
				mutate: func(cl *run.RunCL) {
					cl.Detail.Metadata = append(cl.Detail.Metadata,
						&changelist.StringPair{
							Key:   "Footer",
							Value: "bar",
						})
				},
				expectEqual: true,
			},
			{
				name: "change value of non-disable-reuse footer",
				original: &run.RunCL{Detail: &changelist.Snapshot{
					Metadata: []*changelist.StringPair{
						{
							Key:   "Footer",
							Value: "bar",
						},
					},
				}},
				mutate: func(cl *run.RunCL) {
					cl.Detail.Metadata[0].Value += "blah"
				},
				expectEqual: true,
			},
			{
				name: "add disable-reuse footer",
				mutate: func(cl *run.RunCL) {
					cl.Detail.Metadata = append(cl.Detail.Metadata,
						&changelist.StringPair{
							Key:   "Special-Footer1",
							Value: "bar",
						})
				},
				expectEqual: false,
			},
			{
				name: "change value of disable-reuse footer",
				original: &run.RunCL{Detail: &changelist.Snapshot{
					Metadata: []*changelist.StringPair{
						{
							Key:   "Special-Footer1",
							Value: "bar",
						},
					},
				}},
				mutate: func(cl *run.RunCL) {
					cl.Detail.Metadata[0].Value += "blah"
				},
				expectEqual: false,
			},
			{
				name: "change order of disable-reuse footers",
				original: &run.RunCL{Detail: &changelist.Snapshot{
					Metadata: []*changelist.StringPair{
						{
							Key:   "Special-Footer1",
							Value: "bar",
						},
						{
							Key:   "Special-Footer2",
							Value: "bar",
						},
					},
				}},
				mutate: func(cl *run.RunCL) {
					slices.Reverse(cl.Detail.Metadata)
				},
				expectEqual: true,
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *ftt.Test) {
				cl := test.original
				if cl == nil {
					cl = &run.RunCL{Detail: &changelist.Snapshot{}}
				}

				originalKey := computeReuseKey([]*run.RunCL{cl}, disableReuseFooters)

				test.mutate(cl)

				got := computeReuseKey([]*run.RunCL{cl}, disableReuseFooters)
				if test.expectEqual {
					assert.That(t, got, should.Equal(originalKey))
				} else {
					assert.That(t, got, should.NotEqual(originalKey))
				}
			})
		}
	})
}

func TestResolveBranchTip(t *testing.T) {
	t.Parallel()

	ftt.Run("resolveBranchTip works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		host := "chromium.googlesource.com"
		project := "chromium/src"
		ref := "refs/heads/main"

		t.Run("success", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/heads/main@{#1607281}",
				},
			})

			pos, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(1607281)))
		})

		t.Run("success on non-main branch", func(t *ftt.Test) {
			nonMainRef := "refs/branch-heads/7871"
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				nonMainRef: "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
			}, []*git.Commit{
				{
					Id:      "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3",
					Message: "Release commit message\n\nCr-Commit-Position: refs/branch-heads/7871@{#150}",
				},
			})

			pos, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, nonMainRef, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(150)))
		})

		t.Run("success on cherry-pick commit with quoted original footer", func(t *ftt.Test) {
			cherryPickRef := "refs/branch-heads/7922"
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				cherryPickRef: "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
			}, []*git.Commit{
				{
					Id:      "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4",
					Message: "Original change's description:\n> Cr-Commit-Position: refs/heads/main@{#1607281}\n\nCr-Commit-Position: refs/branch-heads/7922@{#42}",
				},
			})

			pos, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, cherryPickRef, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(42)))
		})

		t.Run("ref mismatch", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/branch-heads/7922@{#42}",
				},
			})

			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("branch ref \"refs/branch-heads/7922\" in Cr-Commit-Position footer does not match requested target ref \"refs/heads/main\""))
		})

		t.Run("caching works", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/heads/main@{#1607281}",
				},
			})

			// First call is a cache miss and queries Gitiles.
			pos, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(1607281)))

			// Update the repository state behind the cache.
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "f9e8d7c6b5a4f9e8d7c6b5a4f9e8d7c6b5a4f9e8",
			}, []*git.Commit{
				{
					Id:      "f9e8d7c6b5a4f9e8d7c6b5a4f9e8d7c6b5a4f9e8",
					Message: "Different commit\n\nCr-Commit-Position: refs/heads/main@{#9999999}",
				},
			})

			// Second call is a cache hit and returns the cached value instantly.
			// We pass a broken internet tool (failFactory{}) to prove that the
			// function answers directly from memory and never touches the network.
			pos, err = resolveBranchTip(ctx, failFactory{}, host, project, ref, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(1607281)))

			// Advance clock past TTL to expire cache.
			ct.Clock.Add(gitilesTipCacheTTL + 1*time.Second)

			// Third call with cache expired, querying Gitiles again.
			// Since we use failFactory{}, it should fail, proving it tried to hit the network.
			_, err = resolveBranchTip(ctx, failFactory{}, host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("failed to make Gitiles client"))

			// Fourth call using real factory, succeeding and getting the new value.
			pos, err = resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(9999999)))
		})

		t.Run("project isolation", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/heads/main@{#1607281}",
				},
			})

			// First call with the chrome project is a cache miss and queries
			// Gitiles.
			pos, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chrome")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(1607281)))

			// Update the repository state.
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "f9e8d7c6b5a4f9e8d7c6b5a4f9e8d7c6b5a4f9e8",
			}, []*git.Commit{
				{
					Id:      "f9e8d7c6b5a4f9e8d7c6b5a4f9e8d7c6b5a4f9e8",
					Message: "Different commit\n\nCr-Commit-Position: refs/heads/main@{#9999999}",
				},
			})

			// Second call with the v8 project is a cache miss due to a different
			// project namespace, queries Gitiles, and gets the new value.
			pos, err = resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "v8")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(9999999)))

			// Third call with the chrome project is a cache hit for chrome's key and
			// returns the old cached value.
			pos, err = resolveBranchTip(ctx, failFactory{}, host, project, ref, "chrome")
			assert.NoErr(t, err)
			assert.That(t, pos, should.Equal(int64(1607281)))
		})

		t.Run("Gitiles API error", func(t *ftt.Test) {
			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, "non-existent-project", ref, "chromium")
			assert.That(t, err, should.ErrLike("failed to get Gitiles log"))
		})

		t.Run("missing footer", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message without footer",
				},
			})

			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("missing Cr-Commit-Position footer"))
		})

		t.Run("multiple footers", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/heads/main@{#100}\nCr-Commit-Position: refs/heads/main@{#101}",
				},
			})

			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("multiple Cr-Commit-Position footers found (2)"))
		})

		t.Run("malformed footer", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: malformed",
				},
			})

			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("malformed Cr-Commit-Position"))
		})

		t.Run("non-integer position in footer", func(t *ftt.Test) {
			ct.GitilesFake.SetRepository(host, project, map[string]string{
				ref: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			}, []*git.Commit{
				{
					Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					Message: "Commit message\n\nCr-Commit-Position: refs/heads/main@{#abc}",
				},
			})

			_, err := resolveBranchTip(ctx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("malformed Cr-Commit-Position"))
		})

		t.Run("nil factory", func(t *ftt.Test) {
			_, err := resolveBranchTip(ctx, nil, host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("gitiles factory is nil"))
		})

		t.Run("MakeClient error", func(t *ftt.Test) {
			_, err := resolveBranchTip(ctx, failFactory{}, host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("failed to make Gitiles client"))
		})

		t.Run("canceled context", func(t *ftt.Test) {
			canceledCtx, cancel := context.WithCancel(ctx)
			cancel()
			_, err := resolveBranchTip(canceledCtx, ct.GitilesFactory(), host, project, ref, "chromium")
			assert.That(t, err, should.ErrLike("context canceled"))
		})
	})
}

type failFactory struct{}

func (failFactory) MakeClient(ctx context.Context, host, luciProject string) (gitiles.Client, error) {
	return nil, fmt.Errorf("injected MakeClient error")
}
