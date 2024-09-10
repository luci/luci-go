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
	"fmt"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	recipe "go.chromium.org/luci/cv/api/recipe/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
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
		assert.Loosely(t, computeReuseKey(cls), should.Equal("2Yh+hI8zJZFe8ac1TrrFjATWGjhiV9aXsKjNJIhzATk="))
	})
}
