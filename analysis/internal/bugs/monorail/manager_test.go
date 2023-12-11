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

package monorail

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func NewCreateRequest() bugs.BugCreateRequest {
	cluster := bugs.BugCreateRequest{
		Description: &clustering.ClusterDescription{
			Title:       "ClusterID",
			Description: "Tests are failing with reason: Some failure reason.",
		},
		MonorailComponents: []string{
			"Blink>Layout",
			"Blink>Network",
			"Blink>Invalid",
		},
		RuleID: "new-rule-id",
	}
	return cluster
}

func TestManager(t *testing.T) {
	t.Parallel()

	Convey("With Bug Manager", t, func() {
		ctx := context.Background()
		f := &FakeIssuesStore{
			NextID:            100,
			PriorityFieldName: "projects/chromium/fieldDefs/11",
			ComponentNames: []string{
				"projects/chromium/componentDefs/Blink",
				"projects/chromium/componentDefs/Blink>Layout",
				"projects/chromium/componentDefs/Blink>Network",
			},
		}
		user := AutomationUsers[0]
		cl, err := NewClient(UseFakeIssuesClient(ctx, f, user), "myhost")
		So(err, ShouldBeNil)
		monorailCfgs := ChromiumTestConfig()

		policyA := config.CreatePlaceholderBugManagementPolicy("policy-a")
		policyA.HumanReadableName = "Problem A"
		policyA.Priority = configpb.BuganizerPriority_P4
		policyA.BugTemplate.Monorail.Labels = []string{"Policy-A-Label"}

		policyB := config.CreatePlaceholderBugManagementPolicy("policy-b")
		policyB.HumanReadableName = "Problem B"
		policyB.Priority = configpb.BuganizerPriority_P0
		policyB.BugTemplate.Monorail.Labels = []string{"Policy-B-Label"}

		policyC := config.CreatePlaceholderBugManagementPolicy("policy-c")
		policyC.HumanReadableName = "Problem C"
		policyC.Priority = configpb.BuganizerPriority_P1
		policyC.BugTemplate.Monorail.Labels = []string{"Policy-C-Label"}

		projectCfg := &configpb.ProjectConfig{
			BugManagement: &configpb.BugManagement{
				DefaultBugSystem: configpb.BugSystem_MONORAIL,
				Monorail:         monorailCfgs,
				Policies: []*configpb.BugManagementPolicy{
					policyA,
					policyB,
					policyC,
				},
			},
		}

		bm, err := NewBugManager(cl, "https://luci-analysis-test.appspot.com", "luciproject", projectCfg)
		So(err, ShouldBeNil)
		now := time.Date(2040, time.January, 1, 2, 3, 4, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("Create", func() {
			createRequest := NewCreateRequest()
			createRequest.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {}, // P4
			}

			expectedIssue := &mpb.Issue{
				Name:             "projects/chromium/issues/100",
				Summary:          "Tests are failing: ClusterID",
				Reporter:         AutomationUsers[0],
				State:            mpb.IssueContentState_ACTIVE,
				Status:           &mpb.Issue_StatusValue{Status: "Untriaged"},
				StatusModifyTime: timestamppb.New(now),
				FieldValues: []*mpb.FieldValue{
					{
						// Type field.
						Field: "projects/chromium/fieldDefs/10",
						Value: "Bug",
					},
					{
						// Priority field.
						Field: "projects/chromium/fieldDefs/11",
						// Monorail projects do not support priority level P4,
						// so we use P3 for policies that require P4.
						Value: "3",
					},
				},
				Components: []*mpb.Issue_ComponentValue{
					{Component: "projects/chromium/componentDefs/Blink>Layout"},
					{Component: "projects/chromium/componentDefs/Blink>Network"},
				},
				Labels: []*mpb.Issue_LabelValue{{
					Label: "LUCI-Analysis-Auto-Filed",
				}, {
					Label: "Policy-A-Label",
				}, {
					Label: "Restrict-View-Google",
				}},
			}

			Convey("With reason-based failure cluster", func() {
				reason := `Expected equality of these values:
					"Expected_Value"
					my_expr.evaluate(123)
						Which is: "Unexpected_Value"`
				createRequest.Description.Title = reason
				createRequest.Description.Description = "A cluster of failures has been found with reason: " + reason

				expectedIssue.Summary = "Tests are failing: Expected equality of these values: \"Expected_Value\" my_expr.evaluate(123) Which is: \"Unexpected_Value\""
				expectedComment := "A cluster of failures has been found with reason: Expected equality " +
					"of these values:\n\t\t\t\t\t\"Expected_Value\"\n\t\t\t\t\tmy_expr.evaluate(123)\n\t\t\t\t\t\t" +
					"Which is: \"Unexpected_Value\"\n" +
					"\n" +
					"These test failures are causing problem(s) which require your attention, including:\n" +
					"- Problem A\n" +
					"\n" +
					"See current problems, failure examples and more in LUCI Analysis at: https://luci-analysis-test.appspot.com/p/luciproject/rules/new-rule-id\n" +
					"\n" +
					"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
					"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
					"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection"

				Convey("Base case", func() {
					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 2)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
				})
				Convey("Policy with comment template", func() {
					policyA.BugTemplate.CommentTemplate = "RuleURL:{{.RuleURL}},BugID:{{if .BugID.IsMonorail}}{{.BugID.MonorailProject}}/{{.BugID.MonorailBugID}}{{end}}"

					bm, err := NewBugManager(cl, "https://luci-analysis-test.appspot.com", "luciproject", projectCfg)
					So(err, ShouldBeNil)

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 2)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					So(issue.Comments[1].Content, ShouldEqual, "RuleURL:https://luci-analysis-test.appspot.com/p/luciproject/rules/new-rule-id,BugID:chromium/100\n\n"+
						"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: policy-a)")
					So(issue.NotifyCount, ShouldEqual, 2)
				})
				Convey("Policy has no comment template", func() {
					policyA.BugTemplate.CommentTemplate = ""

					bm, err := NewBugManager(cl, "https://luci-analysis-test.appspot.com", "luciproject", projectCfg)
					So(err, ShouldBeNil)

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 2)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					So(issue.Comments[1].Content, ShouldBeEmpty)
					So(issue.NotifyCount, ShouldEqual, 1)
				})
				Convey("Policy has no comment template or labels", func() {
					policyA.BugTemplate.CommentTemplate = ""
					policyA.BugTemplate.Monorail = nil

					bm, err := NewBugManager(cl, "https://luci-analysis-test.appspot.com", "luciproject", projectCfg)
					So(err, ShouldBeNil)

					expectedIssue.Labels = removeLabel(expectedIssue.Labels, "Policy-A-Label")

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 1)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					So(issue.NotifyCount, ShouldEqual, 1)
				})
				Convey("Policy has no monorail template", func() {
					policyA.BugTemplate.Monorail = nil

					bm, err := NewBugManager(cl, "https://luci-analysis-test.appspot.com", "luciproject", projectCfg)
					So(err, ShouldBeNil)

					expectedIssue.Labels = removeLabel(expectedIssue.Labels, "Policy-A-Label")

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
				})
				Convey("Multiple policies activated", func() {
					createRequest.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
						"policy-a": {}, // P4
						"policy-b": {}, // P0
						"policy-c": {}, // P1
					}
					expectedIssue.FieldValues[1].Value = "0"
					expectedIssue.Labels = addLabel(expectedIssue.Labels, "Policy-B-Label")
					expectedIssue.Labels = addLabel(expectedIssue.Labels, "Policy-C-Label")
					expectedComment = strings.Replace(expectedComment, "- Problem A\n", "- Problem B\n- Problem C\n- Problem A\n", 1)

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
							"policy-a": {},
							"policy-b": {},
							"policy-c": {},
						},
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 4)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					// Policy activation comments should appear in descending priority order.
					So(issue.Comments[1].Content, ShouldStartWith, "Policy ID: policy-b")
					So(issue.Comments[2].Content, ShouldStartWith, "Policy ID: policy-c")
					So(issue.Comments[3].Content, ShouldStartWith, "Policy ID: policy-a")
					So(issue.NotifyCount, ShouldEqual, 4)
				})
				Convey("Failed to post issue comment", func() {
					f.UpdateError = status.Errorf(codes.Internal, "internal server error")

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					// Both ID and Error set, reflecting partial success.
					So(response.ID, ShouldEqual, "chromium/100")
					So(response.Error, ShouldNotBeNil)
					So(errors.Is(response.Error, f.UpdateError), ShouldBeTrue)
					So(response.Simulated, ShouldBeFalse)
					So(len(f.Issues), ShouldEqual, 1)

					// Do not expect label to be populated as policy activation comment
					// did not get a chance to post.
					expectedIssue.Labels = removeLabel(expectedIssue.Labels, "Policy-A-Label")

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 1)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					So(issue.NotifyCount, ShouldEqual, 1)
				})
			})
			Convey("With test name failure cluster", func() {
				createRequest.Description.Title = "ninja://:blink_web_tests/media/my-suite/my-test.html"
				createRequest.Description.Description = "A test is failing " + createRequest.Description.Title

				expectedIssue.Summary = "Tests are failing: ninja://:blink_web_tests/media/my-suite/my-test.html"
				expectedComment := "A test is failing ninja://:blink_web_tests/media/my-suite/my-test.html\n" +
					"\n" +
					"These test failures are causing problem(s) which require your attention, including:\n" +
					"- Problem A\n" +
					"\n" +
					"See current problems, failure examples and more in LUCI Analysis at: https://luci-analysis-test.appspot.com/p/luciproject/rules/new-rule-id\n" +
					"\n" +
					"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
					"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
					"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection"

				// Act
				response := bm.Create(ctx, createRequest)

				// Verify
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "chromium/100",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(f.Issues), ShouldEqual, 1)

				issue := f.Issues[0]
				So(issue.Issue, ShouldResembleProto, expectedIssue)
				So(len(issue.Comments), ShouldEqual, 2)
				So(issue.Comments[0].Content, ShouldEqual, expectedComment)
				So(issue.Comments[1].Content, ShouldStartWith, "Policy ID: policy-a")
				So(issue.NotifyCount, ShouldEqual, 2)
			})
			Convey("Without Restrict-View-Google", func() {
				monorailCfgs.FileWithoutRestrictViewGoogle = true

				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "chromium/100",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(f.Issues), ShouldEqual, 1)
				issue := f.Issues[0]

				expectedIssue.Labels = removeLabel(expectedIssue.Labels, "Restrict-View-Google")
				So(issue.Issue, ShouldResembleProto, expectedIssue)
			})
			Convey("Does nothing if in simulation mode", func() {
				bm.Simulate = true

				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					Simulated: true,
					ID:        "chromium/12345678",
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
						"policy-a": {},
					},
				})
				So(len(f.Issues), ShouldEqual, 0)
			})
		})
		Convey("Update", func() {
			c := NewCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {}, // P4
				"policy-c": {}, // P1
			}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
					"policy-c": {},
				},
			})
			So(len(f.Issues), ShouldEqual, 1)
			So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "1")
			So(f.Issues[0].Comments, ShouldHaveLength, 3)

			originalCommentCount := len(f.Issues[0].Comments)

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
					Bug:                              bugs.BugID{System: bugs.MonorailSystem, ID: response.ID},
					BugManagementState:               state,
					IsManagingBug:                    true,
					RuleID:                           "rule-id",
					IsManagingBugPriority:            true,
					IsManagingBugPriorityLastUpdated: tc.Now(),
				},
			}
			expectedResponse := []bugs.BugUpdateResponse{
				{
					IsDuplicate:               false,
					PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
				},
			}
			verifyUpdateDoesNothing := func() error {
				originalIssues := CopyIssuesStore(f)
				response, err := bm.Update(ctx, bugsToUpdate)
				if err != nil {
					return errors.Annotate(err, "update bugs").Err()
				}
				if diff := ShouldResemble(response, expectedResponse); diff != "" {
					return errors.Reason("response: %s", diff).Err()
				}
				if diff := ShouldResembleProto(f, originalIssues); diff != "" {
					return errors.Reason("issues store: %s", diff).Err()
				}
				return nil
			}
			// Create a monorail client that interacts with monorail
			// as an end-user. This is needed as we distinguish user
			// updates to the bug from system updates.
			user := "users/100"
			usercl, err := NewClient(UseFakeIssuesClient(ctx, f, user), "myhost")
			So(err, ShouldBeNil)

			Convey("If active policies unchanged and rule association is not pending, does nothing", func() {
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
				So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
				So(f.Issues[0].Comments[originalCommentCount].Content, ShouldEqual,
					"This bug has been associated with failures in LUCI Analysis. "+
						"To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")
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

					// Activates policy B (P0) for the first time.
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{
						"policy-b": {},
					}

					originalNotifyCount := f.Issues[0].NotifyCount

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)

					// Priority was NOT raised to P0, because IsManagingBug is false.
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldNotEqual, "0")

					// Expect the policy B activation comment to appear.
					So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldStartWith,
						"Policy ID: policy-b")

					// Expect the policy B label to appear.
					So(containsLabel(f.Issues[0].Issue.Labels, "Policy-B-Label"), ShouldBeTrue)

					// The policy activation was notified.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount+1)

					// Verify repeated update has no effect.
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true
					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Reduces priority in response to policies de-activating", func() {
					originalNotifyCount := f.Issues[0].NotifyCount

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")
					So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
						"Because the following problem(s) have stopped:\n"+
							"- Problem C (P1)\n"+
							"The bug priority has been decreased from P1 to P3.")
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

					// Does not notify.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount)

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Increases priority in response to priority policies activating", func() {
					// Activates policy B (P0).
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{
						"policy-b": {},
					}

					originalNotifyCount := f.Issues[0].NotifyCount

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")
					So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+2)
					// Expect the policy B activation comment to appear, followed by the priority update comment.
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldStartWith,
						"Policy ID: policy-b")
					So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
						"Because the following problem(s) have started:\n"+
							"- Problem B (P0)\n"+
							"The bug priority has been increased from P1 to P0.")
					So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

					// Notified the policy activation, and the priority increase.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount+2)

					// Verify repeated update has no effect.
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true
					expectedResponse[0].PolicyActivationsNotified = map[bugs.PolicyID]struct{}{}
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Does not adjust priority if priority manually set", func() {
					updateReq := updateBugPriorityRequest(f.Issues[0].Issue.Name, "0")
					err = usercl.ModifyIssues(ctx, updateReq)
					So(err, ShouldBeNil)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

					expectedIssue := CopyIssue(f.Issues[0].Issue)
					originalNotifyCount := f.Issues[0].NotifyCount
					originalCommentCount = len(f.Issues[0].Comments)

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					expectedResponse[0].DisableRulePriorityUpdates = true
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue, ShouldResembleProto, expectedIssue)
					So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldEqual,
						"The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis,"+
							" enable the update priority flag on the rule.\n\nSee failure impact and configure the failure"+
							" association rule for this bug at: https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")
					// Does not notify.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount)

					// Normally, the caller would update IsManagingBugPriority to false
					// now, but as this is a test, we have to do it manually.
					// As priority updates are now off, DisableRulePriorityUpdates
					// should henceforth also return false (as they are already
					// disabled).
					bugsToUpdate[0].IsManagingBugPriority = false
					bugsToUpdate[0].IsManagingBugPriorityLastUpdated = tc.Now().Add(1 * time.Minute)
					expectedResponse[0].DisableRulePriorityUpdates = false

					// Check repeated update does nothing more.
					So(verifyUpdateDoesNothing(), ShouldBeNil)

					Convey("Unless IsManagingBugPriority manually updated", func() {
						bugsToUpdate[0].IsManagingBugPriority = true
						bugsToUpdate[0].IsManagingBugPriorityLastUpdated = tc.Now().Add(3 * time.Minute)

						response, err := bm.Update(ctx, bugsToUpdate)
						So(response, ShouldResemble, expectedResponse)
						So(err, ShouldBeNil)
						So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")
						So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+2)
						So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
							"Because the following problem(s) are active:\n"+
								"- Problem A (P3)\n"+
								"\n"+
								"The bug priority has been set to P3.")
						So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/help#priority-update")
						So(f.Issues[0].Comments[originalCommentCount+1].Content, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

						// Verify repeated update has no effect.
						So(verifyUpdateDoesNothing(), ShouldBeNil)
					})
				})
				Convey("Does nothing if in simulation mode", func() {
					// In simulation mode, changes should be logged but not made.
					bm.Simulate = true
					So(verifyUpdateDoesNothing(), ShouldBeNil)
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
				Convey("Update closes bug", func() {
					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue.Status.Status, ShouldEqual, VerifiedStatus)

					expectedComment := "Because the following problem(s) have stopped:\n" +
						"- Problem C (P1)\n" +
						"- Problem A (P3)\n" +
						"The bug has been verified."
					So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring, expectedComment)
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#bug-verified")
					So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

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
						originalIssues := CopyIssuesStore(f)

						// Act
						response, err := bm.Update(ctx, bugsToUpdate)

						// Verify
						So(err, ShouldBeNil)
						So(response, ShouldResemble, expectedResponse)
						So(f, ShouldResembleProto, originalIssues)
					})

					Convey("If impact increases, bug is re-opened with correct priority", func() {
						// policy-b has priority P0.
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(2 * time.Hour))
						bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].ActivationNotified = true

						Convey("Issue has owner", func() {
							// Update issue owner.
							updateReq := updateOwnerRequest(f.Issues[0].Issue.Name, "users/100")
							err = usercl.ModifyIssues(ctx, updateReq)
							So(err, ShouldBeNil)
							So(f.Issues[0].Issue.Owner.GetUser(), ShouldEqual, "users/100")
							originalCommentCount = len(f.Issues[0].Comments)

							// Issue should return to "Assigned" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(f.Issues[0].Issue.Status.Status, ShouldEqual, AssignedStatus)
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

							// Verify repeated update has no effect.
							So(verifyUpdateDoesNothing(), ShouldBeNil)
						})
						Convey("Issue has no owner", func() {
							// Remove owner.
							updateReq := updateOwnerRequest(f.Issues[0].Issue.Name, "")
							err = usercl.ModifyIssues(ctx, updateReq)
							So(err, ShouldBeNil)
							So(f.Issues[0].Issue.Owner.GetUser(), ShouldEqual, "")
							originalCommentCount = len(f.Issues[0].Comments)

							// Issue should return to "Untriaged" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(f.Issues[0].Issue.Status.Status, ShouldEqual, UntriagedStatus)
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#priority-update")
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/p/luciproject/rules/rule-id")

							// Verify repeated update has no effect.
							So(verifyUpdateDoesNothing(), ShouldBeNil)
						})
					})
				})
			})
			Convey("If bug duplicate", func() {
				f.Issues[0].Issue.Status.Status = DuplicateStatus
				expectedResponse := []bugs.BugUpdateResponse{
					{
						IsDuplicate:               true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}
				Convey("Issue has no assignee", func() {
					f.Issues[0].Issue.Owner = nil

					// As there is no assignee.
					expectedResponse[0].IsDuplicateAndAssigned = false

					originalIssues := CopyIssuesStore(f)

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(f, ShouldResembleProto, originalIssues)
				})
				Convey("Issue has owner", func() {
					f.Issues[0].Issue.Owner = &mpb.Issue_UserValue{
						User: "users/100",
					}

					// As we have an assignee.
					expectedResponse[0].IsDuplicateAndAssigned = true

					originalIssues := CopyIssuesStore(f)

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(f, ShouldResembleProto, originalIssues)
				})
			})
			Convey("Rule not managing a bug archived after 30 days of the bug being in any closed state", func() {
				tc.Add(time.Hour * 24 * 30)

				bugsToUpdate[0].IsManagingBug = false
				f.Issues[0].Issue.Status.Status = FixedStatus

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}
				originalIssues := CopyIssuesStore(f)

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleProto, originalIssues)
			})
			Convey("Rule managing a bug not archived after 30 days of the bug being in fixed state", func() {
				tc.Add(time.Hour * 24 * 30)

				// If LUCI Analysis is mangaging the bug state, the fixed state
				// means the bug is still not verified. Do not archive the
				// rule.
				bugsToUpdate[0].IsManagingBug = true
				f.Issues[0].Issue.Status.Status = FixedStatus

				So(verifyUpdateDoesNothing(), ShouldBeNil)
			})
			Convey("Rules archived immediately if bug archived", func() {
				f.Issues[0].Issue.Status.Status = "Archived"

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive:             true,
						PolicyActivationsNotified: map[bugs.PolicyID]struct{}{},
					},
				}
				originalIssues := CopyIssuesStore(f)

				// Act
				response, err := bm.Update(ctx, bugsToUpdate)

				// Verify
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleProto, originalIssues)
			})
			Convey("If issue does not exist, does nothing", func() {
				f.Issues = nil
				So(verifyUpdateDoesNothing(), ShouldBeNil)
			})
		})
		Convey("GetMergedInto", func() {
			c := NewCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {},
			}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			})
			So(len(f.Issues), ShouldEqual, 1)

			bugID := bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"}
			Convey("Merged into monorail bug", func() {
				f.Issues[0].Issue.Status.Status = DuplicateStatus
				f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
					Issue: "projects/testproject/issues/99",
				}

				result, err := bm.GetMergedInto(ctx, bugID)
				So(err, ShouldEqual, nil)
				So(result, ShouldResemble, &bugs.BugID{
					System: bugs.MonorailSystem,
					ID:     "testproject/99",
				})
			})
			Convey("Merged into buganizer bug", func() {
				f.Issues[0].Issue.Status.Status = DuplicateStatus
				f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
					ExtIdentifier: "b/1234",
				}

				result, err := bm.GetMergedInto(ctx, bugID)
				So(err, ShouldEqual, nil)
				So(result, ShouldResemble, &bugs.BugID{
					System: bugs.BuganizerSystem,
					ID:     "1234",
				})
			})
			Convey("Not merged into any bug", func() {
				// While MergedIntoIssueRef is set, the bug status is not
				// set to "Duplicate", so this value should be ignored.
				f.Issues[0].Issue.Status.Status = UntriagedStatus
				f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
					ExtIdentifier: "b/1234",
				}

				result, err := bm.GetMergedInto(ctx, bugID)
				So(err, ShouldEqual, nil)
				So(result, ShouldBeNil)
			})
		})
		Convey("UpdateDuplicateSource", func() {
			c := NewCreateRequest()
			c.ActivePolicyIDs = map[bugs.PolicyID]struct{}{
				"policy-a": {},
			}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
				PolicyActivationsNotified: map[bugs.PolicyID]struct{}{
					"policy-a": {},
				},
			})
			So(f.Issues, ShouldHaveLength, 1)
			So(f.Issues[0].Comments, ShouldHaveLength, 2)
			originalCommentCount := len(f.Issues[0].Comments)

			f.Issues[0].Issue.Status.Status = DuplicateStatus
			f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
				Issue: "projects/testproject/issues/99",
			}

			Convey("With ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-rule-id",
						Bug:    bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"},
					},
					ErrorMessage: "Some error.",
				}
				err = bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(f.Issues[0].Issue.Status.Status, ShouldNotEqual, DuplicateStatus)
				So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
				So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring, "Some error.")
				So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/luciproject/rules/source-rule-id")
			})
			Convey("Without ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						RuleID: "source-rule-id",
						Bug:    bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"},
					},
					DestinationRuleID: "12345abcdef",
				}
				err = bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(f.Issues[0].Issue.Status.Status, ShouldEqual, DuplicateStatus)
				So(f.Issues[0].Comments, ShouldHaveLength, originalCommentCount+1)
				So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring, "merged the failure association rule for this bug into the rule for the canonical bug.")
				So(f.Issues[0].Comments[originalCommentCount].Content, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/luciproject/rules/12345abcdef")
			})
		})
	})
}

func updateOwnerRequest(name string, owner string) *mpb.ModifyIssuesRequest {
	return &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			{
				Issue: &mpb.Issue{
					Name: name,
					Owner: &mpb.Issue_UserValue{
						User: owner,
					},
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"owner"},
				},
			},
		},
		CommentContent: "User comment.",
	}
}

func updateBugPriorityRequest(name string, priority string) *mpb.ModifyIssuesRequest {
	return &mpb.ModifyIssuesRequest{
		Deltas: []*mpb.IssueDelta{
			{
				Issue: &mpb.Issue{
					Name: name,
					FieldValues: []*mpb.FieldValue{
						{
							Field: "projects/chromium/fieldDefs/11",
							Value: priority,
						},
					},
				},
				UpdateMask: &field_mask.FieldMask{
					Paths: []string{"field_values"},
				},
			},
		},
		CommentContent: "User comment.",
	}
}

func addLabel(labels []*mpb.Issue_LabelValue, label string) []*mpb.Issue_LabelValue {
	result := append(labels, &mpb.Issue_LabelValue{Label: label})
	SortLabels(result)
	return result
}

func removeLabel(labels []*mpb.Issue_LabelValue, label string) []*mpb.Issue_LabelValue {
	var result []*mpb.Issue_LabelValue
	for _, item := range labels {
		if item.Label != label {
			result = append(result, item)
		}
	}
	SortLabels(result)
	return result
}

func containsLabel(labels []*mpb.Issue_LabelValue, label string) bool {
	for _, item := range labels {
		if item.Label == label {
			return true
		}
	}
	return false
}
