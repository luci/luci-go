// Copyright 2023 The LUCI Authors.
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
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestBugManager(t *testing.T) {
	t.Parallel()

	ftt.Run("With Bug Manager", t, func(t *ftt.Test) {
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

		t.Run("Create", func(t *ftt.Test) {
			createRequest := newCreateRequest()
			createRequest.BugManagementState = &bugspb.BugManagementState{
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": {
						IsActive: true,
					}, // P4
				},
			}
			expectedIssue := &issuetracker.Issue{
				IssueId: 1,
				IssueState: &issuetracker.IssueState{
					ComponentId: buganizerCfg.DefaultComponent.Id,
					Type:        issuetracker.Issue_BUG,
					Status:      issuetracker.Issue_NEW,
					Severity:    issuetracker.Issue_S2,
					Priority:    issuetracker.Issue_P4,
					Title:       "Tests are failing: Expected equality of these values: \"Expected_Value\" my_expr.evaluate(123) Which is: \"Unexpected_Value\"",
					Ccs: []*issuetracker.User{
						{
							EmailAddress: "testcc1@google.com",
						},
						{
							EmailAddress: "testcc2@google.com",
						},
					},
					HotlistIds: []int64{1001},
					AccessLimit: &issuetracker.IssueAccessLimit{
						AccessLevel: issuetracker.IssueAccessLimit_LIMIT_NONE,
					},
				},
				CreatedTime:  timestamppb.New(clock.Now(ctx)),
				ModifiedTime: timestamppb.New(clock.Now(ctx)),
			}

			t.Run("With reason-based failure cluster", func(t *ftt.Test) {
				reason := `Expected equality of these values:
					"Expected_Value"
					my_expr.evaluate(123)
						Which is: "Unexpected_Value"`
				createRequest.Description.Title = reason
				createRequest.Description.Description = "A cluster of failures has been found with reason: " + reason

				expectedIssue.Description = &issuetracker.IssueComment{
					CommentNumber: 1,
					Comment: "A cluster of failures has been found with reason: Expected equality " +
						"of these values:\n\t\t\t\t\t\"Expected_Value\"\n\t\t\t\t\tmy_expr.evaluate(123)\n\t\t\t\t\t\t" +
						"Which is: \"Unexpected_Value\"\n" +
						"\n" +
						"These test failures are causing problem(s) which require your attention, including:\n" +
						"- Problem A\n" +
						"\n" +
						"See current problems, failure examples and more in LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id\n" +
						"\n" +
						"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
						"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
						"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection",
				}

				t.Run("Base case", func(t *ftt.Test) {
					response := bm.Create(ctx, createRequest)
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))

					issueData := fakeStore.Issues[1]
					assert.Loosely(t, issueData.Issue, should.Match(expectedIssue))
				})

				t.Run("Policy with comment template", func(t *ftt.Test) {
					policyA.BugTemplate.CommentTemplate = "RuleURL:{{.RuleURL}},BugID:{{if .BugID.IsBuganizer}}{{.BugID.BuganizerBugID}}{{end}}"

					bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
					assert.Loosely(t, err, should.BeNil)

					response := bm.Create(ctx, createRequest)
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))

					issueData := fakeStore.Issues[1]
					assert.Loosely(t, issueData.Issue, should.Match(expectedIssue))
					// Expect no comment for policy-a's activation, just the initial issue description.
					assert.Loosely(t, len(issueData.Comments), should.Equal(2))
					assert.Loosely(t, issueData.Comments[1].Comment, should.Equal("RuleURL:https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id,BugID:1\n\n"+
						"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: policy-a)"))
				})
				t.Run("Policy has no comment template", func(t *ftt.Test) {
					policyA.BugTemplate.CommentTemplate = ""

					bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
					assert.Loosely(t, err, should.BeNil)

					response := bm.Create(ctx, createRequest)
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))

					issueData := fakeStore.Issues[1]
					assert.Loosely(t, issueData.Issue, should.Match(expectedIssue))
					// Expect no comment for policy-a's activation, just the initial issue description.
					assert.Loosely(t, len(issueData.Comments), should.Equal(1))
				})
				t.Run("Multiple policies activated", func(t *ftt.Test) {
					createRequest.BugManagementState = &bugspb.BugManagementState{
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"policy-a": {
								IsActive: true,
							}, // P4
							"policy-b": {
								IsActive: true,
							}, // P0
							"policy-c": {
								IsActive: true,
							}, // P1
						},
					}
					expectedIssue.Description.Comment = strings.Replace(expectedIssue.Description.Comment, "- Problem A\n", "- Problem B\n- Problem C\n- Problem A\n", 1)
					expectedIssue.IssueState.Priority = issuetracker.Issue_P0
					expectedIssue.IssueState.HotlistIds = []int64{1001, 1002, 1003}

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
							"policy-b": {},
							"policy-c": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))

					issueData := fakeStore.Issues[1]
					assert.Loosely(t, issueData.Issue, should.Match(expectedIssue))
					assert.Loosely(t, len(issueData.Comments), should.Equal(4))
					// Policy notifications should be in order of priority.
					assert.Loosely(t, issueData.Comments[1].Comment, should.HavePrefix("Policy ID: policy-b"))
					assert.Loosely(t, issueData.Comments[2].Comment, should.HavePrefix("Policy ID: policy-c"))
					assert.Loosely(t, issueData.Comments[3].Comment, should.HavePrefix("Policy ID: policy-a"))
				})
			})
			t.Run("With test name failure cluster", func(t *ftt.Test) {
				createRequest.Description.Title = "ninja://:blink_web_tests/media/my-suite/my-test.html"
				createRequest.Description.Description = "A test is failing " + createRequest.Description.Title
				expectedIssue.Description = &issuetracker.IssueComment{
					CommentNumber: 1,
					Comment: "A test is failing ninja://:blink_web_tests/media/my-suite/my-test.html\n" +
						"\n" +
						"These test failures are causing problem(s) which require your attention, including:\n" +
						"- Problem A\n" +
						"\n" +
						"See current problems, failure examples and more in LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id\n" +
						"\n" +
						"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
						"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
						"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection",
				}
				expectedIssue.IssueState.Title = "Tests are failing: ninja://:blink_web_tests/media/my-suite/my-test.html"

				response := bm.Create(ctx, createRequest)
				assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				}))
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				issue := fakeStore.Issues[1]

				assert.Loosely(t, issue.Issue, should.Match(expectedIssue))
				assert.Loosely(t, len(issue.Comments), should.Equal(2))
				assert.Loosely(t, issue.Comments[0].Comment, should.ContainSubstring("https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id"))
				assert.Loosely(t, issue.Comments[1].Comment, should.HavePrefix("Policy ID: policy-a"))
			})

			t.Run("With provided component id", func(t *ftt.Test) {
				createRequest.BuganizerComponent = 7890
				response := bm.Create(ctx, createRequest)
				assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				}))
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				issue := fakeStore.Issues[1]
				assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(7890))
			})

			t.Run("With provided component id without permission", func(t *ftt.Test) {
				createRequest.BuganizerComponent = ComponentWithNoAccess
				response := bm.Create(ctx, createRequest)
				assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				}))
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				issue := fakeStore.Issues[1]
				// Should have fallback component ID because no permission to wanted component.
				assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(buganizerCfg.DefaultComponent.Id))
				// No permission to component should appear in comments.
				assert.Loosely(t, len(issue.Comments), should.Equal(3))
				assert.Loosely(t, issue.Comments[1].Comment, should.ContainSubstring("LUCI Analysis does not have permissions to that component"))
				assert.Loosely(t, issue.Comments[1].Comment, should.ContainSubstring(strconv.Itoa(ComponentWithNoAccess)))
				assert.Loosely(t, issue.Comments[2].Comment, should.HavePrefix("Policy ID: policy-a"))
			})

			t.Run("With provided component id that is archived", func(t *ftt.Test) {
				createRequest.BuganizerComponent = ComponentWithIsArchivedSet
				response := bm.Create(ctx, createRequest)
				assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				}))
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				issue := fakeStore.Issues[1]
				// Should have fallback component ID because no permission to wanted component.
				assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(buganizerCfg.DefaultComponent.Id))
				// Component archived comment should appear in comments.
				assert.Loosely(t, len(issue.Comments), should.Equal(3))
				assert.Loosely(t, issue.Comments[1].Comment, should.ContainSubstring("because that component is archived"))
				assert.Loosely(t, issue.Comments[1].Comment, should.ContainSubstring(strconv.Itoa(ComponentWithIsArchivedSet)))
				assert.Loosely(t, issue.Comments[2].Comment, should.HavePrefix("Policy ID: policy-a"))
			})

			t.Run("With Buganizer test mode", func(t *ftt.Test) {
				createRequest.BuganizerComponent = 1234
				// TODO: Mock permission call to fail.
				ctx = context.WithValue(ctx, &BuganizerTestModeKey, true)
				response := bm.Create(ctx, createRequest)
				assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				}))
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				issue := fakeStore.Issues[1]
				// Should have fallback component ID because no permission to wanted component.
				assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(buganizerCfg.DefaultComponent.Id))
				assert.Loosely(t, len(issue.Comments), should.Equal(3))
				assert.Loosely(t, issue.Comments[1].Comment, should.ContainSubstring("This bug was filed in the fallback component"))
				assert.Loosely(t, issue.Comments[2].Comment, should.HavePrefix("Policy ID: policy-a"))
			})

			t.Run("With Limit View Trusted", func(t *ftt.Test) {
				// Check config is respected and we file with Limit View Trusted if the
				// config option to file without it is not set.
				buganizerCfg.FileWithoutLimitViewTrusted = false
				bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
				assert.Loosely(t, err, should.BeNil)

				t.Run("With internal component", func(t *ftt.Test) {
					fakeClient.ComponentAccessLevel = issuetracker.AccessLimit_INTERNAL
					response := bm.Create(ctx, createRequest)
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
					issue := fakeStore.Issues[1]
					assert.Loosely(t, issue.Issue.IssueState.AccessLimit.AccessLevel, should.Equal(issuetracker.IssueAccessLimit_LIMIT_NONE))
				})

				t.Run("With public component", func(t *ftt.Test) {
					fakeClient.ComponentAccessLevel = issuetracker.AccessLimit_EXTERNAL_PUBLIC
					response := bm.Create(ctx, createRequest)
					assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					}))
					assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
					issue := fakeStore.Issues[1]
					assert.Loosely(t, issue.Issue.IssueState.AccessLimit.AccessLevel, should.Equal(issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED))
				})
			})
		})
		t.Run("Update", func(t *ftt.Test) {
			c := newCreateRequest()
			c.BugManagementState = &bugspb.BugManagementState{
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": {
						IsActive: true,
					}, // P4
					"policy-c": {
						IsActive: true,
					}, // P1
				},
			}
			response := bm.Create(ctx, c)
			assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
					"policy-c": {},
				},
			}))
			assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
			assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P1))
			assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(3))

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
					Bug:                              bugs.BugID{System: bugs.BuganizerSystem, ID: response.ID},
					BugManagementState:               state,
					IsManagingBug:                    true,
					RuleID:                           "rule-id",
					IsManagingBugPriority:            true,
					IsManagingBugPriorityLastUpdated: clock.Now(ctx),
				},
			}
			expectedResponse := []bugs.BugUpdateResponse{
				{
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					BugTitle:                  "Tests are failing: ClusterID",
				},
			}
			verifyUpdateDoesNothing := func(t testing.TB) error {
				t.Helper()
				originalIssue := proto.Clone(fakeStore.Issues[1].Issue).(*issuetracker.Issue)
				response, err := bm.Update(ctx, bugsToUpdate)
				if err != nil {
					return errors.Fmt("update bugs: %w", err)
				}
				assert.That(t, response, should.Match(expectedResponse), truth.LineContext())
				assert.That(t, fakeStore.Issues[1].Issue, should.Match(originalIssue), truth.LineContext())
				return nil
			}

			t.Run("If less than expected issues are returned, should not fail", func(t *ftt.Test) {
				fakeStore.Issues = map[int64]*IssueData{}

				bugsToUpdate := []bugs.BugUpdateRequest{
					{
						Bug:                              bugs.BugID{System: bugs.BuganizerSystem, ID: response.ID},
						RuleID:                           "rule-id",
						IsManagingBug:                    true,
						IsManagingBugPriority:            true,
						IsManagingBugPriorityLastUpdated: clock.Now(ctx),
					},
				}
				expectedResponse = []bugs.BugUpdateResponse{
					{
						IsDuplicate:               false,
						ShouldArchive:             false,
						PolicyActivationsNotified: make(map[bugs.PolicyID]struct{}),
					},
				}
				response, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(expectedResponse))
			})

			t.Run("If active policies unchanged and no rule notification pending, does nothing", func(t *ftt.Test) {
				assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
			})
			t.Run("Notifies association between bug and rule", func(t *ftt.Test) {
				// Setup
				// When RuleAssociationNotified is false.
				bugsToUpdate[0].BugManagementState.RuleAssociationNotified = false
				// Even if ManagingBug is also false.
				bugsToUpdate[0].IsManagingBug = false

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				assert.Loosely(t, err, should.BeNil)

				// RuleAssociationNotified is set.
				expectedResponse[0].RuleAssociationNotified = true
				assert.Loosely(t, response, should.Match(expectedResponse))

				// Expect a comment on the bug notifying us about the association.
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.Equal(
					"This bug has been associated with failures in LUCI Analysis. "+
						"To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))
			})
			t.Run("If active policies changed", func(t *ftt.Test) {
				// De-activates policy-c (P1), leaving only policy-a (P4) active.
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].IsActive = false
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				t.Run("Does not update bug priority/verified if IsManagingBug false", func(t *ftt.Test) {
					bugsToUpdate[0].IsManagingBug = false

					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
				})
				t.Run("Notifies policy activation, even if IsManagingBug false", func(t *ftt.Test) {
					bugsToUpdate[0].IsManagingBug = false

					// Activate policy B (P0).
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{
						"policy-b": {},
					}

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))

					// Bug priority should NOT be increased to P0, because IsManagingBug is false.
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.NotEqual(issuetracker.Issue_P0))

					// Expect the policy B activation comment to appear.
					assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"Policy ID: policy-b"))

					// Expect policy B's hotlist to be added to the bug.
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.HotlistIds, should.Match([]int64{1001, 1002, 1003}))

					// Verify repeated update has no effect.
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true
					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}
					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
				})
				t.Run("Reduces priority in response to policies de-activating", func(t *ftt.Test) {
					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P4))
					assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"Because the following problem(s) have stopped:\n"+
							"- Problem C (P1)\n"+
							"The bug priority has been decreased from P1 to P4."))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/help#priority-update"))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

					// Verify repeated update has no effect.
					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
				})
				t.Run("Increases priority in response to priority policies activating", func(t *ftt.Test) {
					// Activate policy B (P0).
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{
						"policy-b": {},
					}

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))
					assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+2))
					// Expect the policy B activation comment to appear, followed by the priority update comment.
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"Policy ID: policy-b"))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, should.ContainSubstring(
						"Because the following problem(s) have started:\n"+
							"- Problem B (P0)\n"+
							"The bug priority has been increased from P1 to P0."))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/help#priority-update"))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

					// Verify repeated update has no effect.
					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
				})
				t.Run("Does not adjust priority if priority manually set", func(t *ftt.Test) {
					ctx := context.WithValue(ctx, &BuganizerSelfEmailKey, "luci-analysis@prod.google.com")
					fakeStore.Issues[1].Issue.IssueState.Priority = issuetracker.Issue_P0
					fakeStore.Issues[1].IssueUpdates = append(fakeStore.Issues[1].IssueUpdates, &issuetracker.IssueUpdate{
						Author: &issuetracker.User{
							EmailAddress: "testuser@google.com",
						},
						Timestamp: timestamppb.New(clock.Now(ctx).Add(time.Minute * 4)),
						FieldUpdates: []*issuetracker.FieldUpdate{
							{
								Field: "priority",
							},
						},
					})
					response, err := bm.Update(ctx, bugsToUpdate)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))
					expectedResponse[0].DisableRulePriorityUpdates = true
					assert.Loosely(t, response[0].DisableRulePriorityUpdates, should.BeTrue)
					assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.Equal(
						"The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis,"+
							" enable the update priority flag on the rule.\n\nSee failure impact and configure the failure"+
							" association rule for this bug at: https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

					// Normally, the caller would update IsManagingBugPriority to false
					// now, but as this is a test, we have to do it manually.
					// As priority updates are now off, DisableRulePriorityUpdates
					// should henceforth also return false (as they are already
					// disabled).
					expectedResponse[0].DisableRulePriorityUpdates = false
					bugsToUpdate[0].IsManagingBugPriority = false
					bugsToUpdate[0].IsManagingBugPriorityLastUpdated = tc.Now().Add(1 * time.Minute)

					// Check repeated update does nothing more.
					initialComments := len(fakeStore.Issues[1].Comments)
					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
					assert.Loosely(t, len(fakeStore.Issues[1].Comments), should.Equal(initialComments))

					t.Run("Unless IsManagingBugPriority manually updated", func(t *ftt.Test) {
						bugsToUpdate[0].IsManagingBugPriority = true
						bugsToUpdate[0].IsManagingBugPriorityLastUpdated = clock.Now(ctx).Add(time.Minute * 15)

						response, err := bm.Update(ctx, bugsToUpdate)
						assert.Loosely(t, response, should.Match(expectedResponse))
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P4))
						assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(initialComments+1))
						assert.Loosely(t, fakeStore.Issues[1].Comments[initialComments].Comment, should.ContainSubstring(
							"Because the following problem(s) are active:\n"+
								"- Problem A (P4)\n"+
								"\n"+
								"The bug priority has been set to P4."))
						assert.Loosely(t, fakeStore.Issues[1].Comments[initialComments].Comment, should.ContainSubstring(
							"https://luci-analysis-test.appspot.com/help#priority-update"))
						assert.Loosely(t, fakeStore.Issues[1].Comments[initialComments].Comment, should.ContainSubstring(
							"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

						// Verify repeated update has no effect.
						assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
					})
				})
			})
			t.Run("If all policies deactivate", func(t *ftt.Test) {
				// De-activate all policies, so the bug would normally be marked verified.
				for _, policyState := range bugsToUpdate[0].BugManagementState.PolicyState {
					if policyState.IsActive {
						policyState.IsActive = false
						policyState.LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
					}
				}

				t.Run("Does not update bug if IsManagingBug false", func(t *ftt.Test) {
					bugsToUpdate[0].IsManagingBug = false

					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
				})
				t.Run("Sets verifier and assignee to luci analysis if assignee is nil", func(t *ftt.Test) {
					fakeStore.Issues[1].Issue.IssueState.Assignee = nil

					response, err := bm.Update(ctx, bugsToUpdate)

					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Verifier.EmailAddress, should.Equal("email@test.com"))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Assignee.EmailAddress, should.Equal("email@test.com"))

					t.Run("If re-opening, LUCI Analysis assignee is removed", func(t *ftt.Test) {
						bugsToUpdate[0].BugManagementState.PolicyState["policy-a"].IsActive = true
						bugsToUpdate[0].BugManagementState.PolicyState["policy-a"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))

						response, err := bm.Update(ctx, bugsToUpdate)

						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response, should.Match(expectedResponse))
						assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
						assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Assignee, should.BeNil)
					})
				})

				t.Run("Update closes bug", func(t *ftt.Test) {
					fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{
						EmailAddress: "user@google.com",
					}

					response, err := bm.Update(ctx, bugsToUpdate)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))
					assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_VERIFIED))

					expectedComment := "Because the following problem(s) have stopped:\n" +
						"- Problem C (P1)\n" +
						"- Problem A (P4)\n" +
						"The bug has been verified."
					assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(expectedComment))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/help#bug-verified"))
					assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

					// Verify repeated update has no effect.
					assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)

					t.Run("Rules for verified bugs archived after 30 days", func(t *ftt.Test) {
						tc.Add(time.Hour * 24 * 30)

						expectedResponse := []bugs.BugUpdateResponse{
							{
								ShouldArchive:             true,
								PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
							},
						}
						tc.Add(time.Minute * 2)
						response, err := bm.Update(ctx, bugsToUpdate)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response, should.Match(expectedResponse))
						assert.Loosely(t, fakeStore.Issues[1].Issue.ModifiedTime, should.Match(timestamppb.New(now)))
					})

					t.Run("If policies re-activate, bug is re-opened with correct priority", func(t *ftt.Test) {
						// policy-b has priority P0.
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true

						originalCommentCount := len(fakeStore.Issues[1].Comments)

						t.Run("Issue has owner", func(t *ftt.Test) {
							fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{
								EmailAddress: "testuser@google.com",
							}

							// Issue should return to "Assigned" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, response, should.Match(expectedResponse))
							assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_ASSIGNED))
							assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(expectedComment))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
								"https://luci-analysis-test.appspot.com/help#bug-reopened"))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
								"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

							// Verify repeated update has no effect.
							assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
						})
						t.Run("Issue has no assignee", func(t *ftt.Test) {
							fakeStore.Issues[1].Issue.IssueState.Assignee = nil

							// Issue should return to "Untriaged" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, response, should.Match(expectedResponse))
							assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
							assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Priority, should.Equal(issuetracker.Issue_P0))

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(expectedComment))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
								"https://luci-analysis-test.appspot.com/help#priority-update"))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
								"https://luci-analysis-test.appspot.com/help#bug-reopened"))
							assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
								"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id"))

							// Verify repeated update has no effect.
							assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
						})
					})
				})
			})
			t.Run("If bug duplicate", func(t *ftt.Test) {
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
				expectedResponse := []bugs.BugUpdateResponse{
					{
						IsDuplicate:               true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(expectedResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.ModifiedTime, should.Match(timestamppb.New(clock.Now(ctx))))
			})
			t.Run("Rule not managing a bug archived after 30 days of the bug being in any closed state", func(t *ftt.Test) {
				bugsToUpdate[0].IsManagingBug = false
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
				fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(tc.Now())

				tc.Add(time.Hour * 24 * 30)

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}
				originalTime := timestamppb.New(fakeStore.Issues[1].Issue.ModifiedTime.AsTime())

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(expectedResponse))
				assert.Loosely(t, fakeStore.Issues[1].Issue.ModifiedTime, should.Match(originalTime))
			})
			t.Run("Rule managing a bug not archived after 30 days of the bug being in fixed state", func(t *ftt.Test) {
				// If LUCI Analysis is mangaging the bug state, the fixed state
				// means the bug is still not verified. Do not archive the
				// rule.
				bugsToUpdate[0].IsManagingBug = true
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
				fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(tc.Now())

				tc.Add(time.Hour * 24 * 30)

				assert.Loosely(t, verifyUpdateDoesNothing(t), should.BeNil)
			})

			t.Run("Rules archived immediately if bug archived", func(t *ftt.Test) {
				fakeStore.Issues[1].Issue.IsArchived = true

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(expectedResponse))
			})
			t.Run("If issue does not exist, does nothing", func(t *ftt.Test) {
				fakeStore.Issues = nil
				response, err := bm.Update(ctx, bugsToUpdate)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response), should.Equal(len(bugsToUpdate)))
				assert.Loosely(t, fakeStore.Issues, should.BeNil)
			})
			t.Run("Falsify a bug closure after a user marks a bug as fixed", func(t *ftt.Test) {
				assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
				now := time.Now()
				twentyFiveHoursAgo := now.Add(-25 * time.Hour)
				fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(twentyFiveHoursAgo)
				bugsToUpdate := []bugs.BugUpdateRequest{
					{
						Bug:                              bugs.BugID{System: bugs.BuganizerSystem, ID: response.ID},
						BugManagementState:               state,
						IsManagingBug:                    true,
						RuleID:                           "rule-id",
						IsManagingBugPriority:            true,
						IsManagingBugPriorityLastUpdated: clock.Now(ctx),
						InvalidationStatus: bugs.BugClosureInvalidationStatus{
							OneDay: bugs.BugClosureInvalidationResult{
								IsInvalidated: true,
								ActivePolicyIDs: map[bugs.PolicyID]struct{}{
									bugs.PolicyID(policyA.Id): struct{}{},
								},
							},
							ThreeDay: bugs.BugClosureInvalidationResult{
								IsInvalidated: true,
							},
							SevenDay: bugs.BugClosureInvalidationResult{
								IsInvalidated: true,
							},
						},
					},
				}
				expectedResponse = []bugs.BugUpdateResponse{
					{
						PolicyActivationsNotified: make(map[bugs.PolicyID]struct{}),
						BugClosureValidationResult: bugs.BugClosureInvalidationResult{
							IsInvalidated: true,
							ActivePolicyIDs: map[bugs.PolicyID]struct{}{
								bugs.PolicyID(policyA.Id): struct{}{},
							},
						},
						BugTitle: "Tests are failing: ClusterID",
					},
				}

				t.Run("mark bug closure as invalid", func(t *ftt.Test) {
					response, err := bm.Update(ctx, bugsToUpdate)

					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(expectedResponse))
				})
			})
		})
		t.Run("GetMergedInto", func(t *ftt.Test) {
			c := newCreateRequest()
			c.BugManagementState = &bugspb.BugManagementState{
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": {
						IsActive: true,
					}, // P4
				},
			}
			response := bm.Create(ctx, c)
			assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			}))
			assert.Loosely(t, len(fakeStore.Issues), should.Equal(1))

			bugID := bugs.BugID{System: bugs.BuganizerSystem, ID: "1"}
			t.Run("Merged into Buganizer bug", func(t *ftt.Test) {
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
				fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

				result, err := bm.GetMergedInto(ctx, bugID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.Match(&bugs.BugID{
					System: bugs.BuganizerSystem,
					ID:     "2",
				}))
			})
			t.Run("Not merged into any bug", func(t *ftt.Test) {
				// While MergedIntoIssueRef is set, the bug status is not
				// set to "Duplicate", so this value should be ignored.
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_NEW
				fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

				result, err := bm.GetMergedInto(ctx, bugID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, result, should.BeNil)
			})
		})
		t.Run("UpdateDuplicateSource", func(t *ftt.Test) {
			c := newCreateRequest()
			c.BugManagementState = &bugspb.BugManagementState{
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": {
						IsActive: true,
					}, // P4
				},
			}
			response := bm.Create(ctx, c)
			assert.Loosely(t, response, should.Match(bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			}))
			assert.Loosely(t, fakeStore.Issues, should.HaveLength(1))
			assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(2))
			originalCommentCount := len(fakeStore.Issues[1].Comments)

			fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
			fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

			t.Run("With ErrorMessage", func(t *ftt.Test) {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-rule-id",
						Bug:    bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					},
					ErrorMessage: "Some error.",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_NEW))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring("Some error."))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/source-rule-id"))
			})
			t.Run("With ErrorMessage and IsAssigned is true", func(t *ftt.Test) {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID:     "source-rule-id",
						Bug:        bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
						IsAssigned: true,
					},
					ErrorMessage: "Some error.",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_ASSIGNED))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring("Some error."))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/source-rule-id"))
			})
			t.Run("Without ErrorMessage", func(t *ftt.Test) {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-bug-rule-id",
						Bug:    bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					},
					DestinationRuleID: "12345abcdef",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, fakeStore.Issues[1].Issue.IssueState.Status, should.Equal(issuetracker.Issue_DUPLICATE))
				assert.Loosely(t, fakeStore.Issues[1].Comments, should.HaveLength(originalCommentCount+1))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring("merged the failure association rule for this bug into the rule for the canonical bug."))
				assert.Loosely(t, fakeStore.Issues[1].Comments[originalCommentCount].Comment, should.ContainSubstring(
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/12345abcdef"))
			})
		})
	})
}

func newCreateRequest() bugs.BugCreateRequest {
	cluster := bugs.BugCreateRequest{
		Description: &clustering.ClusterDescription{
			Title:       "ClusterID",
			Description: "Tests are failing with reason: Some failure reason.",
		},
		RuleID: "new-rule-id",
	}
	return cluster
}
