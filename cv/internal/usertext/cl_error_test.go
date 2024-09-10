// Copyright 2021 The LUCI Authors.
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

package usertext

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
)

func TestFormatCLError(t *testing.T) {
	t.Parallel()

	ftt.Run("CLError formatting works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		const gHost = "x-review.googlesource.com"
		ci := gf.CI(
			43, gf.PS(2), gf.Project("re/po"), gf.Ref("refs/heads/main"),
			gf.CQ(+2, testclock.TestRecentTimeUTC, gf.U("user-1")),
			gf.Updated(testclock.TestRecentTimeUTC),
		)
		cl := &changelist.CL{
			Snapshot: &changelist.Snapshot{
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: ci,
					},
				},
			},
		}

		t.Run("Single CLError", func(t *ftt.Test) {
			// reason.Kind and mode are set in tests below as needed.
			reason := &changelist.CLError{Kind: nil}
			var mode run.Mode
			mustFormat := func() string {
				s, err := SFormatCLError(ctx, reason, cl, mode)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.NotContainSubstring("<no value>"))
				return s
			}
			t.Run("Lacks owner email", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_OwnerLacksEmail{
					OwnerLacksEmail: true,
				}
				assert.Loosely(t, mustFormat(), should.ContainSubstring("set preferred email at https://x-review.googlesource.com/settings/#EmailAddresses"))
			})
			t.Run("Not yet supported mode", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_UnsupportedMode{
					UnsupportedMode: "CUSTOM_RUN",
				}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(`its mode "CUSTOM_RUN" is not supported`))
			})
			t.Run("Depends on itself", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_SelfCqDepend{SelfCqDepend: true}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(`because it depends on itself`))
			})
			t.Run("CorruptGerrit", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_CorruptGerritMetadata{CorruptGerritMetadata: "foo is not bar"}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(`foo is not bar`))
			})
			t.Run("Watched by many config groups", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_WatchedByManyConfigGroups_{
					WatchedByManyConfigGroups: &changelist.CLError_WatchedByManyConfigGroups{
						ConfigGroups: []string{"first", "second"},
					},
				}
				s := mustFormat()
				assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				it is watched by more than 1 config group:
				  * first
				  * second

				Please
			`)))
				assert.Loosely(t, s, should.ContainSubstring(`current CL target ref is "refs/heads/main"`))
			})
			t.Run("Watched by many LUCI projects", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_WatchedByManyProjects_{
					WatchedByManyProjects: &changelist.CLError_WatchedByManyProjects{
						Projects: []string{"first", "second"},
					},
				}
				s := mustFormat()
				assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				it is watched by more than 1 LUCI project:
				  * first
				  * second

				Please
			`)))
			})
			t.Run("Invalid deps", func(t *ftt.Test) {
				// Save a CL snapshot for each dep.
				deps := make(map[int]*changelist.Dep, 3)
				for i := 101; i <= 102; i++ {
					depCL := changelist.MustGobID(gHost, int64(i)).MustCreateIfNotExists(ctx)
					depCL.Snapshot = &changelist.Snapshot{
						LuciProject:           "whatever",
						MinEquivalentPatchset: 1,
						Patchset:              2,
						ExternalUpdateTime:    timestamppb.New(testclock.TestRecentTimeUTC),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: gf.CI(i),
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, depCL), should.BeNil)
					deps[i] = &changelist.Dep{Clid: int64(depCL.ID)}
				}
				invalidDeps := &changelist.CLError_InvalidDeps{ /*set below*/ }
				reason.Kind = &changelist.CLError_InvalidDeps_{InvalidDeps: invalidDeps}

				t.Run("Unwatched", func(t *ftt.Test) {
					invalidDeps.Unwatched = []*changelist.Dep{deps[101]}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				are not watched by the same LUCI project:
				  * https://x-review.googlesource.com/c/101

				Please check Cq-Depend
			`)))
				})
				t.Run("WrongConfigGroup", func(t *ftt.Test) {
					invalidDeps.WrongConfigGroup = []*changelist.Dep{deps[101], deps[102]}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				its deps do not belong to the same config group:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`)))
				})
				t.Run("Singular Full Run with open dependencies", func(t *ftt.Test) {
					mode = run.FullRun
					invalidDeps.SingleFullDeps = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				in "FULL_RUN" mode because it has not yet submitted dependencies:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`)))
				})
				t.Run("Combinable not triggered deps", func(t *ftt.Test) {
					invalidDeps.CombinableUntriggered = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				its dependencies weren't CQ-ed at all:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`)))
				})
				t.Run("Combinable mode mismatch", func(t *ftt.Test) {
					mode = run.FullRun
					invalidDeps.CombinableMismatchedMode = []*changelist.Dep{deps[102], deps[101]}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring(text.Doc(`
				its mode "FULL_RUN" does not match mode on its dependencies:
				  * https://x-review.googlesource.com/c/101
				  * https://x-review.googlesource.com/c/102
			`)))
				})
				t.Run("Too many", func(t *ftt.Test) {
					invalidDeps.TooMany = &changelist.CLError_InvalidDeps_TooMany{Actual: 5, MaxAllowed: 4}
					s := mustFormat()
					assert.Loosely(t, s, should.ContainSubstring("has too many deps: 5 (max supported: 4)"))
				})
			})
			t.Run("Reuse of triggers", func(t *ftt.Test) {
				reason.Kind = &changelist.CLError_ReusedTrigger_{
					ReusedTrigger: &changelist.CLError_ReusedTrigger{Run: "some/123-1-run"},
				}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(`previously completed a Run ("some/123-1-run") triggered by the same vote(s)`))
			})
			t.Run("TrighgerDeps", func(t *ftt.Test) {
				tdeps := &changelist.CLError_TriggerDeps{
					PermissionDenied: []*changelist.CLError_TriggerDeps_PermissionDenied{
						{Clid: 1, Email: "voter@example.org"},
						{Clid: 2},
					},
					NotFound:            []int64{3, 4, 5},
					InternalGerritError: []int64{6},
				}
				// Save a CL snapshot for each dep.
				deps := make(map[int]*changelist.Dep, 6)
				for i := 1; i <= 6; i++ {
					depCL := changelist.MustGobID(gHost, int64(i)).MustCreateIfNotExists(ctx)
					depCL.Snapshot = &changelist.Snapshot{
						LuciProject:           "whatever",
						MinEquivalentPatchset: 1,
						Patchset:              2,
						ExternalUpdateTime:    timestamppb.New(testclock.TestRecentTimeUTC),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: gf.CI(i),
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, depCL), should.BeNil)
					deps[i] = &changelist.Dep{Clid: int64(depCL.ID)}
				}
				reason.Kind = &changelist.CLError_TriggerDeps_{TriggerDeps: tdeps}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(text.Doc(`
					failed to vote the CQ label on the following dependencies.
					  * https://x-review.googlesource.com/c/1 - no permission to vote on behalf of voter@example.org
					  * https://x-review.googlesource.com/c/2 - no permission to vote
					  * https://x-review.googlesource.com/c/3 - the CL no longer exists in Gerrit
					  * https://x-review.googlesource.com/c/4 - the CL no longer exists in Gerrit
					  * https://x-review.googlesource.com/c/5 - the CL no longer exists in Gerrit
					  * https://x-review.googlesource.com/c/6 - internal Gerrit error
				`)))
			})

			t.Run("DepRunFailed", func(t *ftt.Test) {
				// Save a CL snapshot for each dep.
				deps := make(map[int]*changelist.Dep, 2)
				for i := 1; i <= 6; i++ {
					depCL := changelist.MustGobID(gHost, int64(i)).MustCreateIfNotExists(ctx)
					depCL.Snapshot = &changelist.Snapshot{
						LuciProject:           "whatever",
						MinEquivalentPatchset: 1,
						Patchset:              2,
						ExternalUpdateTime:    timestamppb.New(testclock.TestRecentTimeUTC),
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: gf.CI(i),
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, depCL), should.BeNil)
					deps[i] = &changelist.Dep{Clid: int64(depCL.ID)}
				}
				reason.Kind = &changelist.CLError_DepRunFailed{
					DepRunFailed: 2,
				}
				assert.Loosely(t, mustFormat(), should.ContainSubstring(text.Doc(`
					a Run failed on [CL](https://x-review.googlesource.com/c/2) that this CL depends on.
				`)))
			})
		})
	})
}
