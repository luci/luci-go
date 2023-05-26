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
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/bugs"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
)

func NewCreateRequest() *bugs.CreateRequest {
	cluster := &bugs.CreateRequest{
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
		projectCfg := &configpb.ProjectConfig{
			Monorail:            monorailCfgs,
			BugFilingThresholds: bugFilingThreshold,
			BugSystem:           configpb.ProjectConfig_MONORAIL,
		}
		bm, err := NewBugManager(cl, "luci-analysis-test", "luciproject", projectCfg)
		So(err, ShouldBeNil)

		now := time.Date(2040, time.January, 1, 2, 3, 4, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("Create", func() {
			createRequest := NewCreateRequest()
			createRequest.Impact = bugs.LowP1Impact()

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
						Value: "1",
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

				bug, err := bm.Create(ctx, createRequest)
				So(err, ShouldBeNil)
				So(bug, ShouldEqual, "chromium/100")
				So(len(f.Issues), ShouldEqual, 1)
				issue := f.Issues[0]

				expectedIssue.Summary = "Tests are failing: Expected equality of these values: \"Expected_Value\" my_expr.evaluate(123) Which is: \"Unexpected_Value\""

				So(issue.Issue, ShouldResembleProto, expectedIssue)
				So(len(issue.Comments), ShouldEqual, 2)
				So(issue.Comments[0].Content, ShouldContainSubstring, reason)
				So(issue.Comments[0].Content, ShouldNotContainSubstring, "ClusterIDShouldNotAppearInOutput")
				// Links to help should appear in the issue description.
				So(issue.Comments[0].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/help#new-bug-filed")
				So(issue.Comments[0].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/help#feedback")
				// Link to cluster page should appear in output.
				So(issue.Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
				So(issue.NotifyCount, ShouldEqual, 1)
			})
			Convey("With test name failure cluster", func() {
				createRequest.Description.Title = "ninja://:blink_web_tests/media/my-suite/my-test.html"
				createRequest.Description.Description = "A test is failing " + createRequest.Description.Title

				bug, err := bm.Create(ctx, createRequest)
				So(err, ShouldBeNil)
				So(bug, ShouldEqual, "chromium/100")
				So(len(f.Issues), ShouldEqual, 1)
				issue := f.Issues[0]

				expectedIssue.Summary = "Tests are failing: ninja://:blink_web_tests/media/my-suite/my-test.html"
				So(issue.Issue, ShouldResembleProto, expectedIssue)
				So(len(issue.Comments), ShouldEqual, 2)
				So(issue.Comments[0].Content, ShouldContainSubstring, "ninja://:blink_web_tests/media/my-suite/my-test.html")
				// Links to help should appear in the issue description.
				So(issue.Comments[0].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/help#new-bug-filed")
				So(issue.Comments[0].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/help#feedback")
				// Link to cluster page should appear in output.
				So(issue.Comments[1].Content, ShouldContainSubstring, "https://luci-analysis-test.appspot.com/b/chromium/100")
				So(issue.NotifyCount, ShouldEqual, 1)
			})
			Convey("Without Restrict-View-Google", func() {
				monorailCfgs.FileWithoutRestrictViewGoogle = true

				bug, err := bm.Create(ctx, createRequest)
				So(err, ShouldBeNil)
				So(bug, ShouldEqual, "chromium/100")
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
				_, err := bm.Create(ctx, createRequest)
				So(err, ShouldEqual, bugs.ErrCreateSimulated)
				So(len(f.Issues), ShouldEqual, 0)
			})
		})
		Convey("Update", func() {
			c := NewCreateRequest()
			c.Impact = bugs.P2Impact()
			bug, err := bm.Create(ctx, c)
			So(err, ShouldBeNil)
			So(bug, ShouldEqual, "chromium/100")
			So(len(f.Issues), ShouldEqual, 1)
			So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "2")

			bugsToUpdate := []bugs.BugUpdateRequest{
				{
					Bug:                              bugs.BugID{System: bugs.MonorailSystem, ID: bug},
					Impact:                           c.Impact,
					IsManagingBug:                    true,
					IsManagingBugPriority:            true,
					IsManagingBugPriorityLastUpdated: tc.Now(),
				},
			}
			expectedResponse := []bugs.BugUpdateResponse{
				{IsDuplicate: false},
			}
			updateDoesNothing := func() {
				originalIssues := CopyIssuesStore(f)
				response, err := bm.Update(ctx, bugsToUpdate)
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleIssuesStore, originalIssues)
			}
			// Create a monorail client that interacts with monorail
			// as an end-user. This is needed as we distinguish user
			// updates to the bug from system updates.
			user := "users/100"
			usercl, err := NewClient(UseFakeIssuesClient(ctx, f, user), "myhost")
			So(err, ShouldBeNil)

			Convey("If impact unchanged, does nothing", func() {
				updateDoesNothing()
			})
			Convey("If impact changed", func() {
				bugsToUpdate[0].Impact = bugs.P3Impact()
				Convey("Does not reduce priority if impact within hysteresis range", func() {
					bugsToUpdate[0].Impact = bugs.HighP3Impact()

					updateDoesNothing()
				})
				Convey("Does not update bug if IsManagingBug false", func() {
					bugsToUpdate[0].Impact = bugs.ClosureImpact()
					bugsToUpdate[0].IsManagingBug = false

					updateDoesNothing()
				})
				Convey("Does not update bug if Impact unset", func() {
					// Simulate valid impact not being available, e.g. due
					// to ongoing reclustering.
					bugsToUpdate[0].Impact = nil

					updateDoesNothing()
				})
				Convey("Reduces priority in response to reduced impact", func() {
					bugsToUpdate[0].Impact = bugs.P3Impact()
					originalNotifyCount := f.Issues[0].NotifyCount
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")

					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"Because:\n"+
							"- Test Runs Failed (1-day) < 9, and\n"+
							"- Test Results Failed (1-day) < 90\n"+
							"LUCI Analysis has decreased the bug priority from 2 to 3.")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Does not notify.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount)

					// Verify repeated update has no effect.
					updateDoesNothing()
				})
				Convey("Does not increase priority if impact within hysteresis range", func() {
					bugsToUpdate[0].Impact = bugs.LowP1Impact()

					updateDoesNothing()
				})
				Convey("Increases priority in response to increased impact (single-step)", func() {
					bugsToUpdate[0].Impact = bugs.P1Impact()

					originalNotifyCount := f.Issues[0].NotifyCount
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "1")

					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"Because:\n"+
							"- Test Results Failed (1-day) >= 550\n"+
							"LUCI Analysis has increased the bug priority from 2 to 1.")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Notified the increase.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount+1)

					// Verify repeated update has no effect.
					updateDoesNothing()
				})
				Convey("Increases priority in response to increased impact (multi-step)", func() {
					bugsToUpdate[0].Impact = bugs.P0Impact()

					originalNotifyCount := f.Issues[0].NotifyCount
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")

					expectedComment := "Because:\n" +
						"- Test Results Failed (1-day) >= 1000\n" +
						"LUCI Analysis has increased the bug priority from 2 to 0."
					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, expectedComment)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#priority-update")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Notified the increase.
					So(f.Issues[0].NotifyCount, ShouldEqual, originalNotifyCount+1)

					// Verify repeated update has no effect.
					updateDoesNothing()
				})
				Convey("Does not adjust priority if priority manually set", func() {
					updateReq := updateBugPriorityRequest(f.Issues[0].Issue.Name, "0")
					err = usercl.ModifyIssues(ctx, updateReq)
					So(err, ShouldBeNil)
					So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "0")
					// Check the update sets the label.
					expectedIssue := CopyIssue(f.Issues[0].Issue)
					SortLabels(expectedIssue.Labels)
					So(f.Issues[0].NotifyCount, ShouldEqual, 1)
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					expectedResponse[0].DisableRulePriorityUpdates = true
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue, ShouldResembleProto, expectedIssue)

					// Does not notify.
					So(f.Issues[0].NotifyCount, ShouldEqual, 1)

					// Priority updates should be off, but disabling it should return false as we should not check it anymore.
					bugsToUpdate[0].IsManagingBugPriority = false
					expectedResponse[0].DisableRulePriorityUpdates = false
					// Check repeated update does nothing more.
					updateDoesNothing()

					Convey("Unless update priority flag is re-enabled", func() {
						bugsToUpdate[0].IsManagingBugPriority = true
						bugsToUpdate[0].IsManagingBugPriorityLastUpdated = tc.Now().Add(3 * time.Minute)
						response, err := bm.Update(ctx, bugsToUpdate)
						So(response, ShouldResemble, expectedResponse)
						So(err, ShouldBeNil)
						So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")

						// Verify repeated update has no effect.
						updateDoesNothing()
					})
				})
				Convey("Does nothing if in simulation mode", func() {
					bm.Simulate = true
					updateDoesNothing()
				})
			})
			Convey("If impact falls below lowest priority threshold", func() {
				bugsToUpdate[0].Impact = bugs.ClosureImpact()
				Convey("Update leaves bug open if impact within hysteresis range", func() {
					bugsToUpdate[0].Impact = bugs.P3LowestBeforeClosureImpact()

					// Update may reduce the priority from P2 to P3, but the
					// issue should be left open. This is because hysteresis on
					// priority and issue verified state is applied separately.
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue.Status.Status, ShouldEqual, UntriagedStatus)
				})
				Convey("Update closes bug", func() {
					response, err := bm.Update(ctx, bugsToUpdate)
					So(err, ShouldBeNil)
					So(response, ShouldResemble, expectedResponse)
					So(f.Issues[0].Issue.Status.Status, ShouldEqual, VerifiedStatus)

					expectedComment := "Because:\n" +
						"- Test Results Failed (1-day) < 45, and\n" +
						"- Test Results Failed (3-day) < 272, and\n" +
						"- Test Results Failed (7-day) < 1\n" +
						"LUCI Analysis is marking the issue verified."
					So(f.Issues[0].Comments, ShouldHaveLength, 3)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring, expectedComment)
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/help#bug-verified")
					So(f.Issues[0].Comments[2].Content, ShouldContainSubstring,
						"https://luci-analysis-test.appspot.com/b/chromium/100")

					// Verify repeated update has no effect.
					updateDoesNothing()

					Convey("Does not reopen bug if impact within hysteresis range", func() {
						bugsToUpdate[0].Impact = bugs.HighestNotFiledImpact()

						updateDoesNothing()
					})

					Convey("Rules for verified bugs archived after 30 days", func() {
						tc.Add(time.Hour * 24 * 30)

						expectedResponse := []bugs.BugUpdateResponse{
							{
								ShouldArchive: true,
							},
						}
						originalIssues := CopyIssuesStore(f)
						response, err := bm.Update(ctx, bugsToUpdate)
						So(err, ShouldBeNil)
						So(response, ShouldResemble, expectedResponse)
						So(f, ShouldResembleIssuesStore, originalIssues)
					})

					Convey("If impact increases, bug is re-opened with correct priority", func() {
						bugsToUpdate[0].Impact = bugs.P3Impact()
						Convey("Issue has owner", func() {
							// Update issue owner.
							updateReq := updateOwnerRequest(f.Issues[0].Issue.Name, "users/100")
							err = usercl.ModifyIssues(ctx, updateReq)
							So(err, ShouldBeNil)
							So(f.Issues[0].Issue.Owner.GetUser(), ShouldEqual, "users/100")
							expectedResponse[0].IsAssigned = true

							// Issue should return to "Assigned" status.
							response, err := bm.Update(ctx, bugsToUpdate)
							So(err, ShouldBeNil)
							So(response, ShouldResemble, expectedResponse)
							So(f.Issues[0].Issue.Status.Status, ShouldEqual, AssignedStatus)
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")

							expectedComment := "Because:\n" +
								"- Test Results Failed (1-day) >= 75\n" +
								"LUCI Analysis has re-opened the bug.\n\n" +
								"Because:\n" +
								"- Test Runs Failed (1-day) < 9, and\n" +
								"- Test Results Failed (1-day) < 90\n" +
								"LUCI Analysis has decreased the bug priority from 2 to 3."
							So(f.Issues[0].Comments, ShouldHaveLength, 5)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/b/chromium/100")

							// Verify repeated update has no effect.
							updateDoesNothing()
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
							So(ChromiumTestIssuePriority(f.Issues[0].Issue), ShouldEqual, "3")

							expectedComment := "Because:\n" +
								"- Test Results Failed (1-day) >= 75\n" +
								"LUCI Analysis has re-opened the bug.\n\n" +
								"Because:\n" +
								"- Test Runs Failed (1-day) < 9, and\n" +
								"- Test Results Failed (1-day) < 90\n" +
								"LUCI Analysis has decreased the bug priority from 2 to 3."
							So(f.Issues[0].Comments, ShouldHaveLength, 5)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring, expectedComment)
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#priority-update")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/help#bug-reopened")
							So(f.Issues[0].Comments[4].Content, ShouldContainSubstring,
								"https://luci-analysis-test.appspot.com/b/chromium/100")

							// Verify repeated update has no effect.
							updateDoesNothing()
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
				response, err := bm.Update(ctx, bugsToUpdate)
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleIssuesStore, originalIssues)
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
				response, err := bm.Update(ctx, bugsToUpdate)
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleIssuesStore, originalIssues)
			})
			Convey("Rule managing a bug not archived after 30 days of the bug being in fixed state", func() {
				tc.Add(time.Hour * 24 * 30)

				// If LUCI Analysis is mangaging the bug state, the fixed state
				// means the bug is still not verified. Do not archive the
				// rule.
				bugsToUpdate[0].IsManagingBug = true
				f.Issues[0].Issue.Status.Status = FixedStatus

				updateDoesNothing()
			})
			Convey("Rules archived immediately if bug archived", func() {
				f.Issues[0].Issue.Status.Status = "Archived"

				expectedResponse := []bugs.BugUpdateResponse{
					{
						ShouldArchive: true,
					},
				}
				originalIssues := CopyIssuesStore(f)
				response, err := bm.Update(ctx, bugsToUpdate)
				So(err, ShouldBeNil)
				So(response, ShouldResemble, expectedResponse)
				So(f, ShouldResembleIssuesStore, originalIssues)
			})
			Convey("If issue does not exist, does nothing", func() {
				f.Issues = nil
				updateDoesNothing()
			})
		})
		Convey("GetMergedInto", func() {
			c := NewCreateRequest()
			c.Impact = bugs.P2Impact()
			bug, err := bm.Create(ctx, c)
			So(err, ShouldBeNil)
			So(bug, ShouldEqual, "chromium/100")
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
			c.Impact = bugs.P2Impact()
			bug, err := bm.Create(ctx, c)
			So(err, ShouldBeNil)
			So(bug, ShouldEqual, "chromium/100")
			So(f.Issues, ShouldHaveLength, 1)
			So(f.Issues[0].Comments, ShouldHaveLength, 2)

			f.Issues[0].Issue.Status.Status = DuplicateStatus
			f.Issues[0].Issue.MergedIntoIssueRef = &mpb.IssueRef{
				Issue: "projects/testproject/issues/99",
			}

			Convey("With ErrorMessage", func() {
				request := bugs.UpdateDuplicateSourceRequest{
					BugDetails: bugs.BugDetails{
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
					BugDetails: bugs.BugDetails{
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
			c.Impact = bugs.P2Impact()
			bug, err := bm.Create(ctx, c)
			So(err, ShouldBeNil)
			So(bug, ShouldEqual, "chromium/100")
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
