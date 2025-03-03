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

package triager

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

func TestDepsTriage(t *testing.T) {
	t.Parallel()

	ftt.Run("Component's PCL deps triage", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// Truncate start time point s.t. easy to see diff in test failures.
		epoch := testclock.TestRecentTimeUTC.Truncate(10000 * time.Second)
		dryRun := func(ts time.Time) *run.Triggers {
			return &run.Triggers{CqVoteTrigger: &run.Trigger{Mode: string(run.DryRun), Time: timestamppb.New(ts)}}
		}
		fullRun := func(ts time.Time) *run.Triggers {
			return &run.Triggers{CqVoteTrigger: &run.Trigger{Mode: string(run.FullRun), Time: timestamppb.New(ts)}}
		}

		sup := &simplePMState{
			pb: &prjpb.PState{},
			cgs: []*prjcfg.ConfigGroup{
				{ID: "hash/singular", Content: &cfgpb.ConfigGroup{}},
				{ID: "hash/combinable", Content: &cfgpb.ConfigGroup{CombineCls: &cfgpb.CombineCLs{}}},
				{ID: "hash/another", Content: &cfgpb.ConfigGroup{}},
			},
		}
		const singIdx, combIdx, anotherIdx = 0, 1, 2

		do := func(pcl *prjpb.PCL, cgIdx int32) *triagedDeps {
			backup := prjpb.PState{}
			proto.Merge(&backup, sup.pb)

			// Actual component doesn't matter in this test.
			td := triageDeps(ctx, pcl, cgIdx, pmState{sup})
			assert.That(t, sup.pb, should.Match(&backup)) // must not be modified
			return td
		}

		t.Run("Singluar and Combinable behave the same", func(t *ftt.Test) {
			sameTests := func(name string, cgIdx int32) {
				t.Run(name, func(t *ftt.Test) {
					t.Run("no deps", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}},
						}
						td := do(sup.pb.Pcls[0], cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{}))
						assert.Loosely(t, td.OK(), should.BeTrue)
					})

					t.Run("Valid CL stack CQ+1", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(3 * time.Second))},
							{Clid: 32, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(2 * time.Second))},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						td := do(sup.PCL(33), cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{
							lastCQVoteTriggered: epoch.Add(3 * time.Second),
						}))
						assert.Loosely(t, td.OK(), should.BeTrue)
					})

					t.Run("Not yet loaded deps", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							// 31 isn't in PCLs yet
							{Clid: 32, Status: prjpb.PCL_UNKNOWN},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{notYetLoaded: pcl33.GetDeps()}))
						assert.Loosely(t, td.OK(), should.BeTrue)
					})

					t.Run("Unwatched", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, Status: prjpb.PCL_UNWATCHED},
							{Clid: 32, Status: prjpb.PCL_DELETED},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{
							invalidDeps: &changelist.CLError_InvalidDeps{
								Unwatched: pcl33.GetDeps(),
							},
						}))
						assert.Loosely(t, td.OK(), should.BeFalse)
					})

					t.Run("Submitted can be in any config group and they are OK deps", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 32, ConfigGroupIndexes: []int32{anotherIdx}, Submitted: true},
							{Clid: 33, ConfigGroupIndexes: []int32{cgIdx}, Triggers: dryRun(epoch.Add(1 * time.Second)),
								Deps: []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{submitted: pcl33.GetDeps()}))
						assert.Loosely(t, td.OK(), should.BeTrue)
					})

					t.Run("Wrong config group", func(t *ftt.Test) {
						sup.pb.Pcls = []*prjpb.PCL{
							{Clid: 31, Triggers: dryRun(epoch.Add(3 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx}},
							{Clid: 32, Triggers: dryRun(epoch.Add(2 * time.Second)), ConfigGroupIndexes: []int32{anotherIdx, cgIdx}},
							{Clid: 33, Triggers: dryRun(epoch.Add(1 * time.Second)), ConfigGroupIndexes: []int32{cgIdx},
								Deps: []*changelist.Dep{
									{Clid: 31, Kind: changelist.DepKind_SOFT},
									{Clid: 32, Kind: changelist.DepKind_HARD},
								}},
						}
						pcl33 := sup.PCL(33)
						td := do(pcl33, cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{
							lastCQVoteTriggered: epoch.Add(3 * time.Second),
							invalidDeps: &changelist.CLError_InvalidDeps{
								WrongConfigGroup: pcl33.GetDeps(),
							},
						}))
						assert.Loosely(t, td.OK(), should.BeFalse)
					})

					t.Run("Too many deps", func(t *ftt.Test) {
						// Create maxAllowedDeps+1 deps.
						sup.pb.Pcls = make([]*prjpb.PCL, 0, maxAllowedDeps+2)
						deps := make([]*changelist.Dep, 0, maxAllowedDeps+1)
						for i := 1; i <= maxAllowedDeps+1; i++ {
							sup.pb.Pcls = append(sup.pb.Pcls, &prjpb.PCL{
								Clid:               int64(1000 + i),
								ConfigGroupIndexes: []int32{cgIdx},
								Triggers:           dryRun(epoch.Add(time.Second)),
							})
							deps = append(deps, &changelist.Dep{Clid: int64(1000 + i), Kind: changelist.DepKind_SOFT})
						}
						// Add the PCL with the above deps.
						sup.pb.Pcls = append(sup.pb.Pcls, &prjpb.PCL{
							Clid:               2000,
							ConfigGroupIndexes: []int32{cgIdx},
							Triggers:           dryRun(epoch.Add(time.Second)),
							Deps:               deps,
						})
						td := do(sup.PCL(2000), cgIdx)
						assert.That(t, td, should.Match(&triagedDeps{
							lastCQVoteTriggered: epoch.Add(time.Second),
							invalidDeps: &changelist.CLError_InvalidDeps{
								TooMany: &changelist.CLError_InvalidDeps_TooMany{
									Actual:     maxAllowedDeps + 1,
									MaxAllowed: maxAllowedDeps,
								},
							},
						}))
						assert.Loosely(t, td.OK(), should.BeFalse)
					})
				})
			}
			sameTests("singular", singIdx)
			sameTests("combinable", combIdx)
		})

		t.Run("Singular speciality", func(t *ftt.Test) {
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{singIdx},
					Triggers: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{singIdx},
					Triggers: fullRun(epoch.Add(2 * time.Second)),
					Deps:     []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{singIdx},
					Triggers: dryRun(epoch.Add(3 * time.Second)), // doesn't care about deps.
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			t.Run("dry run doesn't care about deps' triggers", func(t *ftt.Test) {
				pcl33 := sup.PCL(33)
				td := do(pcl33, singIdx)
				assert.That(t, td, should.Match(&triagedDeps{
					lastCQVoteTriggered: epoch.Add(3 * time.Second),
				}))
			})
			t.Run("full run with open-deps", func(t *ftt.Test) {
				pcl32 := sup.PCL(32)

				t.Run("allowed if the deps are hard", func(t *ftt.Test) {
					td := do(pcl32, singIdx)
					assert.Loosely(t, td.OK(), should.BeTrue)
				})

				t.Run("not allowed if the deps are soft", func(t *ftt.Test) {
					// Soft dependency (ie via Cq-Depend) won't be submitted as part a
					// single Submit gerrit RPC, so it can't be allowed.
					pcl32.GetDeps()[0].Kind = changelist.DepKind_SOFT
					td := do(pcl32, singIdx)
					assert.That(t, td, should.Match(&triagedDeps{
						lastCQVoteTriggered: epoch.Add(3 * time.Second),
						invalidDeps: &changelist.CLError_InvalidDeps{
							SingleFullDeps: pcl32.GetDeps(),
						},
					}))
					assert.Loosely(t, td.OK(), should.BeFalse)
				})
			})
		})

		t.Run("Full run with chained CQ votes", func(t *ftt.Test) {
			voter := "test@example.org"
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{singIdx},
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{singIdx},
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_HARD},
					},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{singIdx},
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_HARD},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}

			t.Run("Single vote on the topmost CL", func(t *ftt.Test) {
				pcl33 := sup.PCL(33)
				pcl33.Triggers = fullRun(epoch)
				pcl33.Triggers.CqVoteTrigger.Email = voter
				td := do(pcl33, singIdx)

				// The triage dep result should be OK(), but have
				// the not-yet-voted deps in needToTrigger
				assert.Loosely(t, td.OK(), should.BeTrue)
				assert.That(t, td.needToTrigger, should.Match([]*changelist.Dep{
					{Clid: 31, Kind: changelist.DepKind_HARD},
					{Clid: 32, Kind: changelist.DepKind_HARD},
				}))
			})
			t.Run("a dep already has CQ+2", func(t *ftt.Test) {
				pcl31 := sup.PCL(31)
				pcl31.Triggers = fullRun(epoch)
				pcl31.Triggers.CqVoteTrigger.Email = voter
				pcl33 := sup.PCL(33)
				pcl33.Triggers = fullRun(epoch)
				pcl33.Triggers.CqVoteTrigger.Email = voter
				td := do(pcl33, singIdx)

				assert.Loosely(t, td.OK(), should.BeTrue)
				assert.That(t, td.needToTrigger, should.Match([]*changelist.Dep{
					{Clid: 32, Kind: changelist.DepKind_HARD},
				}))
			})
			t.Run("a dep has CQ+1", func(t *ftt.Test) {
				pcl31 := sup.PCL(31)
				pcl31.Triggers = dryRun(epoch)
				pcl31.Triggers.CqVoteTrigger.Email = voter
				pcl33 := sup.PCL(33)
				pcl33.Triggers = fullRun(epoch)
				pcl33.Triggers.CqVoteTrigger.Email = voter

				// triageDep should still put the dep with CQ+1 in
				// needToTrigger, so that PM will schedule a TQ task to override
				// the CQ vote with CQ+2.
				td := do(pcl33, singIdx)
				assert.Loosely(t, td.OK(), should.BeTrue)
				assert.That(t, td.needToTrigger, should.Match([]*changelist.Dep{
					{Clid: 31, Kind: changelist.DepKind_HARD},
					{Clid: 32, Kind: changelist.DepKind_HARD},
				}))
			})
		})
		t.Run("Combinable speciality", func(t *ftt.Test) {
			// Setup valid deps; sub-tests wll mutate this to become invalid.
			sup.pb.Pcls = []*prjpb.PCL{
				{
					Clid: 31, ConfigGroupIndexes: []int32{combIdx},
					Triggers: dryRun(epoch.Add(3 * time.Second)),
				},
				{
					Clid: 32, ConfigGroupIndexes: []int32{combIdx},
					Triggers: dryRun(epoch.Add(2 * time.Second)),
					Deps:     []*changelist.Dep{{Clid: 31, Kind: changelist.DepKind_HARD}},
				},
				{
					Clid: 33, ConfigGroupIndexes: []int32{combIdx},
					Triggers: dryRun(epoch.Add(1 * time.Second)),
					Deps: []*changelist.Dep{
						{Clid: 31, Kind: changelist.DepKind_SOFT},
						{Clid: 32, Kind: changelist.DepKind_HARD},
					},
				},
			}
			t.Run("dry run expects all deps to be dry", func(t *ftt.Test) {
				pcl32 := sup.PCL(32)
				t.Run("ok", func(t *ftt.Test) {
					td := do(pcl32, combIdx)
					assert.That(t, td, should.Match(&triagedDeps{lastCQVoteTriggered: epoch.Add(3 * time.Second)}))
				})

				t.Run("... not full runs", func(t *ftt.Test) {
					// TODO(tandrii): this can and should be supported.
					sup.PCL(31).Triggers.CqVoteTrigger.Mode = string(run.FullRun)
					td := do(pcl32, combIdx)
					assert.That(t, td, should.Match(&triagedDeps{
						lastCQVoteTriggered: epoch.Add(3 * time.Second),
						invalidDeps: &changelist.CLError_InvalidDeps{
							CombinableMismatchedMode: pcl32.GetDeps(),
						},
					}))
				})
			})
			t.Run("full run considers any dep incompatible", func(t *ftt.Test) {
				pcl33 := sup.PCL(33)
				t.Run("ok", func(t *ftt.Test) {
					for _, pcl := range sup.pb.GetPcls() {
						pcl.Triggers.CqVoteTrigger.Mode = string(run.FullRun)
					}
					td := do(pcl33, combIdx)
					assert.That(t, td, should.Match(&triagedDeps{lastCQVoteTriggered: epoch.Add(3 * time.Second)}))
				})
				t.Run("... not dry runs", func(t *ftt.Test) {
					sup.PCL(32).Triggers.CqVoteTrigger.Mode = string(run.FullRun)
					td := do(pcl33, combIdx)
					assert.That(t, td, should.Match(&triagedDeps{
						lastCQVoteTriggered: epoch.Add(3 * time.Second),
						invalidDeps: &changelist.CLError_InvalidDeps{
							CombinableMismatchedMode: []*changelist.Dep{{Clid: 32, Kind: changelist.DepKind_HARD}},
						},
					}))
					assert.Loosely(t, td.OK(), should.BeFalse)
				})
			})
		})

		t.Run("iterateNotSubmitted works", func(t *ftt.Test) {
			d1 := &changelist.Dep{Clid: 1}
			d2 := &changelist.Dep{Clid: 2}
			d3 := &changelist.Dep{Clid: 3}
			pcl := &prjpb.PCL{}
			td := &triagedDeps{}

			iterate := func() (out []*changelist.Dep) {
				td.iterateNotSubmitted(pcl, func(dep *changelist.Dep) { out = append(out, dep) })
				return
			}

			t.Run("no deps", func(t *ftt.Test) {
				assert.Loosely(t, iterate(), should.BeEmpty)
			})
			t.Run("only submitted", func(t *ftt.Test) {
				td.submitted = []*changelist.Dep{d3, d1, d2}
				pcl.Deps = []*changelist.Dep{d3, d1, d2} // order must be the same
				assert.Loosely(t, iterate(), should.BeEmpty)
			})
			t.Run("some submitted", func(t *ftt.Test) {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				td.submitted = []*changelist.Dep{d3}
				assert.That(t, iterate(), should.Match([]*changelist.Dep{d1, d2}))
				td.submitted = []*changelist.Dep{d1}
				assert.That(t, iterate(), should.Match([]*changelist.Dep{d3, d2}))
				td.submitted = []*changelist.Dep{d2}
				assert.That(t, iterate(), should.Match([]*changelist.Dep{d3, d1}))
			})
			t.Run("none submitted", func(t *ftt.Test) {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				assert.That(t, iterate(), should.Match([]*changelist.Dep{d3, d1, d2}))
			})
			t.Run("notYetLoaded deps are iterated over, too", func(t *ftt.Test) {
				pcl.Deps = []*changelist.Dep{d3, d1, d2}
				td.notYetLoaded = []*changelist.Dep{d3}
				td.submitted = []*changelist.Dep{d2}
				assert.That(t, iterate(), should.Match([]*changelist.Dep{d3, d1}))
			})
			t.Run("panic on invalid usage", func(t *ftt.Test) {
				t.Run("wrong PCL", func(t *ftt.Test) {
					pcl.Deps = []*changelist.Dep{d3, d1, d2}
					td.submitted = []*changelist.Dep{d1, d2, d3} // wrong order
					assert.Loosely(t, func() { iterate() }, should.PanicLike("(wrong PCL?)"))
				})
			})
		})
	})
}
