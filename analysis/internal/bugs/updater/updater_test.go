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

package updater

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	mpb "go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"
)

func TestUpdate(t *testing.T) {
	Convey("With bug updater", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx = context.WithValue(ctx, &buganizer.BuganizerSelfEmailKey, "email@test.com")

		const project = "chromeos"

		// Has two policies:
		// exoneration-policy (P2):
		// - activation threshold: 100 in one day
		// - deactivation threshold: 10 in one day
		// cls-rejected-policy (P1):
		// - activation threshold: 10 in one week
		// - deactivation threshold: 1 in one week
		projectCfg := createProjectConfig()
		projectsCfg := map[string]*configpb.ProjectConfig{
			project: projectCfg,
		}
		err := config.SetTestProjectConfig(ctx, projectsCfg)
		So(err, ShouldBeNil)

		compiledCfg, err := compiledcfg.NewConfig(projectCfg)
		So(err, ShouldBeNil)

		suggestedClusters := []*analysis.Cluster{
			makeReasonCluster(compiledCfg, 0),
			makeReasonCluster(compiledCfg, 1),
			makeReasonCluster(compiledCfg, 2),
			makeReasonCluster(compiledCfg, 3),
			makeReasonCluster(compiledCfg, 4),
		}
		analysisClient := &fakeAnalysisClient{
			clusters: suggestedClusters,
		}

		buganizerClient := buganizer.NewFakeClient()
		buganizerStore := buganizerClient.FakeStore

		monorailStore := &monorail.FakeIssuesStore{
			NextID:            100,
			PriorityFieldName: "projects/chromium/fieldDefs/11",
			ComponentNames: []string{
				"projects/chromium/componentDefs/Blink",
				"projects/chromium/componentDefs/Blink>Layout",
				"projects/chromium/componentDefs/Blink>Network",
			},
		}
		user := monorail.AutomationUsers[0]
		monorailClient, err := monorail.NewClient(monorail.UseFakeIssuesClient(ctx, monorailStore, user), "myhost")
		So(err, ShouldBeNil)

		// Unless otherwise specified, assume re-clustering has caught up to
		// the latest version of algorithms and config.
		err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
			runs.NewRun(0).
				WithProject(project).
				WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
				WithConfigVersion(projectCfg.LastUpdated.AsTime()).
				WithRulesVersion(rules.StartingEpoch).
				WithCompletedProgress().Build(),
		})
		So(err, ShouldBeNil)

		progress, err := runs.ReadReclusteringProgress(ctx, project)
		So(err, ShouldBeNil)

		opts := UpdateOptions{
			UIBaseURL:            "https://luci-analysis-test.appspot.com",
			Project:              project,
			AnalysisClient:       analysisClient,
			BuganizerClient:      buganizerClient,
			MonorailClient:       monorailClient,
			MaxBugsFiledPerRun:   1,
			ReclusteringProgress: progress,
			RunTimestamp:         time.Date(2100, 2, 2, 2, 2, 2, 2, time.UTC),
		}

		// Mock current time. This is needed to control behaviours like
		// automatic archiving of rules after 30 days of bug being marked
		// Closed (Verified).
		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, tc := testclock.UseTime(ctx, now)

		Convey("configuration used for testing is valid", func() {
			c := validation.Context{Context: context.Background()}
			config.ValidateProjectConfig(&c, project, projectCfg)
			So(c.Finalize(), ShouldBeNil)
		})
		Convey("with a suggested cluster", func() {
			// Create a suggested cluster we should consider filing a bug for.
			sourceClusterID := reasonClusterID(compiledCfg, "Failed to connect to 100.1.1.99.")
			suggestedClusters[1].ClusterID = sourceClusterID
			suggestedClusters[1].ExampleFailureReason = bigquery.NullString{StringVal: "Failed to connect to 100.1.1.105.", Valid: true}
			suggestedClusters[1].TopTestIDs = []analysis.TopCount{
				{Value: "network-test-1", Count: 10},
				{Value: "network-test-2", Count: 10},
			}
			// Meets failure dispersion thresholds.
			suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3

			expectedRule := &rules.Entry{
				Project:                 "chromeos",
				RuleDefinition:          `reason LIKE "Failed to connect to %.%.%.%."`,
				BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
				IsActive:                true,
				IsManagingBug:           true,
				IsManagingBugPriority:   true,
				SourceCluster:           sourceClusterID,
				CreateUser:              rules.LUCIAnalysisSystem,
				LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
				BugManagementState: &bugspb.BugManagementState{
					RuleAssociationNotified: true,
					PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
						"exoneration-policy": {
							IsActive:           true,
							LastActivationTime: timestamppb.New(opts.RunTimestamp),
							ActivationNotified: true,
						},
						"cls-rejected-policy": {},
					},
				},
			}
			expectedRules := []*rules.Entry{expectedRule}

			expectedBuganizerBug := buganizerBug{
				ID:            1,
				Component:     projectCfg.BugManagement.Buganizer.DefaultComponent.Id,
				ExpectedTitle: "Failed to connect to 100.1.1.105.",
				// Expect the bug description to contain the top tests.
				ExpectedContent: []string{
					"https://luci-analysis-test.appspot.com/p/chromeos/rules/", // Rule ID randomly generated.
					"network-test-1",
					"network-test-2",
				},
				ExpectedPolicyIDsActivated: []string{
					"exoneration-policy",
				},
			}

			issueCount := func() int {
				return len(buganizerStore.Issues) + len(monorailStore.Issues)
			}

			// Bug-filing threshold met.
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay: metrics.Counts{Residual: 100},
			}

			Convey("bug filing threshold must be met to file a new bug", func() {
				Convey("Reason cluster", func() {
					Convey("Above threshold", func() {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)

						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
						So(issueCount(), ShouldEqual, 1)

						// Further updates do nothing.
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)

						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
						So(issueCount(), ShouldEqual, 1)
					})
					Convey("Below threshold", func() {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 99}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)

						// No bug should be created.
						So(verifyRulesResemble(ctx, nil), ShouldBeNil)
						So(issueCount(), ShouldEqual, 0)
					})
				})
				Convey("Test name cluster", func() {
					suggestedClusters[1].ClusterID = testIDClusterID(compiledCfg, "ui-test-1")
					suggestedClusters[1].TopTestIDs = []analysis.TopCount{
						{Value: "ui-test-1", Count: 10},
					}
					expectedRule.RuleDefinition = `test = "ui-test-1"`
					expectedRule.SourceCluster = suggestedClusters[1].ClusterID
					expectedBuganizerBug.ExpectedTitle = "ui-test-1"
					expectedBuganizerBug.ExpectedContent = []string{"ui-test-1"}

					// 34% more impact is required for a test name cluster to
					// be filed, compared to a failure reason cluster.
					Convey("Above threshold", func() {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 134}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
						So(issueCount(), ShouldEqual, 1)

						// Further updates do nothing.
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
						So(issueCount(), ShouldEqual, 1)
					})
					Convey("Below threshold", func() {
						suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 133}}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)

						// No bug should be created.
						So(verifyRulesResemble(ctx, nil), ShouldBeNil)
						So(issueCount(), ShouldEqual, 0)
					})
				})
			})
			Convey("policies are correctly activated when new bugs are filed", func() {
				Convey("other policy activation threshold not met", func() {
					suggestedClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{SevenDay: metrics.Counts{Residual: 9}}

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("other policy activation threshold met", func() {
					suggestedClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{SevenDay: metrics.Counts{Residual: 10}}
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
					expectedRule.BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true
					expectedBuganizerBug.ExpectedPolicyIDsActivated = []string{
						"cls-rejected-policy",
						"exoneration-policy",
					}

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
			})
			Convey("dispersion criteria must be met to file a new bug", func() {
				Convey("met via User CLs with failures", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("met via Postsubmit builds with failures", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("not met", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 0
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					// No bug should be created.
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
			})
			Convey("duplicate bugs are suppressed", func() {
				Convey("where a rule was recently filed for the same suggested cluster, and reclustering is pending", func() {
					createTime := time.Date(2021, time.January, 5, 12, 30, 0, 0, time.UTC)
					buganizerStore.StoreIssue(ctx, buganizer.NewFakeIssue(1))
					existingRule := rules.NewRule(1).
						WithBugSystem(bugs.BuganizerSystem).
						WithProject(project).
						WithCreateTime(createTime).
						WithPredicateLastUpdateTime(createTime.Add(1 * time.Hour)).
						WithLastAuditableUpdateTime(createTime.Add(2 * time.Hour)).
						WithLastUpdateTime(createTime.Add(3 * time.Hour)).
						WithBugPriorityManaged(true).
						WithBugPriorityManagedLastUpdateTime(createTime.Add(1 * time.Hour)).
						WithSourceCluster(sourceClusterID).Build()
					err := rules.SetForTesting(ctx, []*rules.Entry{
						existingRule,
					})
					So(err, ShouldBeNil)

					// Initially do not expect a new bug to be filed.
					err = UpdateBugsForProject(ctx, opts)

					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, []*rules.Entry{existingRule}), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)

					// Once re-clustering has incorporated the version of rules
					// that included this new rule, it is OK to file another bug
					// for the suggested cluster if sufficient impact remains.
					// This should only happen when the rule definition has been
					// manually narrowed in some way from the originally filed bug.
					err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
							WithConfigVersion(projectCfg.LastUpdated.AsTime()).
							WithRulesVersion(createTime).
							WithCompletedProgress().Build(),
					})
					So(err, ShouldBeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					So(err, ShouldBeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					expectedBuganizerBug.ID = 2 // Because we already created a bug with ID 1 above.
					expectedRule.BugID.ID = "2"
					So(verifyRulesResemble(ctx, []*rules.Entry{expectedRule, existingRule}), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 2)
				})
				Convey("when re-clustering to new algorithms", func() {
					err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion - 1).
							WithConfigVersion(projectCfg.LastUpdated.AsTime()).
							WithRulesVersion(rules.StartingEpoch).
							WithCompletedProgress().Build(),
					})
					So(err, ShouldBeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					So(err, ShouldBeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
				Convey("when re-clustering to new config", func() {
					err = runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
						runs.NewRun(0).
							WithProject(project).
							WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
							WithConfigVersion(projectCfg.LastUpdated.AsTime().Add(-1 * time.Hour)).
							WithRulesVersion(rules.StartingEpoch).
							WithCompletedProgress().Build(),
					})
					So(err, ShouldBeNil)
					progress, err := runs.ReadReclusteringProgress(ctx, project)
					So(err, ShouldBeNil)
					opts.ReclusteringProgress = progress

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
			})
			Convey("bugs are routed to the correct issue tracker and component", func() {
				suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{
					{Value: "77777", Count: 20},
				}
				expectedBuganizerBug.Component = 77777

				suggestedClusters[1].TopMonorailComponents = []analysis.TopCount{
					{Value: "Blink>Layout", Count: 40},  // >30% of failures.
					{Value: "Blink>Network", Count: 31}, // >30% of failures.
					{Value: "Blink>Other", Count: 4},
				}
				expectedMonorailBug := monorailBug{
					Project: "chromium",
					ID:      100,
					ExpectedComponents: []string{
						"projects/chromium/componentDefs/Blink>Layout",
						"projects/chromium/componentDefs/Blink>Network",
					},
					ExpectedTitle: "Failed to connect to 100.1.1.105.",
					// Expect the bug description to contain the top tests.
					ExpectedContent: []string{
						"network-test-1",
						"network-test-2",
					},
					ExpectedPolicyIDsActivated: []string{
						"exoneration-policy",
					},
				}
				expectedRule.BugID = bugs.BugID{
					System: "monorail",
					ID:     "chromium/100",
				}

				Convey("if Monorail component has greatest failure count, should create Monorail issue", func() {
					suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{{
						Value: "12345",
						Count: 39,
					}}

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectMonorailBug(monorailStore, expectedMonorailBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("if Buganizer component has higher failure count, should creates Buganizer issue", func() {
					suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{{
						// Check that null values are ignored.
						Value: "",
						Count: 100,
					}, {
						Value: "681721",
						Count: 41,
					}}
					expectedBuganizerBug.Component = 681721

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					expectedRule.BugID = bugs.BugID{
						System: "buganizer",
						ID:     "1",
					}
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("with no Buganizer configuration, should use Monorail as default system", func() {
					// Ensure Buganizer component has highest failure impact.
					suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{{
						Value: "88888",
						Count: 99999,
					}}

					// But buganizer is not configured, so we should file into monorail.
					projectCfg.BugManagement.Buganizer = nil
					err = config.SetTestProjectConfig(ctx, projectsCfg)
					So(err, ShouldBeNil)

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectMonorailBug(monorailStore, expectedMonorailBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("with no Monorail configuration, should use Buganizer as default system", func() {
					// Ensure Monorail component has highest failure impact.
					suggestedClusters[1].TopMonorailComponents = []analysis.TopCount{{
						Value: "Infra",
						Count: 99999,
					}}

					// But monorail is not configured, so we should file into Buganizer.
					projectCfg.BugManagement.Monorail = nil
					err = config.SetTestProjectConfig(ctx, projectsCfg)
					So(err, ShouldBeNil)

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					expectedRule.BugID = bugs.BugID{
						System: "buganizer",
						ID:     "1",
					}
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("in case of tied failure count between monorail/buganizer, should use default bug system", func() {
					// The default bug system is buganizer.
					So(projectCfg.BugManagement.DefaultBugSystem, ShouldEqual, configpb.BugSystem_BUGANIZER)

					suggestedClusters[1].TopBuganizerComponents = []analysis.TopCount{{
						Value: "",
						Count: 55,
					}, {
						Value: "681721",
						Count: 40, // Tied with monorail.
					}}
					expectedRule.BugID = bugs.BugID{
						System: "buganizer",
						ID:     "1",
					}
					expectedBuganizerBug.Component = 681721

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify we filed into Buganizer.
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
			})
			Convey("partial success creating bugs is correctly handled", func() {
				// Inject an error updating the bug after creation.
				buganizerClient.CreateCommentError = status.Errorf(codes.Internal, "internal error creating comment")

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Do not expect policy activations to have been notified.
				expectedBuganizerBug.ExpectedPolicyIDsActivated = []string{}
				expectedRule.BugManagementState.PolicyState["exoneration-policy"].ActivationNotified = false

				// Verify the rule was still created.
				So(err, ShouldErrLike, "internal error creating comment")
				So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
				So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
				So(issueCount(), ShouldEqual, 1)
			})
		})
		Convey("With both failure reason and test name clusters above bug-filing threshold", func() {
			// Reason cluster above the 1-day exoneration threshold.
			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 100},
				ThreeDay: metrics.Counts{Residual: 100},
				SevenDay: metrics.Counts{Residual: 100},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1

			// Test name cluster with 33% more impact.
			suggestedClusters[1] = makeTestNameCluster(compiledCfg, 3)
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 133},
				ThreeDay: metrics.Counts{Residual: 133},
				SevenDay: metrics.Counts{Residual: 133},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.MaxBugsFiledPerRun = 1

			Convey("reason clusters preferred over test name clusters", func() {
				// Test name cluster has <34% more impact than the reason
				// cluster.

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify reason cluster filed.
				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(len(rs), ShouldEqual, 1)
				So(rs[0].SourceCluster, ShouldResemble, suggestedClusters[2].ClusterID)
				So(rs[0].SourceCluster.IsFailureReasonCluster(), ShouldBeTrue)
			})
			Convey("test name clusters can be filed if significantly more impact", func() {
				// Increase impact of the test name cluster so that the
				// test name cluster has >34% more impact than the reason
				// cluster.
				suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
					OneDay:   metrics.Counts{Residual: 135},
					ThreeDay: metrics.Counts{Residual: 135},
					SevenDay: metrics.Counts{Residual: 135},
				}

				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify test name cluster filed.
				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(len(rs), ShouldEqual, 1)
				So(rs[0].SourceCluster, ShouldResemble, suggestedClusters[1].ClusterID)
				So(rs[0].SourceCluster.IsTestNameCluster(), ShouldBeTrue)
			})
		})
		Convey("With multiple rules / bugs on file", func() {
			// Use a mix of test name and failure reason clusters for
			// code path coverage.
			suggestedClusters[0] = makeTestNameCluster(compiledCfg, 0)
			suggestedClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 940},
				ThreeDay: metrics.Counts{Residual: 940},
				SevenDay: metrics.Counts{Residual: 940},
			}
			suggestedClusters[0].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[1] = makeReasonCluster(compiledCfg, 1)
			suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 300},
				ThreeDay: metrics.Counts{Residual: 300},
				SevenDay: metrics.Counts{Residual: 300},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 250},
				ThreeDay: metrics.Counts{Residual: 250},
				SevenDay: metrics.Counts{Residual: 250},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[2].TopMonorailComponents = []analysis.TopCount{
				{Value: "Monorail", Count: 250},
			}

			suggestedClusters[3] = makeReasonCluster(compiledCfg, 3)
			suggestedClusters[3].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 200},
				ThreeDay: metrics.Counts{Residual: 200},
				SevenDay: metrics.Counts{Residual: 200},
			}
			suggestedClusters[3].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[3].TopMonorailComponents = []analysis.TopCount{
				{Value: "Monorail", Count: 200},
			}

			expectedRules := []*rules.Entry{
				{
					Project:                 "chromeos",
					RuleDefinition:          `test = "testname-0"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1"},
					SourceCluster:           suggestedClusters[0].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "2"},
					SourceCluster:           suggestedClusters[1].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foofoo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/100"},
					SourceCluster:           suggestedClusters[2].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
				{
					Project:                 "chromeos",
					RuleDefinition:          `reason LIKE "want foofoofoo, got bar"`,
					BugID:                   bugs.BugID{System: bugs.MonorailSystem, ID: "chromium/101"},
					SourceCluster:           suggestedClusters[3].ClusterID,
					IsActive:                true,
					IsManagingBug:           true,
					IsManagingBugPriority:   true,
					CreateUser:              rules.LUCIAnalysisSystem,
					LastAuditableUpdateUser: rules.LUCIAnalysisSystem,
					BugManagementState: &bugspb.BugManagementState{
						RuleAssociationNotified: true,
						PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
							"exoneration-policy": {
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
								ActivationNotified: true,
							},
							"cls-rejected-policy": {},
						},
					},
				},
			}

			// The offset of the first monorail rule in the rules slice.
			// (Rules read by rules.Read...() are sorted by bug system and bug ID,
			// so monorail always appears after Buganizer.)
			const firstMonorailRuleIndex = 2

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.MaxBugsFiledPerRun = 1

			// Verify one bug is filed at a time.
			for i := 0; i < len(expectedRules); i++ {
				// Act
				err = UpdateBugsForProject(ctx, opts)

				// Verify
				So(err, ShouldBeNil)
				So(verifyRulesResemble(ctx, expectedRules[:i+1]), ShouldBeNil)
			}

			// Further updates do nothing.
			err = UpdateBugsForProject(ctx, opts)

			So(err, ShouldBeNil)
			So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

			rs, err := rules.ReadAllForTesting(span.Single(ctx))
			So(err, ShouldBeNil)

			bugClusters := []*analysis.Cluster{
				makeBugCluster(rs[0].RuleID),
				makeBugCluster(rs[1].RuleID),
				makeBugCluster(rs[2].RuleID),
				makeBugCluster(rs[3].RuleID),
			}

			Convey("if re-clustering in progress", func() {
				analysisClient.clusters = append(suggestedClusters, bugClusters...)

				Convey("negligable cluster metrics does not affect issue priority, status or active policies", func() {
					// The policy should already be active from previous setup.
					So(expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive, ShouldBeTrue)

					issue := buganizerStore.Issues[1]
					originalPriority := issue.Issue.IssueState.Priority
					originalStatus := issue.Issue.IssueState.Status
					So(originalStatus, ShouldNotEqual, issuetracker.Issue_VERIFIED)

					SetResidualMetrics(bugClusters[1], bugs.ClusterMetrics{
						metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{},
					})

					// Act
					err = UpdateBugsForProject(ctx, opts)

					// Verify.
					So(err, ShouldBeNil)
					So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
					So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
				})
			})
			Convey("with re-clustering complete", func() {
				analysisClient.clusters = append(suggestedClusters, bugClusters...)

				// Move residual impact from suggested clusters to new bug clusters.
				bugClusters[0].MetricValues = suggestedClusters[0].MetricValues
				bugClusters[1].MetricValues = suggestedClusters[1].MetricValues
				bugClusters[2].MetricValues = suggestedClusters[2].MetricValues
				bugClusters[3].MetricValues = suggestedClusters[3].MetricValues

				// Clear residual impact on suggested clusters to inhibit
				// further bug filing.
				suggestedClusters[0].MetricValues = emptyMetricValues()
				suggestedClusters[1].MetricValues = emptyMetricValues()
				suggestedClusters[2].MetricValues = emptyMetricValues()
				suggestedClusters[3].MetricValues = emptyMetricValues()

				// Mark reclustering complete.
				err := runs.SetRunsForTesting(ctx, []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject(project).
						WithAlgorithmsVersion(algorithms.AlgorithmsVersion).
						WithConfigVersion(projectCfg.LastUpdated.AsTime()).
						WithRulesVersion(rs[3].PredicateLastUpdateTime).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)

				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.ReclusteringProgress = progress

				opts.RunTimestamp = opts.RunTimestamp.Add(10 * time.Minute)

				Convey("policy activation", func() {
					// Verify updates work, even when rules are in later batches.
					opts.UpdateRuleBatchSize = 1

					Convey("policy remains inactive if activation threshold unmet", func() {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"]
						So(expectedPolicyState.IsActive, ShouldBeFalse)

						// Set metrics just below the policy activation threshold.
						bugClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 9},
							ThreeDay: metrics.Counts{Residual: 9},
							SevenDay: metrics.Counts{Residual: 9},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activation unchanged.
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy activates if activation threshold met", func() {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"]
						So(expectedPolicyState.IsActive, ShouldBeFalse)
						So(expectedPolicyState.ActivationNotified, ShouldBeFalse)

						// Update metrics so that policy should activate.
						bugClusters[1].MetricValues[metrics.HumanClsFailedPresubmit.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 0},
							ThreeDay: metrics.Counts{Residual: 0},
							SevenDay: metrics.Counts{Residual: 10},
						}

						issue := buganizerStore.Issues[2]
						So(expectedRules[1].BugID, ShouldResemble, bugs.BugID{System: bugs.BuganizerSystem, ID: "2"})
						existingCommentCount := len(issue.Comments)

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activates.
						So(err, ShouldBeNil)
						expectedPolicyState.IsActive = true
						expectedPolicyState.LastActivationTime = timestamppb.New(opts.RunTimestamp)
						expectedPolicyState.ActivationNotified = true
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

						// Expect comments to be posted.
						So(issue.Comments, ShouldHaveLength, existingCommentCount+2)
						So(issue.Comments[2].Comment, ShouldContainSubstring,
							"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: cls-rejected-policy)")
						So(issue.Comments[3].Comment, ShouldContainSubstring,
							"The bug priority has been increased from P2 to P1.")
					})
					Convey("policy remains active if deactivation threshold unmet", func() {
						// The policy should already be active from previous setup.
						expectedPolicyState := expectedRules[0].BugManagementState.PolicyState["exoneration-policy"]
						So(expectedPolicyState.IsActive, ShouldBeTrue)

						// Metrics still meet/exceed the deactivation threshold, so deactivation is inhibited.
						bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 10},
							ThreeDay: metrics.Counts{Residual: 10},
							SevenDay: metrics.Counts{Residual: 10},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy activation should be unchanged.
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy deactivates if deactivation threshold met", func() {
						// The policy should already be active from previous setup.
						expectedPolicyState := expectedRules[0].BugManagementState.PolicyState["exoneration-policy"]
						So(expectedPolicyState.IsActive, ShouldBeTrue)

						// Update metrics so that policy should de-activate.
						bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 9},
							ThreeDay: metrics.Counts{Residual: 9},
							SevenDay: metrics.Counts{Residual: 9},
						}

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify policy deactivated.
						So(err, ShouldBeNil)
						expectedPolicyState.IsActive = false
						expectedPolicyState.LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy configuration changes are handled", func() {
						// Delete the existing policy named "exoneration-policy", and replace it with a new policy,
						// "new-exoneration-policy". Activation and de-activation criteria remain the same.
						projectCfg.BugManagement.Policies[0].Id = "new-exoneration-policy"

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify state for the old policy is deleted, and state for the new policy is added.
						So(err, ShouldBeNil)
						expectedRules[0].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {
								// The new policy should activate, because the metrics justify its activation.
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.RunTimestamp),
							},
							"cls-rejected-policy": {},
						}
						expectedRules[1].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {},
							"cls-rejected-policy":    {},
						}
						expectedRules[2].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {},
							"cls-rejected-policy":    {},
						}
					})
				})
				Convey("rule associated notification", func() {
					Convey("buganizer", func() {
						// Select a Buganizer issue.
						issue := buganizerStore.Issues[1]

						// Get the corresponding rule, confirming we got the right one.
						rule := rs[0]
						So(rule.BugID.ID, ShouldEqual, fmt.Sprintf("%v", issue.Issue.IssueId))

						// Reset RuleAssociationNotified on the rule.
						rule.BugManagementState.RuleAssociationNotified = false
						So(rules.SetForTesting(ctx, rs), ShouldBeNil)

						originalCommentCount := len(issue.Comments)

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(issue.Comments, ShouldHaveLength, originalCommentCount+1)
						So(issue.Comments[originalCommentCount].Comment, ShouldEqual,
							"This bug has been associated with failures in LUCI Analysis."+
								" To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/"+rule.RuleID)

						// Further runs should not lead to repeated posting of the comment.
						err = UpdateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(issue.Comments, ShouldHaveLength, originalCommentCount+1)
					})
					Convey("monorail", func() {
						// Select a Monorail issue.
						issue := monorailStore.Issues[0]

						// Get the corresponding rule, and confirm we got the right one.
						const ruleIndex = firstMonorailRuleIndex
						rule := rs[ruleIndex]
						So(rule.BugID.ID, ShouldEqual, "chromium/100")
						So(issue.Issue.Name, ShouldEqual, "projects/chromium/issues/100")

						// Reset RuleAssociationNotified on the rule.
						rule.BugManagementState.RuleAssociationNotified = false
						So(rules.SetForTesting(ctx, rs), ShouldBeNil)

						originalCommentCount := len(issue.Comments)

						// Act
						err = UpdateBugsForProject(ctx, opts)

						// Verify
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(issue.Comments, ShouldHaveLength, originalCommentCount+1)
						So(issue.Comments[originalCommentCount].Content, ShouldContainSubstring,
							"This bug has been associated with failures in LUCI Analysis."+
								" To view failure examples or update the association, go to LUCI Analysis at: https://luci-analysis-test.appspot.com/p/chromeos/rules/"+rule.RuleID)

						// Further runs should not lead to repeated posting of the comment.
						err = UpdateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						So(issue.Comments, ShouldHaveLength, originalCommentCount+1)
					})
				})
				Convey("priority updates and auto-closure", func() {
					Convey("buganizer", func() {
						// Select a Buganizer issue.
						issue := buganizerStore.Issues[1]
						originalPriority := issue.Issue.IssueState.Priority
						originalStatus := issue.Issue.IssueState.Status
						So(originalStatus, ShouldEqual, issuetracker.Issue_NEW)

						// Get the corresponding rule, confirming we got the right one.
						rule := rs[0]
						So(rule.BugID.ID, ShouldEqual, fmt.Sprintf("%v", issue.Issue.IssueId))

						// Activate the cls-rejected-policy, which should raise the priority to P1.
						So(originalPriority, ShouldNotEqual, issuetracker.Issue_P1)
						SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
							metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
							metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
						})

						Convey("priority updates to reflect active policies", func() {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true
							So(originalPriority, ShouldNotEqual, issuetracker.Issue_P1)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P1)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("disabling IsManagingBugPriority prevents priority updates", func() {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							// Set IsManagingBugPriority to false on the rule.
							rule.IsManagingBugPriority = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
							So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)

							// Check the rules have not changed except for the IsManagingBugPriority change.
							expectedRules[0].IsManagingBugPriority = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("manually setting a priority prevents bug updates", func() {
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[0].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							issue.IssueUpdates = append(issue.IssueUpdates, &issuetracker.IssueUpdate{
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

							Convey("happy path", func() {
								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
								So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
								expectedRules[0].IsManagingBugPriority = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								Convey("further updates leave no comments", func() {
									initialComments := len(issue.Comments)

									// Act
									err = UpdateBugsForProject(ctx, opts)

									// Verify
									So(err, ShouldBeNil)
									So(len(issue.Comments), ShouldEqual, initialComments)
									So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
									So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
									So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								})
							})

							Convey("errors updating other bugs", func() {
								// Check we handle partial success correctly:
								// Even if there is an error updating another bug, if we comment on a bug
								// to say the user took manual priority control, we must commit
								// the rule update setting IsManagingBugPriority to false. Otherwise
								// we may get stuck in a loop where we comment on the bug every
								// time bug filing runs.

								// Trigger a priority update for bug 2 in addition to the
								// manual priority update.
								SetResidualMetrics(bugClusters[1], bugs.ClusterMetrics{
									metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
									metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
								})

								// But prevent LUCI Analysis from applying that priority update, due to an error.
								modifyError := errors.New("this issue may not be modified")
								buganizerStore.Issues[2].UpdateError = modifyError

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify

								// The error modifying bug 2 is bubbled up.
								So(err, ShouldNotBeNil)
								So(errors.Is(err, modifyError), ShouldBeTrue)

								// The policy on the bug 2 was activated, and we notified
								// bug 2 of the policy activation, even if we did
								// not succeed then updating its priority.
								// Furthermore, we record that we notified the policy
								// activation, so repeated notifications do not occur.
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
								expectedRules[1].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

								otherIssue := buganizerStore.Issues[2]
								So(otherIssue.Comments[len(otherIssue.Comments)-1].Comment, ShouldContainSubstring,
									"Why LUCI Analysis posted this comment: https://luci-analysis-test.appspot.com/help#policy-activated (Policy ID: cls-rejected-policy)")

								// Despite the issue with bug 2, bug 1 was commented on updated and
								// IsManagingBugPriority was set to false.
								expectedRules[0].IsManagingBugPriority = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Comments[len(issue.Comments)-1].Comment, ShouldContainSubstring,
									"The bug priority has been manually set.")
								So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
								So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
							})
						})
						Convey("if all policies de-activate, bug is auto-closed", func() {
							SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P2)
						})
						Convey("disabling IsManagingBug prevents bug closure", func() {
							SetResidualMetrics(bugClusters[0], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)

							// Set IsManagingBug to false on the rule.
							rule.IsManagingBug = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check the rules have not changed except for the IsManagingBug change.
							expectedRules[0].IsManagingBug = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
							So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
						})
						Convey("cluster disappearing closes issue", func() {
							// Drop the corresponding bug cluster. This is consistent with
							// no more failures in the cluster occuring.
							bugClusters = bugClusters[1:]
							analysisClient.clusters = append(suggestedClusters, bugClusters...)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)

							Convey("rule automatically archived after 30 days", func() {
								tc.Add(time.Hour * 24 * 30)

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[0].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
							})
						})
						Convey("if all policies are removed, bug is auto-closed", func() {
							projectCfg.BugManagement.Policies = nil
							err := config.SetTestProjectConfig(ctx, projectsCfg)
							So(err, ShouldBeNil)

							for _, expectedRule := range expectedRules {
								expectedRule.BugManagementState.PolicyState = nil
							}

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P2)
						})
					})
					Convey("monorail", func() {
						// Select a Monorail issue.
						issue := monorailStore.Issues[0]
						originalPriority := monorail.ChromiumTestIssuePriority(issue.Issue)
						originalStatus := issue.Issue.Status.Status
						So(originalStatus, ShouldEqual, monorail.UntriagedStatus)

						// Get the corresponding rule, and confirm we got the right one.
						const ruleIndex = firstMonorailRuleIndex
						rule := rs[ruleIndex]
						So(rule.BugID.ID, ShouldEqual, "chromium/100")
						So(issue.Issue.Name, ShouldEqual, "projects/chromium/issues/100")

						// Activate the cls-rejected-policy, which should raise the priority to P1.
						So(originalPriority, ShouldNotEqual, issuetracker.Issue_P1)
						SetResidualMetrics(bugClusters[firstMonorailRuleIndex], bugs.ClusterMetrics{
							metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
							metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
						})

						Convey("priority updates to reflect active policies", func() {
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true
							So(originalPriority, ShouldNotEqual, "1")

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "1")
						})
						Convey("disabling IsManagingBugPriority prevents priority updates", func() {
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							// Set IsManagingBugPriority to false on the rule.
							rule.IsManagingBugPriority = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check the rules have not changed except for the IsManagingBugPriority change.
							expectedRules[firstMonorailRuleIndex].IsManagingBugPriority = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)
						})
						Convey("manually setting a priority prevents bug updates", func() {
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["cls-rejected-policy"].ActivationNotified = true

							// Create a fake client to interact with monorail as a user.
							userClient, err := monorail.NewClient(monorail.UseFakeIssuesClient(ctx, monorailStore, "user@google.com"), "myhost")
							So(err, ShouldBeNil)

							// Set priority to P0 manually.
							updateRequest := &mpb.ModifyIssuesRequest{
								Deltas: []*mpb.IssueDelta{
									{
										Issue: &mpb.Issue{
											Name: issue.Issue.Name,
											FieldValues: []*mpb.FieldValue{
												{
													Field: "projects/chromium/fieldDefs/11",
													Value: "0",
												},
											},
										},
										UpdateMask: &fieldmaskpb.FieldMask{
											Paths: []string{"field_values"},
										},
									},
								},
								CommentContent: "User comment.",
							}
							err = userClient.ModifyIssues(ctx, updateRequest)
							So(err, ShouldBeNil)

							Convey("happy path", func() {
								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)

								// Expect IsManagingBugPriority to get set to false.
								expectedRules[firstMonorailRuleIndex].IsManagingBugPriority = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								// Expect a comment on the bug.
								So(issue.Comments[len(issue.Comments)-1].Content, ShouldContainSubstring,
									"The bug priority has been manually set.")
								So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
								So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "0")

								Convey("further updates leave no comments", func() {
									initialComments := len(issue.Comments)

									// Act
									err = UpdateBugsForProject(ctx, opts)

									// Verify
									So(err, ShouldBeNil)
									So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
									So(len(issue.Comments), ShouldEqual, initialComments)
									So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
									So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "0")
								})
							})
							Convey("errors updating other bugs", func() {
								// Check we handle partial success correctly:
								// Even if there is an error updating another bug, if we comment on a bug
								// to say the user took manual priority control, we must commit
								// the rule update setting IsManagingBugPriority to false. Otherwise
								// we may get stuck in a loop where we comment on the bug every
								// time bug filing runs.

								// Trigger a priority update for another monorail bug in addition to the
								// manual priority update.
								SetResidualMetrics(bugClusters[firstMonorailRuleIndex+1], bugs.ClusterMetrics{
									metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 100},
									metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{SevenDay: 10},
								})
								expectedRules[firstMonorailRuleIndex+1].BugManagementState.PolicyState["cls-rejected-policy"].IsActive = true
								expectedRules[firstMonorailRuleIndex+1].BugManagementState.PolicyState["cls-rejected-policy"].LastActivationTime = timestamppb.New(opts.RunTimestamp)

								// But prevent LUCI Analysis from applying that priority update, due to an error.
								modifyError := errors.New("this issue may not be modified")
								monorailStore.Issues[1].UpdateError = modifyError

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldNotBeNil)

								// The error modifying the other bug is bubbled up.
								So(errors.Is(err, modifyError), ShouldBeTrue)

								// Nonetheless, our bug was commented on updated and
								// IsManagingBugPriority was set to false.
								expectedRules[firstMonorailRuleIndex].IsManagingBugPriority = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Comments[len(issue.Comments)-1].Content, ShouldContainSubstring,
									"The bug priority has been manually set.")
								So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
								So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "0")
							})
						})
						Convey("if all policies de-activate, bug is auto-closed", func() {
							SetResidualMetrics(bugClusters[firstMonorailRuleIndex], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)
						})
						Convey("disabling IsManagingBug prevents bug closure", func() {
							SetResidualMetrics(bugClusters[firstMonorailRuleIndex], bugs.ClusterMetrics{
								metrics.CriticalFailuresExonerated.ID: bugs.MetricValues{OneDay: 9},
								metrics.HumanClsFailedPresubmit.ID:    bugs.MetricValues{},
							})
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)

							// Set IsManagingBug to false on the rule.
							rule.IsManagingBug = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check the rules have not changed except for the IsManagingBug change.
							expectedRules[firstMonorailRuleIndex].IsManagingBug = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)
						})
						Convey("cluster disappearing closes issue", func() {
							// Drop the corresponding bug cluster. This is consistent with
							// no more failures in the cluster occuring.
							newBugClusters := []*analysis.Cluster{}
							newBugClusters = append(newBugClusters, bugClusters[:firstMonorailRuleIndex]...)
							newBugClusters = append(newBugClusters, bugClusters[firstMonorailRuleIndex+1:]...)
							analysisClient.clusters = append(suggestedClusters, newBugClusters...)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[firstMonorailRuleIndex].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.RunTimestamp)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)

							Convey("rule automatically archived after 30 days", func() {
								tc.Add(time.Hour * 24 * 30)

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[firstMonorailRuleIndex].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
								So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)
							})
						})
						Convey("if all policies are removed, bug is auto-closed", func() {
							projectCfg.BugManagement.Policies = nil
							err := config.SetTestProjectConfig(ctx, projectsCfg)
							So(err, ShouldBeNil)

							for _, expectedRule := range expectedRules {
								expectedRule.BugManagementState.PolicyState = nil
							}

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
					})
				})
				Convey("duplicate handling", func() {
					Convey("buganizer to buganizer", func() {
						// Setup
						issueOne := buganizerStore.Issues[1]
						issueTwo := buganizerStore.Issues[2]
						issueOne.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
						issueOne.Issue.IssueState.CanonicalIssueId = issueTwo.Issue.IssueId

						issueOneOriginalCommentCount := len(issueOne.Comments)
						issueTwoOriginalCommentCount := len(issueTwo.Comments)

						// Ensure rule association and policy activation notified, so we
						// can confirm whether notifications are correctly reset.
						rs[0].BugManagementState.RuleAssociationNotified = true
						for _, policyState := range rs[0].BugManagementState.PolicyState {
							policyState.ActivationNotified = true
						}
						So(rules.SetForTesting(ctx, rs), ShouldBeNil)

						expectedRules[0].BugManagementState.RuleAssociationNotified = true
						for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
							policyState.ActivationNotified = true
						}

						Convey("happy path", func() {
							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
							So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, expectedRules[2].RuleID)

							So(issueTwo.Comments, ShouldHaveLength, issueTwoOriginalCommentCount)
						})
						Convey("happy path, with comments for duplicate bugs disabled", func() {
							// Setup
							projectCfg.BugManagement.DisableDuplicateBugComments = true
							projectsCfg := map[string]*configpb.ProjectConfig{
								project: projectCfg,
							}
							err = config.SetTestProjectConfig(ctx, projectsCfg)
							So(err, ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount)
							So(issueTwo.Comments, ShouldHaveLength, issueTwoOriginalCommentCount)
						})
						Convey("happy path, bug marked as duplicate of bug without a rule in this project", func() {
							// Setup
							issueOne.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
							issueOne.Issue.IssueState.CanonicalIssueId = 1234

							buganizerStore.StoreIssue(ctx, buganizer.NewFakeIssue(1234))

							extraRule := &rules.Entry{
								Project:                 "otherproject",
								RuleDefinition:          `reason LIKE "blah"`,
								RuleID:                  "1234567890abcdef1234567890abcdef",
								BugID:                   bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"},
								IsActive:                true,
								IsManagingBug:           true,
								IsManagingBugPriority:   true,
								BugManagementState:      &bugspb.BugManagementState{},
								CreateUser:              "user@chromium.org",
								LastAuditableUpdateUser: "user@chromium.org",
							}
							_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
								ms, err := rules.Create(extraRule, "user@chromium.org")
								if err != nil {
									return err
								}
								span.BufferWrite(ctx, ms)
								return nil
							})
							So(err, ShouldBeNil)

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
							// Should reset to false as we didn't create the destination bug.
							expectedRules[0].IsManagingBug = false
							// Should reset because of the change in associated bug.
							expectedRules[0].BugManagementState.RuleAssociationNotified = false
							for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
								policyState.ActivationNotified = false
							}
							expectedRules = append(expectedRules, extraRule)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
							So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, expectedRules[0].RuleID)
						})
						Convey("error cases", func() {
							Convey("bugs are in a duplicate bug cycle", func() {
								// Note that this is a simple cycle with only two bugs.
								// The implementation allows for larger cycles, however.
								issueTwo.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
								issueTwo.Issue.IssueState.CanonicalIssueId = issueOne.Issue.IssueId

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)

								// Issue one kicked out of duplicate status.
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

								// As the cycle is now broken, issue two is merged into
								// issue one.
								expectedRules[0].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
								expectedRules[1].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
								So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
							})
							Convey("merged rule would be too long", func() {
								// Setup
								// Make one of the rules we will be merging very close
								// to the rule length limit.
								longRule := fmt.Sprintf("test = \"%s\"", strings.Repeat("a", rules.MaxRuleDefinitionLength-10))

								_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
									issueOneRule, err := rules.ReadByBug(ctx, bugs.BugID{System: bugs.BuganizerSystem, ID: "1"})
									if err != nil {
										return err
									}
									issueOneRule[0].RuleDefinition = longRule

									ms, err := rules.Update(issueOneRule[0], rules.UpdateOptions{
										IsAuditableUpdate: true,
										PredicateUpdated:  true,
									}, rules.LUCIAnalysisSystem)
									if err != nil {
										return err
									}
									span.BufferWrite(ctx, ms)
									return nil
								})
								So(err, ShouldBeNil)

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)

								// Rules should not have changed (except for the update we made).
								expectedRules[0].RuleDefinition = longRule
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								// Issue one kicked out of duplicate status.
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

								// Comment should appear on the bug.
								So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
								So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, "the merged failure association rule would be too long")
							})
							Convey("bug marked as duplicate of bug we cannot access", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue one kicked out of duplicate status.
								So(err, ShouldBeNil)
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)
								So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
								So(issueOne.Comments[issueOneOriginalCommentCount].Comment, ShouldContainSubstring, "LUCI Analysis cannot merge the association rule for this bug into the rule")
							})
							Convey("failed to handle duplicate bug - bug has an assignee", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has an assignee.
								issueOne.Issue.IssueState.Assignee = &issuetracker.User{
									EmailAddress: "user@google.com",
								}
								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue is put back to assigned status, instead of New.
								So(err, ShouldBeNil)
								So(issueOne.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_ASSIGNED)
							})
							Convey("failed to handle duplicate bug - bug has no assignee", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has no assignee.
								issueOne.Issue.IssueState.Assignee = nil

								// Act
								err = UpdateBugsForProject(ctx, opts)

								// Verify issue is put back to New status, instead of Assigned.
								So(err, ShouldBeNil)
								So(issueOne.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
							})
						})
					})
					Convey("monorail to monorail", func() {
						// Note that much of the duplicate handling logic, including error
						// handling, is shared code and not implemented in the bug system-specific
						// bug manager. As such, we do not re-test all of the error cases above,
						// only select cases to confirm the integration is correct.

						issueOne := monorailStore.Issues[0]
						issueTwo := monorailStore.Issues[1]

						issueOne.Issue.Status.Status = monorail.DuplicateStatus
						issueOne.Issue.MergedIntoIssueRef = &mpb.IssueRef{
							Issue: issueTwo.Issue.Name,
						}

						issueOneRule := expectedRules[firstMonorailRuleIndex]
						issueTwoRule := expectedRules[firstMonorailRuleIndex+1]

						issueOneOriginalCommentCount := len(issueOne.Comments)
						issueTwoOriginalCommentCount := len(issueTwo.Comments)

						Convey("happy path", func() {
							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							issueOneRule.IsActive = false
							issueTwoRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\nreason LIKE \"want foofoofoo, got bar\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
							So(issueOne.Comments[issueOneOriginalCommentCount].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[issueOneOriginalCommentCount].Content, ShouldContainSubstring, issueOneRule.RuleID)

							So(issueTwo.Comments, ShouldHaveLength, issueTwoOriginalCommentCount)
						})
						Convey("error case", func() {
							// Note that this is a simple cycle with only two bugs.
							// The implementation allows for larger cycles, however.
							issueTwo.Issue.Status.Status = monorail.DuplicateStatus
							issueTwo.Issue.MergedIntoIssueRef = &mpb.IssueRef{
								Issue: issueOne.Issue.Name,
							}

							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Issue one kicked out of duplicate status.
							So(issueOne.Issue.Status.Status, ShouldNotEqual, monorail.DuplicateStatus)

							// As the cycle is now broken, issue two is merged into
							// issue one.
							issueOneRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\nreason LIKE \"want foofoofoo, got bar\""
							issueTwoRule.IsActive = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
							So(issueOne.Comments[issueOneOriginalCommentCount].Content, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
						})
					})
					Convey("monorail to buganizer", func() {
						issueOne := monorailStore.Issues[0]
						issueTwo := buganizerStore.Issues[1]

						issueOne.Issue.Status.Status = monorail.DuplicateStatus
						issueOne.Issue.MergedIntoIssueRef = &mpb.IssueRef{
							ExtIdentifier: fmt.Sprintf("b/%v", issueTwo.Issue.IssueId),
						}

						issueOneRule := expectedRules[firstMonorailRuleIndex]
						issueTwoRule := expectedRules[0]

						issueOneOriginalCommentCount := len(issueOne.Comments)
						issueTwoOriginalCommentCount := len(issueTwo.Comments)

						Convey("happy path", func() {
							// Act
							err = UpdateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							issueOneRule.IsActive = false
							issueTwoRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, issueOneOriginalCommentCount+1)
							So(issueOne.Comments[issueOneOriginalCommentCount].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[issueOneOriginalCommentCount].Content, ShouldContainSubstring, issueOneRule.RuleID)

							So(issueTwo.Comments, ShouldHaveLength, issueTwoOriginalCommentCount)
						})
					})
				})
				Convey("bug marked as archived should archive rule", func() {
					Convey("buganizer", func() {
						issueOne := buganizerStore.Issues[1].Issue
						issueOne.IsArchived = true

						// Act
						err = UpdateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[0].IsActive = false
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("monorail", func() {
						issue := monorailStore.Issues[0]
						issue.Issue.Status.Status = "Archived"

						// Act
						err = UpdateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[2].IsActive = false
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
				})
			})
		})
	})
}

func createProjectConfig() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		BugManagement: &configpb.BugManagement{
			DefaultBugSystem: configpb.BugSystem_BUGANIZER,
			Buganizer:        buganizer.ChromeOSTestConfig(),
			Monorail:         monorail.ChromiumTestConfig(),
			Policies: []*configpb.BugManagementPolicy{
				createExonerationPolicy(),
				createCLsRejectedPolicy(),
			},
		},
		LastUpdated: timestamppb.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)),
	}
}

func createExonerationPolicy() *configpb.BugManagementPolicy {
	return &configpb.BugManagementPolicy{
		Id:                "exoneration-policy",
		Owners:            []string{"username@google.com"},
		HumanReadableName: "test variant(s) are being exonerated in presubmit",
		Priority:          configpb.BuganizerPriority_P2,
		Metrics: []*configpb.BugManagementPolicy_Metric{
			{
				MetricId: metrics.CriticalFailuresExonerated.ID.String(),
				ActivationThreshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(100),
				},
				DeactivationThreshold: &configpb.MetricThreshold{
					OneDay: proto.Int64(10),
				},
			},
		},
		Explanation: &configpb.BugManagementPolicy_Explanation{
			ProblemHtml: "problem",
			ActionHtml:  "action",
		},
		BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
			CommentTemplate: `{{if .BugID.IsBuganizer }}Buganizer Bug ID: {{ .BugID.BuganizerBugID }}{{end}}` +
				`{{if .BugID.IsMonorail }}Monorail Project: {{ .BugID.MonorailProject }}; ID: {{ .BugID.MonorailBugID }}{{end}}` +
				`Rule URL: {{.RuleURL}}`,
			Monorail: &configpb.BugManagementPolicy_BugTemplate_Monorail{
				Labels: []string{"Test-Exonerated"},
			},
			Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
				Hotlists: []int64{1234},
			},
		},
	}
}

func createCLsRejectedPolicy() *configpb.BugManagementPolicy {
	return &configpb.BugManagementPolicy{
		Id:                "cls-rejected-policy",
		Owners:            []string{"username@google.com"},
		HumanReadableName: "many CL(s) are being falsely rejected in presubmit",
		Priority:          configpb.BuganizerPriority_P1,
		Metrics: []*configpb.BugManagementPolicy_Metric{
			{
				MetricId: metrics.HumanClsFailedPresubmit.ID.String(),
				ActivationThreshold: &configpb.MetricThreshold{
					SevenDay: proto.Int64(10),
				},
				DeactivationThreshold: &configpb.MetricThreshold{
					SevenDay: proto.Int64(1),
				},
			},
		},
		Explanation: &configpb.BugManagementPolicy_Explanation{
			ProblemHtml: "problem",
			ActionHtml:  "action",
		},
		BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
			CommentTemplate: `Many CLs are failing presubmit. Policy text goes here.`,
			Monorail: &configpb.BugManagementPolicy_BugTemplate_Monorail{
				Labels: []string{"Test-Exonerated"},
			},
			Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
				Hotlists: []int64{1234},
			},
		},
	}
}

// verifyRulesResemble verifies rules stored in Spanner resemble
// the passed expectations, modulo assigned RuleIDs and
// audit timestamps.
func verifyRulesResemble(ctx context.Context, expectedRules []*rules.Entry) error {
	// Read all rules. Sorted by BugSystem, BugId, Project.
	rs, err := rules.ReadAllForTesting(span.Single(ctx))
	if err != nil {
		return err
	}

	// Sort expectations in the same order as rules.
	sortedExpected := make([]*rules.Entry, len(expectedRules))
	copy(sortedExpected, expectedRules)
	sort.Slice(sortedExpected, func(i, j int) bool {
		ruleI := sortedExpected[i]
		ruleJ := sortedExpected[j]
		if ruleI.BugID.System != ruleJ.BugID.System {
			return ruleI.BugID.System < ruleJ.BugID.System
		}
		if ruleI.BugID.ID != ruleJ.BugID.ID {
			return ruleI.BugID.ID < ruleJ.BugID.ID
		}
		return ruleI.Project < ruleJ.Project
	})

	for _, r := range rs {
		// Accept whatever values the implementation has set
		// (these values are assigned non-deterministically).
		r.RuleID = ""
		r.CreateTime = time.Time{}
		r.LastAuditableUpdateTime = time.Time{}
		r.LastUpdateTime = time.Time{}
		r.PredicateLastUpdateTime = time.Time{}
		r.IsManagingBugPriorityLastUpdateTime = time.Time{}
	}
	for i, rule := range sortedExpected {
		expectationCopy := rule.Clone()
		// Clear the fields on the expectations as well.
		expectationCopy.RuleID = ""
		expectationCopy.CreateTime = time.Time{}
		expectationCopy.LastAuditableUpdateTime = time.Time{}
		expectationCopy.LastUpdateTime = time.Time{}
		expectationCopy.PredicateLastUpdateTime = time.Time{}
		expectationCopy.IsManagingBugPriorityLastUpdateTime = time.Time{}
		sortedExpected[i] = expectationCopy
	}

	if diff := ShouldResembleProto(rs, sortedExpected); diff != "" {
		return errors.Reason("stored rules: %s", diff).Err()
	}
	return nil
}

type buganizerBug struct {
	// Bug ID.
	ID int64
	// Expected buganizer component ID.
	Component int64
	// Content that is expected to appear in the bug title.
	ExpectedTitle string
	// Content that is expected to appear in the bug description.
	ExpectedContent []string
	// The policies which were expected to have activated, in the
	// order they should have reported activation.
	ExpectedPolicyIDsActivated []string
}

func expectBuganizerBug(buganizerStore *buganizer.FakeIssueStore, bug buganizerBug) error {
	issue := buganizerStore.Issues[bug.ID].Issue
	if issue == nil {
		return errors.Reason("buganizer issue %v not found", bug.ID).Err()
	}
	if issue.IssueId != bug.ID {
		return errors.Reason("issue ID: got %v, want %v", issue.IssueId, bug.ID).Err()
	}
	if !strings.Contains(issue.IssueState.Title, bug.ExpectedTitle) {
		return errors.Reason("issue title: got %q, expected it to contain %q", issue.IssueState.Title, bug.ExpectedTitle).Err()
	}
	if issue.IssueState.ComponentId != bug.Component {
		return errors.Reason("component: got %v; want %v", issue.IssueState.ComponentId, bug.Component).Err()
	}

	for _, expectedContent := range bug.ExpectedContent {
		if !strings.Contains(issue.Description.Comment, expectedContent) {
			return errors.Reason("issue description: got %q, expected it to contain %q", issue.Description.Comment, expectedContent).Err()
		}
	}
	comments := buganizerStore.Issues[bug.ID].Comments
	if len(comments) != 1+len(bug.ExpectedPolicyIDsActivated) {
		return errors.Reason("issue comments: got %v want %v", len(comments), 1+len(bug.ExpectedPolicyIDsActivated)).Err()
	}
	for i, activatedPolicyID := range bug.ExpectedPolicyIDsActivated {
		expectedContent := fmt.Sprintf("(Policy ID: %s)", activatedPolicyID)
		if !strings.Contains(comments[1+i].Comment, expectedContent) {
			return errors.Reason("issue comment %v: got %q, expected it to contain %q", i+1, comments[i+1].Comment, expectedContent).Err()
		}
	}
	return nil
}

type monorailBug struct {
	// The monorail project.
	Project string
	// The monorail bug ID.
	ID int

	ExpectedComponents []string
	// Content that is expected to appear in the bug title.
	ExpectedTitle string
	// Content that is expected to appear in the bug description.
	ExpectedContent []string
	// The policies which were expected to have activated, in the
	// order they should have reported activation.
	ExpectedPolicyIDsActivated []string
}

func expectMonorailBug(monorailStore *monorail.FakeIssuesStore, bug monorailBug) error {
	var issue *monorail.IssueData
	name := fmt.Sprintf("projects/%s/issues/%v", bug.Project, bug.ID)
	for _, iss := range monorailStore.Issues {
		if iss.Issue.Name == name {
			issue = iss
			break
		}
	}
	if issue == nil {
		return errors.Reason("monorail issue %q not found", name).Err()
	}
	if !strings.Contains(issue.Issue.Summary, bug.ExpectedTitle) {
		return errors.Reason("issue title: got %q, expected it to contain %q", issue.Issue.Summary, bug.ExpectedTitle).Err()
	}
	var actualComponents []string
	for _, component := range issue.Issue.Components {
		actualComponents = append(actualComponents, component.Component)
	}
	if msg := ShouldResemble(actualComponents, bug.ExpectedComponents); msg != "" {
		return errors.Reason("components: %s", msg).Err()
	}
	comments := issue.Comments
	if len(comments) != 1+len(bug.ExpectedPolicyIDsActivated) {
		return errors.Reason("issue comments: got %v want %v", len(comments), 1+len(bug.ExpectedPolicyIDsActivated)).Err()
	}
	for _, expectedContent := range bug.ExpectedContent {
		if !strings.Contains(issue.Comments[0].Content, expectedContent) {
			return errors.Reason("issue description: got %q, expected it to contain %q", issue.Comments[0].Content, expectedContent).Err()
		}
	}
	expectedLink := "https://luci-analysis-test.appspot.com/p/chromeos/rules/"
	if !strings.Contains(comments[0].Content, expectedLink) {
		return errors.Reason("issue comment #1: got %q, expected it to contain %q", issue.Comments[0].Content, expectedLink).Err()
	}
	for i, activatedPolicyID := range bug.ExpectedPolicyIDsActivated {
		expectedContent := fmt.Sprintf("(Policy ID: %s)", activatedPolicyID)
		if !strings.Contains(comments[i+1].Content, expectedContent) {
			return errors.Reason("issue comment %v: got %q, expected it to contain %q", i+2, comments[i+1].Content, expectedContent).Err()
		}
	}
	return nil
}
