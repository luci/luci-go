// Copyright 2026 The LUCI Authors.
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

package buganizer

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// TestBugStateMachine tests the bug management state machine across all
// bug lifecycle states:
//   - Open (NEW, ASSIGNED, ACCEPTED)
//   - Verified (VERIFIED)
//   - Fixed (FIXED by user, awaiting verification)
//   - ClosedOther (INTENDED_BEHAVIOR, NOT_REPRODUCIBLE, INFEASIBLE, OBSOLETE)
func TestBugStateMachine(t *testing.T) {
	t.Parallel()

	ftt.Run("Bug State Machine", t, func(t *ftt.Test) {
		ctx := context.Background()
		fakeClient := NewFakeClient()
		fakeStore := fakeClient.FakeStore
		buganizerCfg := ChromeOSTestConfig()

		policyA := config.CreatePlaceholderBugManagementPolicy("policy-a")
		policyA.HumanReadableName = "Problem A"
		policyA.Priority = configpb.BuganizerPriority_P4
		policyA.BugTemplate.Buganizer.Hotlists = []int64{1001}

		policyB := config.CreatePlaceholderBugManagementPolicy("policy-b")
		policyB.HumanReadableName = "Problem B"
		policyB.Priority = configpb.BuganizerPriority_P0
		policyB.BugTemplate.Buganizer.Hotlists = []int64{1002}

		policyC := config.CreatePlaceholderBugManagementPolicy("policy-c")
		policyC.HumanReadableName = "Problem C"
		policyC.Priority = configpb.BuganizerPriority_P1
		policyC.BugTemplate.Buganizer.Hotlists = []int64{1003}

		projectCfg := &configpb.ProjectConfig{
			BugManagement: &configpb.BugManagement{
				DefaultBugSystem: configpb.BugSystem_BUGANIZER,
				Buganizer:        buganizerCfg,
				Policies: []*configpb.BugManagementPolicy{
					policyA,
					policyB,
					policyC,
				},
				BugClosureInvalidationAction: &configpb.BugManagement_FileNewBugs{},
			},
		}

		bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
		assert.Loosely(t, err, should.BeNil)

		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		// Initial bug creation: P1 with policy-a (P4) and policy-c (P1) active.
		createReq := newCreateRequest()
		createReq.BugManagementState = &bugspb.BugManagementState{
			PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
				"policy-a": {IsActive: true}, // P4
				"policy-c": {IsActive: true}, // P1
			},
		}
		createRes := bm.Create(ctx, createReq)
		assert.Loosely(t, createRes.ID, should.Equal("1"))
		assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P1))
		originalCommentCount := len(fakeStore.Issues[1].Comments)

		activationTime := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)
		state := &bugspb.BugManagementState{
			RuleAssociationNotified: true,
			PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
				"policy-a": { // P4
					IsActive:           true,
					LastActivationTime: timestamppb.New(activationTime),
					ActivationNotified: true,
				},
				"policy-b": { // P0
					IsActive:             false,
					LastActivationTime:   timestamppb.New(activationTime.Add(-time.Hour)),
					LastDeactivationTime: timestamppb.New(activationTime),
					ActivationNotified:   false,
				},
				"policy-c": { // P1
					IsActive:           true,
					LastActivationTime: timestamppb.New(activationTime),
					ActivationNotified: true,
				},
			},
		}

		bugsToUpdate := []bugs.BugUpdateRequest{
			{
				Bug:                              bugs.BugID{System: bugs.BuganizerSystem, ID: createRes.ID},
				BugManagementState:               state,
				IsManagingBug:                    true,
				RuleID:                           "rule-id",
				IsManagingBugPriority:            true,
				IsManagingBugPriorityLastUpdated: clock.Now(ctx),
			},
		}

		expectedNoOpResponse := []bugs.BugUpdateResponse{
			{
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
			},
		}

		assertNoOp := func(t testing.TB) {
			t.Helper()
			beforeIssue := proto.Clone(fakeStore.Issues[1].Issue).(*issuetracker.Issue)
			beforeComments := len(fakeStore.Issues[1].Comments)
			res, err := bm.Update(ctx, bugsToUpdate)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			assert.That(t, res, should.Match(expectedNoOpResponse), truth.LineContext())
			assert.That(t, fakeStore.Issues[1].Issue, should.Match(beforeIssue), truth.LineContext())
			assert.Loosely(t, len(fakeStore.Issues[1].Comments), should.Equal(beforeComments), truth.LineContext())
		}

		// =========================================================================
		// 1. State: Open (NEW / ASSIGNED)
		// =========================================================================
		t.Run("State: Open (NEW / ASSIGNED)", func(t *ftt.Test) {
			t.Run("Steady state: policies unchanged -> no-op", func(t *ftt.Test) {
				assertNoOp(t)
			})

			t.Run("Priority increase: higher-priority policy activates (P1 -> P0)", func(t *ftt.Test) {
				state.PolicyState["policy-b"].IsActive = true
				state.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))
				state.PolicyState["policy-b"].ActivationNotified = true

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because the following problem(s) have started:\n"+
						"- Problem B (P0)\n"+
						"The bug priority has been increased from P1 to P0."))
			})

			t.Run("Priority decrease: higher-priority policy deactivates (P1 -> P4)", func(t *ftt.Test) {
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P4))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because the following problem(s) have stopped:\n"+
						"- Problem C (P1)\n"+
						"The bug priority has been decreased from P1 to P4."))
			})

			t.Run("Priority manually set by user -> does not update priority and disables rule priority updates", func(t *ftt.Test) {
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				fakeStore.Issues[1].Issue.IssueState.Priority = issuetracker.Issue_P2
				fakeStore.Issues[1].IssueUpdates = append(fakeStore.Issues[1].IssueUpdates, &issuetracker.IssueUpdate{
					Author:    &issuetracker.User{EmailAddress: "user@google.com"},
					Timestamp: timestamppb.New(tc.Now().Add(time.Minute)),
					FieldUpdates: []*issuetracker.FieldUpdate{
						{Field: "priority"},
					},
				})

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res[0].DisableRulePriorityUpdates, should.BeTrue)
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P2))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"The bug priority has been manually set."))
			})

			t.Run("All policies deactivate -> transitions Open to VERIFIED", func(t *ftt.Test) {
				state.PolicyState["policy-a"].IsActive = false
				state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Verifier.EmailAddress, should.Equal("email@test.com"))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because the following problem(s) have stopped:\n"+
						"- Problem C (P1)\n"+
						"- Problem A (P4)\n"+
						"The bug has been verified."))
			})
		})

		// =========================================================================
		// 2. State: Verified (VERIFIED)
		// =========================================================================
		t.Run("State: Verified (VERIFIED)", func(t *ftt.Test) {
			fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_VERIFIED
			fakeStore.Issues[1].Issue.VerifiedTime = timestamppb.New(tc.Now())
			state.PolicyState["policy-a"].IsActive = false
			state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
			state.PolicyState["policy-c"].IsActive = false
			state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

			t.Run("Policies remain inactive (< 30 days) -> stays VERIFIED, does not archive", func(t *ftt.Test) {
				tc.Add(29 * 24 * time.Hour)
				assertNoOp(t)
			})

			t.Run("Policies remain inactive (>= 30 days) -> archives rule", func(t *ftt.Test) {
				tc.Add(30 * 24 * time.Hour)
				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match([]bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}))
			})

			t.Run("Policy re-activates -> re-opens issue and sets priority", func(t *ftt.Test) {
				fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{EmailAddress: "owner@google.com"}
				state.PolicyState["policy-b"].IsActive = true
				state.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))
				state.PolicyState["policy-b"].ActivationNotified = true

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_ASSIGNED))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because the following problem(s) have started:\n"+
						"- Problem B (P0)\n"+
						"The bug has been re-opened as P0."))
			})
		})

		// =========================================================================
		// 3. State: Fixed (FIXED by user, awaiting verification)
		// =========================================================================
		t.Run("State: Fixed (FIXED by user)", func(t *ftt.Test) {
			fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
			fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{EmailAddress: "owner@google.com"}
			fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(tc.Now())

			t.Run("Policies remain active (< 1 day since resolved) -> stays FIXED, no priority update or re-open", func(t *ftt.Test) {
				tc.Add(12 * time.Hour)
				assertNoOp(t)
			})

			t.Run("Higher-priority policy deactivates while lower policy remains active (P1 -> P4) -> stays FIXED at P1 without priority churn", func(t *ftt.Test) {
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				assertNoOp(t)
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_FIXED))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P1))
			})

			t.Run("Policies remain active (>= 30 days since resolved) -> does not archive while policies active", func(t *ftt.Test) {
				tc.Add(30 * 24 * time.Hour)
				assertNoOp(t)
			})

			t.Run("Policies remain active (>= 1 day since resolved, using testclock) -> invalidates bug closure", func(t *ftt.Test) {
				t.Skip("TODO(b/564445116): Enable once manager.go uses clock.Now(ctx) instead of time.Since")

				tc.Add(25 * time.Hour)
				bugsToUpdate[0].InvalidationStatus = bugs.BugClosureInvalidationStatus{
					OneDay: bugs.BugClosureInvalidationResult{
						IsInvalidated: true,
						ActivePolicyIDs: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					},
				}

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match([]bugs.BugUpdateResponse{
					{
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
						BugClosureValidationResult: bugs.BugClosureInvalidationResult{
							IsInvalidated: true,
							ActivePolicyIDs: map[bugs.PolicyID]struct{}{
								"policy-a": {},
							},
						},
					},
				}))
				// Issue itself remains FIXED in BugManager (BugUpdater files the new bug).
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_FIXED))
			})

			t.Run("All policies deactivate (single step) -> transitions FIXED to VERIFIED with delta explanation", func(t *ftt.Test) {
				t.Skip("TODO(b/564445116): Enable once FIXED -> VERIFIED transition is restored")

				state.PolicyState["policy-a"].IsActive = false
				state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Verifier.EmailAddress, should.Equal("email@test.com"))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P1))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because the following problem(s) have stopped:\n"+
						"- Problem C (P1)\n"+
						"- Problem A (P4)\n"+
						"The bug has been verified."))
			})

			t.Run("All policies deactivate across multiple steps while FIXED (P1 stops day 1, P4 stops day 3) -> transitions FIXED to VERIFIED with 'all problems have stopped' explanation", func(t *ftt.Test) {
				t.Skip("TODO(b/564445116): Enable once FIXED -> VERIFIED transition is restored")

				// Day 1: policy-c (P1) deactivated while the bug was FIXED (so bug priority stayed P1).
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(24 * time.Hour))
				assertNoOp(t)

				// Day 3: policy-a (P4) also deactivates. Because the intermediate P1->P4 priority
				// change was suppressed while FIXED, the comment should explain that all problems
				// have stopped rather than claiming only Problem A (P4) stopped.
				state.PolicyState["policy-a"].IsActive = false
				state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(72 * time.Hour))

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match(expectedNoOpResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Verifier.EmailAddress, should.Equal("email@test.com"))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"Because all problems have stopped, the bug has been verified."))
			})

			t.Run("User manually changed priority and marked FIXED, then all policies deactivate -> transitions FIXED to VERIFIED without manual-priority warning", func(t *ftt.Test) {
				t.Skip("TODO(b/564445116): Enable once FIXED -> VERIFIED transition ignores manual priority updates")

				fakeStore.Issues[1].Issue.IssueState.Priority = issuetracker.Issue_P2
				fakeStore.Issues[1].IssueUpdates = append(fakeStore.Issues[1].IssueUpdates, &issuetracker.IssueUpdate{
					Author:    &issuetracker.User{EmailAddress: "owner@google.com"},
					Timestamp: timestamppb.New(tc.Now().Add(time.Minute)),
					FieldUpdates: []*issuetracker.FieldUpdate{
						{Field: "priority"},
					},
				})

				state.PolicyState["policy-a"].IsActive = false
				state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				// Should NOT disable rule priority updates or complain about manual priority when verifying a FIXED bug.
				assert.Loosely(t, res[0].DisableRulePriorityUpdates, should.BeFalse)
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.NotContainSubstring(
					"The bug priority has been manually set."))
			})

			t.Run("Resolved >= 30 days ago with all policies already inactive (backlog cleanup) -> silently archives rule without commenting on bug", func(t *ftt.Test) {
				t.Skip("TODO(b/564445116): Enable once shouldArchiveRule archives stale FIXED bugs with no active policies")

				state.PolicyState["policy-a"].IsActive = false
				state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
				state.PolicyState["policy-c"].IsActive = false
				state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				tc.Add(30 * 24 * time.Hour)

				res, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res, should.Match([]bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}))
				// Should not modify or comment on the old FIXED bug.
				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_FIXED))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount))
			})
		})

		// =========================================================================
		// 4. State: ClosedOther (INTENDED_BEHAVIOR, NOT_REPRODUCIBLE, INFEASIBLE, OBSOLETE)
		// =========================================================================
		t.Run("State: ClosedOther (INTENDED_BEHAVIOR / WontFix / Obsolete)", func(t *ftt.Test) {
			closedOtherStatuses := []issuetracker.Issue_Status{
				issuetracker.Issue_INTENDED_BEHAVIOR,
				issuetracker.Issue_NOT_REPRODUCIBLE,
				issuetracker.Issue_INFEASIBLE,
				issuetracker.Issue_OBSOLETE,
			}

			for _, closedStatus := range closedOtherStatuses {
				t.Run(closedStatus.String(), func(t *ftt.Test) {
					fakeStore.Issues[1].Issue.IssueState.Status = closedStatus
					fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(tc.Now())

					t.Run("Policies active and priority changes -> does not update priority or comment", func(t *ftt.Test) {
						t.Skip("TODO(b/564445116): Enable once ClosedOther statuses are excluded from priority updates")

						state.PolicyState["policy-b"].IsActive = true
						state.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))
						state.PolicyState["policy-b"].ActivationNotified = true

						assertNoOp(t)
					})

					t.Run("All policies deactivate -> does not overwrite status with VERIFIED", func(t *ftt.Test) {
						t.Skip("TODO(b/564445116): Enable once ClosedOther statuses are excluded from VERIFIED transition")

						state.PolicyState["policy-a"].IsActive = false
						state.PolicyState["policy-a"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
						state.PolicyState["policy-c"].IsActive = false
						state.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

						assertNoOp(t)
						assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(closedStatus))
					})

					t.Run("Resolved >= 30 days ago -> archives rule even when IsManagingBug is true", func(t *ftt.Test) {
						t.Skip("TODO(b/564445116): Enable once shouldArchiveRule archives ClosedOther bugs after 30 days")

						tc.Add(30 * 24 * time.Hour)

						res, err := bm.Update(ctx, bugsToUpdate)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, res, should.Match([]bugs.BugUpdateResponse{
							{
								ShouldArchive:             true,
								PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
							},
						}))
					})
				})
			}
		})
	})
}
