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

package acls

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/auth/identity"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestCheckRunCLs(t *testing.T) {
	t.Parallel()

	const (
		lProject   = "chromium"
		gerritHost = "chromium-review.googlesource.com"
		committers = "committer-group"
		dryRunners = "dry-runner-group"
		npRunners  = "new-patchset-runner-group"
	)

	ftt.Run("CheckRunCreate", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		cg := prjcfg.ConfigGroup{
			Content: &cfgpb.ConfigGroup{
				Verifiers: &cfgpb.Verifiers{
					GerritCqAbility: &cfgpb.Verifiers_GerritCQAbility{
						CommitterList:            []string{committers},
						DryRunAccessList:         []string{dryRunners},
						NewPatchsetRunAccessList: []string{npRunners},
					},
				},
			},
		}

		authState := &authtest.FakeState{FakeDB: authtest.NewFakeDB()}
		ctx = auth.WithState(ctx, authState)
		addMember := func(email, grp string) {
			id, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
			assert.NoErr(t, err)
			authState.FakeDB.(*authtest.FakeDB).AddMocks(authtest.MockMembership(id, grp))
		}
		addCommitter := func(email string) {
			addMember(email, committers)
		}
		addDryRunner := func(email string) {
			addMember(email, dryRunners)
		}
		addNPRunner := func(email string) {
			addMember(email, npRunners)
		}

		// test helpers
		var cls []*changelist.CL
		var trs []*run.Trigger
		var clid int64
		addCL := func(triggerer, owner string, m run.Mode) (*changelist.CL, *run.Trigger) {
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: owner},
			})

			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: triggerer},
			})

			clid++
			cl := &changelist.CL{
				ID:         common.CLID(clid),
				ExternalID: changelist.MustGobID(gerritHost, clid),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gerritHost,
							Info: &gerritpb.ChangeInfo{
								Owner: &gerritpb.AccountInfo{
									Email: owner,
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
			cls = append(cls, cl)
			tr := &run.Trigger{
				Email: triggerer,
				Mode:  string(m),
			}
			trs = append(trs, tr)
			return cl, tr
		}
		addDep := func(base *changelist.CL, owner string) *changelist.CL {
			ct.GFake.AddLinkedAccountMapping([]*gerritpb.EmailInfo{
				{Email: owner},
			})

			clid++
			dep := &changelist.CL{
				ID:         common.CLID(clid),
				ExternalID: changelist.MustGobID(gerritHost, clid),
				Snapshot: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gerritHost,
							Info: &gerritpb.ChangeInfo{
								Owner: &gerritpb.AccountInfo{
									Email: owner,
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, dep), should.BeNil)
			base.Snapshot.Deps = append(base.Snapshot.Deps, &changelist.Dep{Clid: clid})
			return dep
		}

		mustOK := func() {
			res, err := CheckRunCreate(ctx, ct.GFake, &cg, trs, cls)
			assert.NoErr(t, err)
			assert.Loosely(t, res.FailuresSummary(), should.BeEmpty)
			assert.Loosely(t, res.OK(), should.BeTrue)
		}
		mustFailWith := func(cl *changelist.CL, format string, args ...any) CheckResult {
			res, err := CheckRunCreate(ctx, ct.GFake, &cg, trs, cls)
			assert.NoErr(t, err)
			assert.Loosely(t, res.OK(), should.BeFalse)
			assert.Loosely(t, res.Failure(cl), should.ContainSubstring(fmt.Sprintf(format, args...)))
			return res
		}
		markCLSubmittable := func(cl *changelist.CL) {
			cl.Snapshot.GetGerrit().GetInfo().Submittable = true
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
		}
		submitCL := func(cl *changelist.CL) {
			cl.Snapshot.GetGerrit().GetInfo().Status = gerritpb.ChangeStatus_MERGED
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
		}
		setAllowOwner := func(action cfgpb.Verifiers_GerritCQAbility_CQAction) {
			cg.Content.Verifiers.GerritCqAbility.AllowOwnerIfSubmittable = action
		}
		setTrustDryRunnerDeps := func(trust bool) {
			cg.Content.Verifiers.GerritCqAbility.TrustDryRunnerDeps = trust
		}
		setAllowNonOwnerDryRunner := func(allow bool) {
			cg.Content.Verifiers.GerritCqAbility.AllowNonOwnerDryRunner = allow
		}

		addSubmitReq := func(cl *changelist.CL, name string, st gerritpb.SubmitRequirementResultInfo_Status) {
			ci := cl.Snapshot.Kind.(*changelist.Snapshot_Gerrit).Gerrit.Info
			ci.SubmitRequirements = append(ci.SubmitRequirements,
				&gerritpb.SubmitRequirementResultInfo{Name: name, Status: st})
			assert.Loosely(t, datastore.Put(ctx, cl), should.BeNil)
		}
		satisfiedReq := func(cl *changelist.CL, name string) {
			addSubmitReq(cl, name, gerritpb.SubmitRequirementResultInfo_SATISFIED)
		}
		unsatisfiedReq := func(cl *changelist.CL, name string) {
			addSubmitReq(cl, name, gerritpb.SubmitRequirementResultInfo_UNSATISFIED)
		}
		naReq := func(cl *changelist.CL, name string) {
			addSubmitReq(cl, name, gerritpb.SubmitRequirementResultInfo_NOT_APPLICABLE)
		}

		t.Run("mode == FullRun", func(t *ftt.Test) {
			m := run.FullRun

			t.Run("triggerer == owner", func(t *ftt.Test) {
				tr, owner := "t@example.org", "t@example.org"
				cl, _ := addCL(tr, owner, m)

				t.Run("triggerer is a committer", func(t *ftt.Test) {
					addCommitter(tr)

					// Should succeed when CL becomes submittable.
					mustFailWith(cl, notSubmittable)
					markCLSubmittable(cl)
					mustOK()
				})
				t.Run("triggerer is a dry-runner", func(t *ftt.Test) {
					addDryRunner(tr)

					// Dry-runner can trigger a full-run for own submittable CL.
					unsatisfiedReq(cl, "Code-Review")

					mustFailWith(cl, "CV cannot start a Run because this CL is not satisfying the `Code-Review` submit requirement. Please hover over the corresponding entry in the Submit Requirements section to check what is missing.")
					markCLSubmittable(cl)
					mustOK()
				})
				t.Run("triggerer is neither dry-runner nor committer", func(t *ftt.Test) {
					t.Run("CL submittable", func(t *ftt.Test) {
						// Should fail, even if it was submittable.
						markCLSubmittable(cl)
						mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a committer", tr)
						// unless AllowOwnerIfSubmittable == COMMIT
						setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
						mustOK()
					})
					t.Run("CL not submittable", func(t *ftt.Test) {
						// Should fail always.
						mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a committer", tr)
						setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
						mustFailWith(cl, notSubmittable)
					})
				})
				t.Run("suspiciously not submittable", func(t *ftt.Test) {
					addDryRunner(tr)
					addSubmitReq(cl, "Code-Review", gerritpb.SubmitRequirementResultInfo_SATISFIED)
					mustFailWith(cl, notSubmittableSuspicious)
				})
			})

			t.Run("triggerer != owner", func(t *ftt.Test) {
				tr, owner := "t@example.org", "o@example.org"
				cl, _ := addCL(tr, owner, m)

				t.Run("triggerer is a committer", func(t *ftt.Test) {
					addCommitter(tr)

					// Should succeed when CL becomes submittable.
					mustFailWith(cl, notSubmittable)
					markCLSubmittable(cl)
					mustOK()
				})
				t.Run("triggerer is a dry-runner", func(t *ftt.Test) {
					addDryRunner(tr)

					// Dry-runner cannot trigger a full-run for someone else' CL,
					// whether it is submittable or not
					mustFailWith(cl, "neither the CL owner nor a committer")
					markCLSubmittable(cl)
					mustFailWith(cl, "neither the CL owner nor a committer")

					// AllowOwnerIfSubmittable doesn't change the decision, either.
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
					mustFailWith(cl, "neither the CL owner nor a committer")

					// TrustDryRunnerDeps doesn't change the decision, either.
					setTrustDryRunnerDeps(true)
					mustFailWith(cl, "neither the CL owner nor a committer")

					// AllowNonOwnerDryRunner doesn't change the decision, either.
					setAllowNonOwnerDryRunner(true)
					mustFailWith(cl, "neither the CL owner nor a committer")
				})
				t.Run("triggerer is neither dry-runner nor committer", func(t *ftt.Test) {
					// Should fail always.
					mustFailWith(cl, "neither the CL owner nor a committer")
					markCLSubmittable(cl)
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
					mustFailWith(cl, "neither the CL owner nor a committer")
				})
				t.Run("suspiciously not submittable", func(t *ftt.Test) {
					addCommitter(tr)
					addSubmitReq(cl, "Code-Review", gerritpb.SubmitRequirementResultInfo_SATISFIED)
					mustFailWith(cl, notSubmittableSuspicious)
				})
			})
		})

		t.Run("mode == DryRun", func(t *ftt.Test) {
			m := run.DryRun

			t.Run("triggerer == owner", func(t *ftt.Test) {
				tr, owner := "t@example.org", "t@example.org"
				cl, _ := addCL(tr, owner, m)

				t.Run("triggerer is a committer", func(t *ftt.Test) {
					// Committers can trigger a dry-run for someone else' CL
					// even if the CL is not submittable
					addCommitter(tr)
					mustOK()
				})
				t.Run("triggerer is a dry-runner", func(t *ftt.Test) {
					// Should succeed even if the CL is not submittable.
					addDryRunner(tr)
					mustOK()
				})
				t.Run("triggerer is neither dry-runner nor committer", func(t *ftt.Test) {
					t.Run("CL submittable", func(t *ftt.Test) {
						// Should fail, even if the CL is submittable.
						markCLSubmittable(cl)
						mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a dry-runner", owner)
						// Unless AllowOwnerIfSubmittable == DRY_RUN
						setAllowOwner(cfgpb.Verifiers_GerritCQAbility_DRY_RUN)
						mustOK()
						// Or, COMMIT
						setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
						mustOK()
					})
					t.Run("CL not submittable", func(t *ftt.Test) {
						// Should fail always.
						mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a dry-runner", owner)
						setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
						mustFailWith(cl, notSubmittable)
					})
				})
			})

			t.Run("triggerer != owner", func(t *ftt.Test) {
				tr, owner := "t@example.org", "o@example.org"
				cl, _ := addCL(tr, owner, m)

				t.Run("triggerer is a committer", func(t *ftt.Test) {
					// Should succeed whether CL is submittable or not.
					addCommitter(tr)
					mustOK()
					markCLSubmittable(cl)
					mustOK()
				})
				t.Run("triggerer is a dry-runner", func(t *ftt.Test) {
					// Only committers can trigger a dry-run for someone else's CL.
					addDryRunner(tr)
					mustFailWith(cl, "neither the CL owner nor a committer")
					markCLSubmittable(cl)
					mustFailWith(cl, "neither the CL owner nor a committer")
					// AllowOwnerIfSubmittable doesn't change the decision, either.
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
					mustFailWith(cl, "neither the CL owner nor a committer")
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_DRY_RUN)
					mustFailWith(cl, "neither the CL owner nor a committer")
					// TrustDryRunnerDeps doesn't change the decision, either.
					setTrustDryRunnerDeps(true)
					mustFailWith(cl, "neither the CL owner nor a committer")
				})
				t.Run("triggerer is a dry-runner (with allow_non_owner_dry_runner)", func(t *ftt.Test) {
					// With allow_non_owner_dry_runner, dry-runners can trigger a dry-run for someone else's CL.
					addDryRunner(tr)
					setAllowNonOwnerDryRunner(true)
					mustOK()
					markCLSubmittable(cl)
					mustOK()
				})
				t.Run("triggerer is neither dry-runner nor committer", func(t *ftt.Test) {
					// Only committers can trigger a dry-run for someone else' CL.
					mustFailWith(cl, "neither the CL owner nor a committer")
					markCLSubmittable(cl)
					mustFailWith(cl, "neither the CL owner nor a committer")
					// AllowOwnerIfSubmittable doesn't change the decision, either.
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_COMMIT)
					mustFailWith(cl, "neither the CL owner nor a committer")
					setAllowOwner(cfgpb.Verifiers_GerritCQAbility_DRY_RUN)
					mustFailWith(cl, "neither the CL owner nor a committer")
				})
			})

			t.Run("w/ dependencies", func(t *ftt.Test) {
				// if triggerer is not the owner, but a
				// committer/dry-runner, then untrusted deps
				// should be checked.
				tr, owner := "t@example.org", "o@example.org"
				cl, _ := addCL(tr, owner, m)

				dep1 := addDep(cl, "dep_owner1@example.org")
				dep2 := addDep(cl, "dep_owner2@example.org")
				dep1URL := dep1.ExternalID.MustURL()
				dep2URL := dep2.ExternalID.MustURL()

				testCases := func() {
					t.Run("untrusted", func(t *ftt.Test) {
						res := mustFailWith(cl, untrustedDeps)
						assert.Loosely(t, res.Failure(cl), should.ContainSubstring(dep1URL))
						assert.Loosely(t, res.Failure(cl), should.ContainSubstring(dep2URL))
						// if the deps have no submit requirements, the rejection message
						// shouldn't contain a warning for suspicious CLs.
						assert.Loosely(t, res.Failure(cl), should.NotContainSubstring(untrustedDepsSuspicious))

						t.Run("with TrustDryRunnerDeps", func(t *ftt.Test) {
							setTrustDryRunnerDeps(true)
							mustFailWith(cl, untrustedDepsTrustDryRunnerDeps)
						})

						t.Run("but dep2 satisfies all the SubmitRequirements", func(t *ftt.Test) {
							naReq(dep1, "Code-Review")
							unsatisfiedReq(dep1, "Code-Owner")
							satisfiedReq(dep2, "Code-Review")
							satisfiedReq(dep2, "Code-Owner")
							res := mustFailWith(cl, untrustedDeps)
							assert.Loosely(t, res.Failure(cl), should.ContainSubstring(fmt.Sprintf(""+
								"- %s: not submittable, although submit requirements `Code-Review` and `Code-Owner` are satisfied",
								dep2URL,
							)))
							assert.Loosely(t, res.Failure(cl), should.ContainSubstring(untrustedDepsSuspicious))
						})

						t.Run("because all the deps have unsatisfied requirements", func(t *ftt.Test) {
							dep3 := addDep(cl, "dep_owner3@example.org")
							dep3URL := dep3.ExternalID.MustURL()

							unsatisfiedReq(dep1, "Code-Review")
							unsatisfiedReq(dep2, "Code-Review")
							unsatisfiedReq(dep2, "Code-Owner")
							unsatisfiedReq(dep3, "Code-Review")
							unsatisfiedReq(dep3, "Code-Owner")
							unsatisfiedReq(dep3, "Code-Quiz")

							res := mustFailWith(cl, untrustedDeps)
							assert.Loosely(t, res.Failure(cl), should.NotContainSubstring(untrustedDepsSuspicious))
							assert.Loosely(t, res.Failure(cl), should.ContainSubstring(
								dep1URL+": not satisfying the `Code-Review` submit requirement"))
							assert.Loosely(t, res.Failure(cl), should.ContainSubstring(
								dep2URL+": not satisfying the `Code-Review` and `Code-Owner` submit requirement"))
							assert.Loosely(t, res.Failure(cl), should.ContainSubstring(
								dep3URL+": not satisfying the `Code-Review`, `Code-Owner`, and `Code-Quiz` submit requirement"))
						})
					})
					t.Run("trusted because it's apart of the Run", func(t *ftt.Test) {
						cls = append(cls, dep1, dep2)
						trs = append(trs, &run.Trigger{Email: tr, Mode: string(m)})
						trs = append(trs, &run.Trigger{Email: tr, Mode: string(m)})
						mustOK()
					})
					t.Run("trusted because of submittable", func(t *ftt.Test) {
						markCLSubmittable(dep1)
						markCLSubmittable(dep2)
						mustOK()
					})
					t.Run("trusterd because they have been merged already", func(t *ftt.Test) {
						submitCL(dep1)
						submitCL(dep2)
						mustOK()
					})
					t.Run("trusted because the owner is a committer", func(t *ftt.Test) {
						addCommitter("dep_owner1@example.org")
						addCommitter("dep_owner2@example.org")
						mustOK()
					})
					t.Run("trusted because the owner is a dry-runner", func(t *ftt.Test) {
						addDryRunner("dep_owner1@example.org")
						addDryRunner("dep_owner2@example.org")

						// Not allowed without TrustDryRunnerDeps.
						res := mustFailWith(cl, untrustedDeps)
						assert.Loosely(t, res.Failure(cl), should.ContainSubstring(dep1URL))
						assert.Loosely(t, res.Failure(cl), should.ContainSubstring(dep2URL))

						setTrustDryRunnerDeps(true)
						mustOK()
					})
					t.Run("a mix of untrusted and trusted deps", func(t *ftt.Test) {
						addCommitter("dep_owner1@example.org")
						res := mustFailWith(cl, untrustedDeps)
						assert.Loosely(t, res.Failure(cl), should.NotContainSubstring(dep1URL))
						assert.Loosely(t, res.Failure(cl), should.ContainSubstring(dep2URL))
					})
				}

				t.Run("committer", func(t *ftt.Test) {
					addCommitter(tr)
					testCases()
				})

				t.Run("dry-runner (with allow_non_owner_dry_runner)", func(t *ftt.Test) {
					addDryRunner(tr)
					setAllowNonOwnerDryRunner(true)
					testCases()
				})
			})
		})

		t.Run("mode == NewPatchsetRun", func(t *ftt.Test) {
			tr, owner := "t@example.org", "t@example.org"
			cl, _ := addCL(tr, owner, run.NewPatchsetRun)
			t.Run("owner is disallowed", func(t *ftt.Test) {
				mustFailWith(cl, "CL owner is not in the allowlist.")
			})
			t.Run("owner is allowed", func(t *ftt.Test) {
				addNPRunner(owner)
				mustOK()
			})
		})

		t.Run("mode is non standard mode", func(t *ftt.Test) {
			tr, owner := "t@example.org", "t@example.org"
			cl, trigger := addCL(tr, owner, "CUSTOM_RUN")
			trigger.ModeDefinition = &cfgpb.Mode{
				Name:            "CUSTOM_RUN",
				TriggeringLabel: "CUSTOM",
				TriggeringValue: 1,
			}
			t.Run("dry", func(t *ftt.Test) {
				trigger.ModeDefinition.CqLabelValue = 1
				t.Run("disallowed", func(t *ftt.Test) {
					mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a dry-runner", owner)
				})
				t.Run("allowed", func(t *ftt.Test) {
					addDryRunner(owner)
					mustOK()
				})
			})
			t.Run("full", func(t *ftt.Test) {
				trigger.ModeDefinition.CqLabelValue = 2
				markCLSubmittable(cl)
				t.Run("disallowed", func(t *ftt.Test) {
					mustFailWith(cl, "CV cannot start a Run for `%s` because the user is not a committer", owner)
				})
				t.Run("allowed", func(t *ftt.Test) {
					addCommitter(owner)
					mustOK()
				})
			})
		})

		t.Run("multiple CLs", func(t *ftt.Test) {
			m := run.DryRun
			tr, owner := "t@example.org", "t@example.org"
			cl1, _ := addCL(tr, owner, m)
			cl2, _ := addCL(tr, owner, m)
			setAllowOwner(cfgpb.Verifiers_GerritCQAbility_DRY_RUN)

			t.Run("all CLs passed", func(t *ftt.Test) {
				markCLSubmittable(cl1)
				markCLSubmittable(cl2)
				mustOK()
			})
			t.Run("all CLs failed", func(t *ftt.Test) {
				mustFailWith(cl1, notSubmittable)
				mustFailWith(cl2, notSubmittable)
			})
			t.Run("Some CLs failed", func(t *ftt.Test) {
				markCLSubmittable(cl1)
				mustFailWith(cl1, "CV cannot start a Run due to errors in the following CL(s)")
				mustFailWith(cl2, notSubmittable)
			})
		})
	})
}
