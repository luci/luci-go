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
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBugManager(t *testing.T) {
	t.Parallel()

	Convey("With Bug Manager", t, func() {
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
			},
		}

		bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
		So(err, ShouldBeNil)
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("Create", func() {
			createRequest := newCreateRequest()
			createRequest.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {}, // P4
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

			Convey("With reason-based failure cluster", func() {
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

				Convey("Base case", func() {
					response := bm.Create(ctx, createRequest)
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(fakeStore.Issues), ShouldEqual, 1)

					issueData := fakeStore.Issues[1]
					So(issueData.Issue, ShouldResembleProto, expectedIssue)
				})

				Convey("Policy with comment template", func() {
					policyA.BugTemplate.CommentTemplate = "RuleURL:{{.RuleURL}},BugID:{{if .BugID.IsBuganizer}}{{.BugID.BuganizerBugID}}{{end}}"

					bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
					So(err, ShouldBeNil)

					response := bm.Create(ctx, createRequest)
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(fakeStore.Issues), ShouldEqual, 1)

					issueData := fakeStore.Issues[1]
					So(issueData.Issue, ShouldResembleProto, expectedIssue)
					// Expect no comment for policy-a's activation, just the initial issue description.
					So(len(issueData.Comments), ShouldEqual, 2)
					So(issueData.Comments[1].Comment, ShouldEqual, "RuleURL:https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id,BugID:1\n\n"+
						"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: policy-a)")
				})
				Convey("Policy has no comment template", func() {
					policyA.BugTemplate.CommentTemplate = ""

					bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
					So(err, ShouldBeNil)

					response := bm.Create(ctx, createRequest)
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(fakeStore.Issues), ShouldEqual, 1)

					issueData := fakeStore.Issues[1]
					So(issueData.Issue, ShouldResembleProto, expectedIssue)
					// Expect no comment for policy-a's activation, just the initial issue description.
					So(len(issueData.Comments), ShouldEqual, 1)
				})
				Convey("Multiple policies activated", func() {
					createRequest.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
						"policy-a": {}, // P4
						"policy-b": {}, // P0
						"policy-c": {}, // P1
					}
					expectedIssue.Description.Comment = strings.Replace(expectedIssue.Description.Comment, "- Problem A\n", "- Problem B\n- Problem C\n- Problem A\n", 1)
					expectedIssue.IssueState.Priority = issuetracker.Issue_P0
					expectedIssue.IssueState.HotlistIds = []int64{1001, 1002, 1003}

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "1",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
							"policy-b": {},
							"policy-c": {},
						},
					})
					So(len(fakeStore.Issues), ShouldEqual, 1)

					issueData := fakeStore.Issues[1]
					So(issueData.Issue, ShouldResembleProto, expectedIssue)
					So(len(issueData.Comments), ShouldEqual, 4)
					// Policy notifications should be in order of priority.
					So(issueData.Comments[1].Comment, ShouldStartWith, "Policy ID: policy-b")
					So(issueData.Comments[2].Comment, ShouldStartWith, "Policy ID: policy-c")
					So(issueData.Comments[3].Comment, ShouldStartWith, "Policy ID: policy-a")
				})
			})
			Convey("With test name failure cluster", func() {
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
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(fakeStore.Issues), ShouldEqual, 1)
				issue := fakeStore.Issues[1]

				So(issue.Issue, ShouldResembleProto, expectedIssue)
				So(len(issue.Comments), ShouldEqual, 2)
				So(issue.Comments[0].Comment, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/p/chromeos/rules/new-rule-id")
				So(issue.Comments[1].Comment, ShouldStartWith, "Policy ID: policy-a")
			})

			Convey("With provided component id", func() {
				createRequest.BuganizerComponent = 7890
				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(fakeStore.Issues), ShouldEqual, 1)
				issue := fakeStore.Issues[1]
				So(issue.Issue.IssueState.ComponentId, ShouldEqual, 7890)
			})

			Convey("With provided component id without permission", func() {
				createRequest.BuganizerComponent = ComponentWithNoAccess
				// TODO: Mock permission call to fail.
				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(fakeStore.Issues), ShouldEqual, 1)
				issue := fakeStore.Issues[1]
				// Should have fallback component ID because no permission to wanted component.
				So(issue.Issue.IssueState.ComponentId, ShouldEqual, buganizerCfg.DefaultComponent.Id)
				// No permission to component should appear in comments.
				So(len(issue.Comments), ShouldEqual, 3)
				So(issue.Comments[1].Comment, ShouldContainSubstring, strconv.Itoa(ComponentWithNoAccess))
				So(issue.Comments[2].Comment, ShouldStartWith, "Policy ID: policy-a")
			})

			Convey("With Buganizer test mode", func() {
				createRequest.BuganizerComponent = 1234
				// TODO: Mock permission call to fail.
				ctx = context.WithValue(ctx, &BuganizerTestModeKey, true)
				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(fakeStore.Issues), ShouldEqual, 1)
				issue := fakeStore.Issues[1]
				// Should have fallback component ID because no permission to wanted component.
				So(issue.Issue.IssueState.ComponentId, ShouldEqual, buganizerCfg.DefaultComponent.Id)
				So(len(issue.Comments), ShouldEqual, 3)
				So(issue.Comments[1].Comment, ShouldContainSubstring, "This bug was filed in the fallback component")
				So(issue.Comments[2].Comment, ShouldStartWith, "Policy ID: policy-a")
			})

			Convey("With Limit View Trusted", func() {
				// Check config is respected and we file with Limit View Trusted if the
				// config option to file without it is not set.
				buganizerCfg.FileWithoutLimitViewTrusted = false
				bm, err := NewBugManager(fakeClient, "https://luci-analysis-test.appspot.com", "chromeos", "email@test.com", projectCfg)
				So(err, ShouldBeNil)

				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "1",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(fakeStore.Issues), ShouldEqual, 1)
				issue := fakeStore.Issues[1]
				So(issue.Issue.IssueState.AccessLimit.AccessLevel, ShouldEqual, issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED)
			})
		})
		Convey("Update", func() {
			c := newCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {}, // P4
				"policy-c": {}, // P1
			}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
					"policy-c": {},
				},
			})
			So(len(fakeStore.Issues), ShouldEqual, 1)
			So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P1)
			So(fakeStore.Issues[1].Comments, ShouldHaveLength, 3)

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
					IsDuplicate:               false,
					ShouldArchive:             false,
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
				},
			}
			verifyUpdateDoesNothing := func() error {
				originalIssue := proto.Clone(fakeStore.Issues[1].Issue).(*issuetracker.Issue)
				response, err := bm.Update(ctx, bugsToUpdate)
				if err != nil {
					return errors.Annotate(err, "update bugs").Err()
				}
				if diff := ShouldResemble(response, expectedResponse); diff != "" {
					return errors.Reason("response: %s", diff).Err()
				}
				if diff := ShouldResembleProto(fakeStore.Issues[1].Issue, originalIssue); diff != "" {
					return errors.Reason("issue 1: %s", diff).Err()
				}
				return nil
			}

			Convey("If less than expected issues are returned, should not fail", func() {
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
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
			})

			Convey("If active policies unchanged and no rule notification pending, does nothing", func() {
				So(verifyUpdateDoesNothing(), ShouldBeNil)
			})
			Convey("Notifies association between bug and rule", func() {
				// Setup
				// When RuleAssociationNotified is false.
				bugsToUpdate[0].BugManagementState.RuleAssociationNotified = false
				// Even if ManagingBug is also false.
				bugsToUpdate[0].IsManagingBug = false

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				So(err, ShouldBeNil)

				// RuleAssociationNotified is set.
				expectedResponse[0].RuleAssociationNotified = true
				So(response, ShouldResemble, expectedResponse)

				// Expect a comment on the bug notifying us about the association.
				So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldEqual,
					"This bug has been associated with failures in LUCI Analysis. "+
						"To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")
			})
			Convey("If active policies changed", func() {
				// De-activates policy-c (P1), leaving only policy-a (P4) active.
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].IsActive = false
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				Convey("Does not update bug priority/verified if IsManagingBug false", func() {
					bugsToUpdate[0].IsManagingBug = false

					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Notifies policy activation, even if IsManagingBug false", func() {
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
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)

					// Bug priority should NOT be increased to P0, because IsManagingBug is false.
					So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldNotEqual, issuetracker.Issue_P0)

					// Expect the policy B activation comment to appear.
					So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"Policy ID: policy-b")

					// Expect policy B's hotlist to be added to the bug.
					So(fakeStore.Issues[1].Issue.IssueState.HotlistIds, ShouldResemble, []int64{1001, 1002, 1003})

					// Verify repeated update has no effect.
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true
					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Reduces priority in response to policies de-activating", func() {
					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P4)
					So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"Because the following problem(s) have stopped:\n"+
							"- Problem C (P1)\n"+
							"The bug priority has been decreased from P1 to P4.")
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Increases priority in response to priority policies activating", func() {
					// Activate policy B (P0).
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{
						"policy-b": {},
					}

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)
					So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+2)
					// Expect the policy B activation comment to appear, followed by the priority update comment.
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"Policy ID: policy-b")
					So(fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, ShouldContainSubstring,
						"Because the following problem(s) have started:\n"+
							"- Problem B (P0)\n"+
							"The bug priority has been increased from P1 to P0.")
					So(fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(fakeStore.Issues[1].Comments[originalCommentCount+1].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Does not adjust priority if priority manually set", func() {
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
					So(err, ShouldBeNil)
					So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)
					expectedResponse[0].DisableRulePriorityUpdates = true
					So(response[0].DisableRulePriorityUpdates, ShouldBeTrue)
					So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldEqual,
						"The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis,"+
							" enable the update priority flag on the rule.\n\nSee failure impact and configure the failure"+
							" association rule for this bug at: https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

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
					So(verifyUpdateDoesNothing(), ShouldBeNil)
					So(len(fakeStore.Issues[1].Comments), ShouldEqual, initialComments)

					Convey("Unless IsManagingBugPriority manually updated", func() {
						bugsToUpdate[0].IsManagingBugPriority = true
						bugsToUpdate[0].IsManagingBugPriorityLastUpdated = clock.Now(ctx).Add(time.Minute * 15)

						response, err := bm.Update(ctx, bugsToUpdate)
						So(response, ShouldResemble, expectedResponse)
						So(err, ShouldBeNil)
						So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P4)
						So(fakeStore.Issues[1].Comments, ShouldHaveLength, initialComments+1)
						So(fakeStore.Issues[1].Comments[initialComments].Comment, ShouldContainSubstring,
							"Because the following problem(s) are active:\n"+
								"- Problem A (P4)\n"+
								"\n"+
								"The bug priority has been set to P4.")
						So(fakeStore.Issues[1].Comments[initialComments].Comment, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/help#priority-update")
						So(fakeStore.Issues[1].Comments[initialComments].Comment, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

						// Verify repeated update has no effect.
						So(verifyUpdateDoesNothing(), ShouldBeNil)
					})
				})
			})
			Convey("If all policies deactivate", func() {
				// De-activate all policies, so the bug would normally be marked verified.
				for _, policyState := range bugsToUpdate[0].BugManagementState.PolicyState {
					if policyState.IsActive {
						policyState.IsActive = false
						policyState.LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))
					}
				}

				Convey("Does not update bug if IsManagingBug false", func() {
					bugsToUpdate[0].IsManagingBug = false

					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Sets verifier and assignee to luci analysis if assignee is nil", func() {
					fakeStore.Issues[1].Issue.IssueState.Assignee = nil

					response, err := bm.Update(ctx, bugsToUpdate)

					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
					So(fakeStore.Issues[1].Issue.IssueState.Verifier.EmailAddress, ShouldEqual, "email@test.com")
					So(fakeStore.Issues[1].Issue.IssueState.Assignee.EmailAddress, ShouldEqual, "email@test.com")

					Convey("If re-opening, LUCI Analysis assignee is removed", func() {
						bugsToUpdate[0].BugManagementState.PolicyState["policy-a"].IsActive = true
						bugsToUpdate[0].BugManagementState.PolicyState["policy-a"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))

						response, err := bm.Update(ctx, bugsToUpdate)

						So(err, ShouldBeNil)
						So(response, ShouldResemble, expectedResponse)
						So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
						So(fakeStore.Issues[1].Issue.IssueState.Assignee, ShouldBeNil)
					})
				})

				Convey("Update closes bug", func() {
					fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{
						EmailAddress: "user@google.com",
					}

					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)

					expectedComment := "Because the following problem(s) have stopped:\n" +
						"- Problem C (P1)\n" +
						"- Problem A (P4)\n" +
						"The bug has been verified."
					So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, expectedComment)
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#bug-verified")
					So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)

					Convey("Rules for verified bugs archived after 30 days", func() {
						tc.Add(time.Hour * 24 * 30)

						expectedResponse := []bugs.BugUpdateResponse{
							{
								ShouldArchive:             true,
								PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
							},
						}
						tc.Add(time.Minute * 2)
						response, err := bm.Update(ctx, bugsToUpdate)
						So(err, ShouldBeNil)
						So(response, ShouldResemble, expectedResponse)
						So(fakeStore.Issues[1].Issue.ModifiedTime, ShouldResembleProto, timestamppb.New(now))
					})

					Convey("If policies re-activate, bug is re-opened with correct priority", func() {
						// policy-b has priority P0.
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true

						originalCommentCount := len(fakeStore.Issues[1].Comments)

						Convey("Issue has owner", func() {
							fakeStore.Issues[1].Issue.IssueState.Assignee = &issuetracker.User{
								EmailAddress: "testuser@google.com",
							}

							// Issue should return to "Assigned" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_ASSIGNED)
							So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, expectedComment)
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

							// Verify repeated update has no effect.
							So(verifyUpdateDoesNothing(), ShouldBeNil)
						})
						Convey("Issue has no assignee", func() {
							fakeStore.Issues[1].Issue.IssueState.Assignee = nil

							// Issue should return to "Untriaged" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
							So(fakeStore.Issues[1].Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, expectedComment)
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#priority-update")
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/p/chromeos/rules/rule-id")

							// Verify repeated update has no effect.
							So(verifyUpdateDoesNothing(), ShouldBeNil)
						})
					})
				})
			})
			Convey("If bug duplicate", func() {
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
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(fakeStore.Issues[1].Issue.ModifiedTime, ShouldResembleProto, timestamppb.New(clock.Now(ctx)))
			})
			Convey("Rule not managing a bug archived after 30 days of the bug being in any closed state", func() {
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
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(fakeStore.Issues[1].Issue.ModifiedTime, ShouldResembleProto, originalTime)
			})
			Convey("Rule managing a bug not archived after 30 days of the bug being in fixed state", func() {
				// If LUCI Analysis is mangaging the bug state, the fixed state
				// means the bug is still not verified. Do not archive the
				// rule.
				bugsToUpdate[0].IsManagingBug = true
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_FIXED
				fakeStore.Issues[1].Issue.ResolvedTime = timestamppb.New(tc.Now())

				tc.Add(time.Hour * 24 * 30)

				So(verifyUpdateDoesNothing(), ShouldBeNil)
			})

			Convey("Rules archived immediately if bug archived", func() {
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
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
			})
			Convey("If issue does not exist, does nothing", func() {
				fakeStore.Issues = nil
				response, err := bm.Update(ctx, bugsToUpdate)
				So(err, ShouldBeNil)
				So(len(response), ShouldEqual, len(bugsToUpdate))
				So(fakeStore.Issues, ShouldBeNil)
			})
		})
		Convey("GetMergedInto", func() {
			c := newCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{"policy-a": {}}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			})
			So(len(fakeStore.Issues), ShouldEqual, 1)

			bugID := bugs.BugID{System: bugs.BuganizerSystem, ID: "1"}
			Convey("Merged into Buganizer bug", func() {
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
				fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

				result, err := bm.GetMergedInto(ctx, bugID)
				So(err, ShouldEqual, nil)
				So(result, ShouldResemble, &bugs.BugID{
					System: bugs.BuganizerSystem,
					ID:     "2",
				})
			})
			Convey("Not merged into any bug", func() {
				// While MergedIntoIssueRef is set, the bug status is not
				// set to "Duplicate", so this value should be ignored.
				fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_NEW
				fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

				result, err := bm.GetMergedInto(ctx, bugID)
				So(err, ShouldEqual, nil)
				So(result, ShouldBeNil)
			})
		})
		Convey("UpdateDuplicateSource", func() {
			c := newCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{"policy-a": {}}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "1",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			})
			So(fakeStore.Issues, ShouldHaveLength, 1)
			So(fakeStore.Issues[1].Comments, ShouldHaveLength, 2)
			originalCommentCount := len(fakeStore.Issues[1].Comments)

			fakeStore.Issues[1].Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
			fakeStore.Issues[1].Issue.IssueState.CanonicalIssueId = 2

			Convey("With ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-rule-id",
						Bug:    bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					},
					ErrorMessage: "Some error.",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
				So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, "Some error.")
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/source-rule-id")
			})
			Convey("With ErrorMessage and IsAssigned is true", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID:     "source-rule-id",
						Bug:        bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
						IsAssigned: true,
					},
					ErrorMessage: "Some error.",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_ASSIGNED)
				So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, "Some error.")
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/source-rule-id")
			})
			Convey("Without ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-bug-rule-id",
						Bug:    bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					},
					DestinationRuleID: "12345abcdef",
				}
				err := bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(fakeStore.Issues[1].Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_DUPLICATE)
				So(fakeStore.Issues[1].Comments, ShouldHaveLength, originalCommentCount+1)
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring, "merged the failure association rule for this bug into the rule for the canonical bug.")
				So(fakeStore.Issues[1].Comments[originalCommentCount].Comment, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/12345abcdef")
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
