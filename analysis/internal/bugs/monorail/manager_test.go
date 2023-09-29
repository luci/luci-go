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

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
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

		bugFilingThreshold := bugs.TestBugFilingThresholds()

		policyA := config.CreatePlaceholderBugManagementPolicy("policy-a")
		policyA.HumanReadableName = "Problem A"
		policyA.Priority = configpb.BuganizerPriority_P4

		policyB := config.CreatePlaceholderBugManagementPolicy("policy-b")
		policyB.HumanReadableName = "Problem B"
		policyB.Priority = configpb.BuganizerPriority_P0

		policyC := config.CreatePlaceholderBugManagementPolicy("policy-c")
		policyC.HumanReadableName = "Problem C"
		policyC.Priority = configpb.BuganizerPriority_P1

		projectCfg := &configpb.ProjectConfig{
			Monorail:            monorailCfgs,
			BugFilingThresholds: bugFilingThreshold,
			BugSystem:           configpb.BugSystem_MONORAIL,
			BugManagement: &configpb.BugManagement{
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
			createRequest.ActivePolicyIDs = map[string]struct{}{
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
					"\n" +
					"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
					"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
					"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection"

				Convey("Single policy activated", func() {
					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 2)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					// Link to cluster page should appear in follow-up comment.
					So(issue.Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
					So(issue.NotifyCount, ShouldEqual, 1)
				})
				Convey("Multiple policies activated", func() {
					createRequest.ActivePolicyIDs = map[string]struct{}{
						"policy-a": {}, // P4
						"policy-b": {}, // P0
						"policy-c": {}, // P1
					}
					expectedIssue.FieldValues[1].Value = "0"
					expectedComment = strings.Replace(expectedComment, "- Problem A\n", "- Problem B\n- Problem C\n- Problem A\n", 1)

					// Act
					response := bm.Create(ctx, createRequest)

					// Verify
					So(response, ShouldResemble, bugs.BugCreateResponse{
						ID: "chromium/100",
					})
					So(len(f.Issues), ShouldEqual, 1)

					issue := f.Issues[0]
					So(issue.Issue, ShouldResembleProto, expectedIssue)
					So(len(issue.Comments), ShouldEqual, 2)
					So(issue.Comments[0].Content, ShouldEqual, expectedComment)
					// Link to cluster page should appear in follow-up comment.
					So(issue.Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
					So(issue.NotifyCount, ShouldEqual, 1)
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
					"\n" +
					"How to action this bug: https://luci-analysis-test.appspot.com/help#new-bug-filed\n" +
					"Provide feedback: https://luci-analysis-test.appspot.com/help#feedback\n" +
					"Was this bug filed in the wrong component? See: https://luci-analysis-test.appspot.com/help#component-selection"

				// Act
				response := bm.Create(ctx, createRequest)

				// Verify
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "chromium/100",
				})
				So(len(f.Issues), ShouldEqual, 1)

				issue := f.Issues[0]
				So(issue.Issue, ShouldResembleProto, expectedIssue)
				So(len(issue.Comments), ShouldEqual, 2)
				So(issue.Comments[0].Content, ShouldEqual, expectedComment)
				// Link to cluster page should appear in follow-up comment.
				So(issue.Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
				So(issue.NotifyCount, ShouldEqual, 1)
			})
			Convey("Without Restrict-View-Google", func() {
				monorailCfgs.FileWithoutRestrictViewGoogle = true

				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID: "chromium/100",
				})
				So(len(f.Issues), ShouldEqual, 1)
				issue := f.Issues[0]

				// No Restrict-View-Google label.
				expectedIssue.Labels = []*mpb.Issue_LabelValue{{
					Label: "LUCI-Analysis-Auto-Filed",
				}}
				So(issue.Issue, ShouldResembleProto, expectedIssue)
			})
			Convey("Does nothing if in simulation mode", func() {
				bm.Simulate = true

				response := bm.Create(ctx, createRequest)
				So(response, ShouldResemble, bugs.BugCreateResponse{
					ID:        "chromium/12345678",
					Simulated: true,
				})
				So(len(f.Issues), ShouldEqual, 0)
			})
		})
		Convey("Update", func() {
			c := NewCreateRequest()
			c.ActivePolicyIDs = map[string]struct{}{
				"policy-a": {}, // P4
				"policy-c": {}, // P1
			}
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
			})
			So(len(f.Issues), ShouldEqual, 1)
			So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "1")

			activationTime := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)
			state := &bugspb.BugManagementState{
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": { // P4
						IsActive:           true,
						LastActivationTime: timestamppb.New(activationTime),
					},
					"policy-b": { // P0
						IsActive:             false,
						LastActivationTime:   timestamppb.New(activationTime.Add(-time.Hour)),
						LastDeactivationTime: timestamppb.New(activationTime),
					},
					"policy-c": { // P1
						IsActive:           true,
						LastActivationTime: timestamppb.New(activationTime),
					},
				},
			}

			bugsToUpdate := []bugs.BugUpdateRequest{
				{
					Bug:                              bugs.BugID{System: bugs.MonorailSystem, ID: response.ID},
					BugManagementState:               state,
					IsManagingBug:                    true,
					RuleID:                           "123",
					IsManagingBugPriority:            true,
					IsManagingBugPriorityLastUpdated: tc.Now(),
				},
			}
			expectedResponse := []bugs.BugUpdateResponse{
				{IsDuplicate: false},
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

			Convey("If active policies unchanged, does nothing", func() {
				So(verifyUpdateDoesNothing(), ShouldBeNil)
			})
			Convey("If active policies changed", func() {
				// De-activates policy-c (P1), leaving only policy-a (P4) active.
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].IsActive = false
				bugsToUpdate[0].BugManagementState.PolicyState["policy-c"].LastDeactivationTime = timestamppb.New(activationTime.Add(time.Hour))

				Convey("Does not update bug if IsManagingBug false", func() {
					bugsToUpdate[0].IsManagingBug = false

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
					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"Because the following problem(s) have stopped:\n"+
							"- Problem C (P1)\n"+
							"The bug priority has been decreased from P1 to P3.")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Does not notify.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount)

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Increases priority in response to priority policies activating", func() {
					// Activates policy B (P0).
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].IsActive = true
					bugsToUpdate[0].BugManagementState.PolicyState["policy-b"].LastActivationTime = timestamppb.New(activationTime.Add(time.Hour))

					originalNotifyCount := f.Issues[0].NotifyCount

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")
					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"Because the following problem(s) have started:\n"+
							"- Problem B (P0)\n"+
							"The bug priority has been increased from P1 to P0.")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Notified the increase.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount+1)

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)
				})
				Convey("Does not adjust priority if priority manually set", func() {
					updateReq := updateBugPriorityRequest(f.Issues[0].Issue.Name, "0")
					err = usercl.ModifyIssues(ctx, updateReq)
					So(err, ShouldBeNil)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

					expectedIssue := CopyIssue(f.Issues[0].Issue)
					// SortLabels(expectedIssue.Labels)
					originalNotifyCount := f.Issues[0].NotifyCount

					// Act
					response, err := bm.Update(ctx, bugsToUpdate)

					// Verify
					So(err, ShouldBeNil)
					expectedResponse[0].DisableRulePriorityUpdates = true
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue, ShouldResembleProto, expectedIssue)
					So(f.Issues[0].Comments, ShouldHaveLength, 4)
					So(f.Issues[0].Comments[3].Content, ShouldEqual,
						"The bug priority has been manually set. To re-enable automatic priority updates by LUCI Analysis,"+
							" enable the update priority flag on the rule.\n\nSee failure impact and configure the failure"+
							" association rule for this bug at: https://luci-analysis-test.appspot.com/b/chromium/100")

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
						So(f.Issues[0].Comments, ShouldHaveLength, 5)
						So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
							"Because the following problem(s) are active:\n"+
								"- Problem A (P3)\n"+
								"\n"+
								"The bug priority has been set to P3.")
						So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/help#priority-update")
						So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
							"https://luci-analysis-test.appspot.com/b/chromium/100")

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
						"- Problem A (P3)\n" +
						"- Problem C (P1)\n" +
						"The bug has been verified."
					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, expectedComment)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#bug-verified")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Verify repeated update has no effect.
					So(verifyUpdateDoesNothing(), ShouldBeNil)

					Convey("Rules for verified bugs archived after 30 days", func() {
						tc.Add(time.Hour * 24 * 30)

						expectedResponse := []bugs.BugUpdateResponse{
							{
								ShouldArchive: true,
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

						Convey("Issue has owner", func() {
							// Update issue owner.
							updateReq := updateOwnerRequest(f.Issues[0].Issue.Name, "users/100")
							err = usercl.ModifyIssues(ctx, updateReq)
							So(err, ShouldBeNil)
							So(f.Issues[0].Issue.Owner.GetUser(), ShouldEqual, "users/100")
							expectedResponse[0].IsDuplicateAndAssigned = true

							// Issue should return to "Assigned" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(f.Issues[0].Issue.Status.Status, ShouldEqual, AssignedStatus)
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(f.Issues[0].Comments, ShouldHaveLength, 5)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/b/chromium/100")

							// Verify repeated update has no effect.
							So(verifyUpdateDoesNothing(), ShouldBeNil)
						})
						Convey("Issue has no owner", func() {
							// Remove owner.
							updateReq := updateOwnerRequest(f.Issues[0].Issue.Name, "")
							err = usercl.ModifyIssues(ctx, updateReq)
							So(err, ShouldBeNil)
							So(f.Issues[0].Issue.Owner.GetUser(), ShouldEqual, "")

							// Issue should return to "Untriaged" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(f.Issues[0].Issue.Status.Status, ShouldEqual, UntriagedStatus)
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

							expectedComment := "Because the following problem(s) have started:\n" +
								"- Problem B (P0)\n" +
								"The bug has been re-opened as P0."
							So(f.Issues[0].Comments, ShouldHaveLength, 5)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#priority-update")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/b/chromium/100")

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
						IsDuplicate: true,
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
			Convey("Rule not managing a bug archived after 30 days of the bug being in any closed state", func() {
				tc.Add(time.Hour * 24 * 30)

				bugsToUpdate[0].IsManagingBug = false
				f.Issues[0].Issue.Status.Status = FixedStatus

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive: true,
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
						ShouldArchive: true,
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
			c.Metrics = bugs.P2Impact()
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
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
			c.Metrics = bugs.P2Impact()
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
			})
			So(f.Issues, ShouldHaveLength, 1)
			So(f.Issues[0].Comments, ShouldHaveLength, 2)

			f.Issues[0].Issue.Status.Status = DuplicateStatus
			f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
				Issue: "projects/testproject/issues/99",
			}

			Convey("With ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						Bug: bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"},
					},
					ErrorMessage: "Some error.",
				}
				err = bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(f.Issues[0].Issue.Status.Status, ShouldNotEqual, DuplicateStatus)
				So(f.Issues[0].Comments, ShouldHaveLength, 3)
				So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, "Some error.")
				So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/b/chromium/100")
			})
			Convey("Without ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.DuplicateBugDetails{
						Bug: bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"},
					},
					DestinationRuleID: "12345abcdef",
				}
				err = bm.UpdateDuplicateSource(ctx, request)
				So(err, ShouldBeNil)

				So(f.Issues[0].Issue.Status.Status, ShouldEqual, DuplicateStatus)
				So(f.Issues[0].Comments, ShouldHaveLength, 3)
				So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, "merged the failure association rule for this bug into the rule for the canonical bug.")
				So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
					"https://luci-analysis-test.appspot.com/p/luciproject/rules/12345abcdef")
			})
		})
		Convey("UpdateDuplicateDestination", func() {
			c := NewCreateRequest()
			c.Metrics = bugs.P2Impact()
			response := bm.Create(ctx, c)
			So(response, ShouldResemble, bugs.BugCreateResponse{
				ID: "chromium/100",
			})
			So(f.Issues, ShouldHaveLength, 1)
			So(f.Issues[0].Comments, ShouldHaveLength, 2)

			bugID := bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"}
			err = bm.UpdateDuplicateDestination(ctx, bugID)
			So(err, ShouldBeNil)

			So(f.Issues[0].Issue.Status, ShouldNotEqual, DuplicateStatus)
			So(f.Issues[0].Comments, ShouldHaveLength, 3)
			So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, "merged the failure association rule for that bug into the rule for this bug.")
			So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
				"https://luci-analysis-test.appspot.com/b/chromium/100")
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
