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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
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
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// Contains tests for old (non-policy based) bug management.
func TestLegacyUpdate(t *testing.T) {
	Convey("With bug updater", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = memory.Use(ctx)
		ctx = context.WithValue(ctx, &buganizer.BuganizerSelfEmailKey, "email@test.com")

		const project = "chromeos"

		// Has two policies:
		// exoneration-policy:
		// - activation threshold: 100 in one day
		// - deactivation threshold: 10 in one day
		// cls-rejected-policy:
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

		opts := updateOptions{
			uiBaseURL:            "https://luci-analysis-test.appspot.com",
			project:              project,
			analysisClient:       analysisClient,
			buganizerClient:      buganizerClient,
			monorailClient:       monorailClient,
			maxBugsFiledPerRun:   1,
			reclusteringProgress: progress,
			runTimestamp:         time.Date(2100, 2, 2, 2, 2, 2, 2, time.UTC),
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
						"exoneration-policy":  {},
						"cls-rejected-policy": {},
					},
				},
			}
			expectedRules := []*rules.Entry{expectedRule}

			expectedBuganizerBug := buganizerBug{
				ID:            1,
				Component:     projectCfg.Buganizer.DefaultComponent.Id,
				ExpectedTitle: "Failed to connect to 100.1.1.105.",
				// Expect the bug description to contain the top tests.
				ExpectedContent: []string{
					"network-test-1",
					"network-test-2",
				},
			}

			issueCount := func() int {
				return len(buganizerStore.Issues) + len(monorailStore.Issues)
			}

			Convey("bug filing threshold must be met to file a new bug", func() {
				Convey("threshold not met", func() {
					err = updateBugsForProject(ctx, opts)
					So(err, ShouldBeNil)

					// No failure association rules.
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)

					// No issues.
					So(issueCount(), ShouldEqual, 0)
				})
				Convey("1d unexpected failures", func() {
					Convey("Reason cluster", func() {
						Convey("Above threshold", func() {
							suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
							So(issueCount(), ShouldEqual, 1)

							// Further updates do nothing.
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
							So(issueCount(), ShouldEqual, 1)
						})
						Convey("Below threshold", func() {
							suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 99}}

							// Act
							err = updateBugsForProject(ctx, opts)

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
							suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 134}}

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
							So(issueCount(), ShouldEqual, 1)

							// Further updates do nothing.
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
							So(issueCount(), ShouldEqual, 1)
						})
						Convey("Below threshold", func() {
							suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 133}}

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// No bug should be created.
							So(verifyRulesResemble(ctx, nil), ShouldBeNil)
							So(issueCount(), ShouldEqual, 0)
						})
					})
				})
				Convey("3d unexpected failures", func() {
					suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{ThreeDay: metrics.Counts{Residual: 300}}

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)

					// Further updates do nothing.
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)

					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("7d unexpected failures", func() {
					suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{ThreeDay: metrics.Counts{Residual: 700}}

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)

					// Further updates do nothing.
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
			})
			Convey("policies are correctly activated when new bugs are filed", func() {
				// Bug-filing threshold met.
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

				Convey("policy activation threshold not met", func() {
					suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 99}}

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
				Convey("policy activation threshold met", func() {
					suggestedClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}
					expectedRule.BugManagementState.PolicyState["exoneration-policy"].IsActive = true
					expectedRule.BugManagementState.PolicyState["exoneration-policy"].LastActivationTime = timestamppb.New(opts.runTimestamp)

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
			})
			Convey("dispersion criteria must be met to file a new bug", func() {
				// Cluster meets bug-filing threshold.
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
					OneDay: metrics.Counts{Residual: 100},
				}

				Convey("met via User CLs with failures", func() {
					suggestedClusters[1].DistinctUserCLsWithFailures7d.Residual = 3
					suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 0

					// Act
					err = updateBugsForProject(ctx, opts)

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
					err = updateBugsForProject(ctx, opts)

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
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					// No bug should be created.
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
			})
			Convey("duplicate bugs are suppressed", func() {
				Convey("where a rule was recently filed for the same suggested cluster, and reclustering is pending", func() {
					suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

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
					err = updateBugsForProject(ctx, opts)

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
					opts.reclusteringProgress = progress

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify
					So(err, ShouldBeNil)
					expectedBuganizerBug.ID = 2 // Because we already created a bug with ID 1 above.
					expectedRule.BugID.ID = "2"
					So(verifyRulesResemble(ctx, []*rules.Entry{expectedRule, existingRule}), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 2)
				})
				Convey("when re-clustering to new algorithms", func() {
					suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

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
					opts.reclusteringProgress = progress

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
				Convey("when re-clustering to new config", func() {
					suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{OneDay: metrics.Counts{Residual: 100}}

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
					opts.reclusteringProgress = progress

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify no bugs were filed.
					So(err, ShouldBeNil)
					So(verifyRulesResemble(ctx, nil), ShouldBeNil)
					So(issueCount(), ShouldEqual, 0)
				})
			})
			Convey("bugs are routed to the correct issue tracker and component", func() {
				suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
					OneDay: metrics.Counts{Residual: 100},
				}

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
					err = updateBugsForProject(ctx, opts)

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
					err = updateBugsForProject(ctx, opts)

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
					projectCfg.Buganizer = nil
					err = config.SetTestProjectConfig(ctx, projectsCfg)
					So(err, ShouldBeNil)

					// Act
					err = updateBugsForProject(ctx, opts)

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
					projectCfg.Monorail = nil
					err = config.SetTestProjectConfig(ctx, projectsCfg)
					So(err, ShouldBeNil)

					// Act
					err = updateBugsForProject(ctx, opts)

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
					So(projectCfg.BugSystem, ShouldEqual, configpb.BugSystem_BUGANIZER)

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
					err = updateBugsForProject(ctx, opts)

					// Verify we filed into Buganizer.
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					So(expectBuganizerBug(buganizerStore, expectedBuganizerBug), ShouldBeNil)
					So(issueCount(), ShouldEqual, 1)
				})
			})
		})
		Convey("With both failure reason and test name clusters above bug-filing threshold", func() {
			// Reason cluster above the 3-day failure threshold.
			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 400},
				SevenDay: metrics.Counts{Residual: 400},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1

			// Test name cluster with 33% more impact.
			suggestedClusters[1] = makeTestNameCluster(compiledCfg, 3)
			suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 532},
				SevenDay: metrics.Counts{Residual: 532},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			// Limit to one bug filed each time, so that
			// we test change throttling.
			opts.maxBugsFiledPerRun = 1

			Convey("reason clusters preferred over test name clusters", func() {
				// Test name cluster has <34% more impact than the reason
				// cluster.

				// Act
				err = updateBugsForProject(ctx, opts)

				// Verify reason cluster filed.
				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(len(rs), ShouldEqual, 1)
				So(rs[0].SourceCluster, ShouldResemble, suggestedClusters[2].ClusterID)
				So(rs[0].SourceCluster.IsFailureReasonCluster(), ShouldBeTrue)
			})
			Convey("test name clusters can be filed if significantly more impact", func() {
				// Reduce impact of the reason-based cluster so that the
				// test name cluster has >34% more impact than the reason
				// cluster.
				suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
					ThreeDay: metrics.Counts{Residual: 390},
					SevenDay: metrics.Counts{Residual: 390},
				}

				// Act
				err = updateBugsForProject(ctx, opts)

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
			suggestedClusters[0].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 940},
				ThreeDay: metrics.Counts{Residual: 940},
				SevenDay: metrics.Counts{Residual: 940},
			}
			suggestedClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 100},
				ThreeDay: metrics.Counts{Residual: 100},
				SevenDay: metrics.Counts{Residual: 100},
			}
			suggestedClusters[0].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[1] = makeReasonCluster(compiledCfg, 1)
			suggestedClusters[1].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				ThreeDay: metrics.Counts{Residual: 300},
				SevenDay: metrics.Counts{Residual: 300},
			}
			suggestedClusters[1].PostsubmitBuildsWithFailures7d.Residual = 1

			suggestedClusters[2] = makeReasonCluster(compiledCfg, 2)
			suggestedClusters[2].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
				OneDay:   metrics.Counts{Residual: 250},
				ThreeDay: metrics.Counts{Residual: 250},
				SevenDay: metrics.Counts{Residual: 250},
			}
			suggestedClusters[2].PostsubmitBuildsWithFailures7d.Residual = 1
			suggestedClusters[2].TopMonorailComponents = []analysis.TopCount{
				{Value: "Monorail", Count: 250},
			}

			suggestedClusters[3] = makeReasonCluster(compiledCfg, 3)
			suggestedClusters[3].MetricValues[metrics.Failures.ID] = metrics.TimewiseCounts{
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
								LastActivationTime: timestamppb.New(opts.runTimestamp),
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
							"exoneration-policy":  {},
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
							"exoneration-policy":  {},
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
							"exoneration-policy":  {},
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
			opts.maxBugsFiledPerRun = 1

			// Verify one bug is filed at a time.
			for i := 0; i < len(expectedRules); i++ {
				// Act
				err = updateBugsForProject(ctx, opts)

				// Verify
				So(err, ShouldBeNil)
				So(verifyRulesResemble(ctx, expectedRules[:i+1]), ShouldBeNil)
			}

			// Further updates do nothing.
			err = updateBugsForProject(ctx, opts)

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

				Convey("negligable cluster impact does not affect issue priority or status", func() {
					issue := buganizerStore.Issues[1]
					originalPriority := issue.Issue.IssueState.Priority
					originalStatus := issue.Issue.IssueState.Status
					So(originalStatus, ShouldNotEqual, issuetracker.Issue_VERIFIED)

					SetResidualMetrics(bugClusters[1], bugs.ClosureImpact())

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify.
					So(err, ShouldBeNil)
					So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
					So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
					So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
				})
				Convey("policy activation is unchanged", func() {
					// The policy should already be active from previous setup.
					So(expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive, ShouldBeTrue)

					// Update metrics so that policy should de-activate if
					// reclustering was complete.
					bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{}

					// Act
					err = updateBugsForProject(ctx, opts)

					// Verify.
					So(err, ShouldBeNil)
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
						WithRulesVersion(rs[2].PredicateLastUpdateTime).
						WithCompletedProgress().Build(),
				})
				So(err, ShouldBeNil)

				progress, err := runs.ReadReclusteringProgress(ctx, project)
				So(err, ShouldBeNil)
				opts.reclusteringProgress = progress

				opts.runTimestamp = opts.runTimestamp.Add(10 * time.Minute)

				Convey("policy activation", func() {
					// Verify updates work, even when rules are in later batches.
					opts.updateRuleBatchSize = 1

					Convey("policy remains inactive if activation threshold unmet", func() {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["exoneration-policy"]
						So(expectedPolicyState.IsActive, ShouldBeFalse)

						// Set metrics just below the policy activation threshold.
						bugClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 99},
							ThreeDay: metrics.Counts{Residual: 99},
							SevenDay: metrics.Counts{Residual: 99},
						}

						// Act
						err = updateBugsForProject(ctx, opts)

						// Verify policy activation unchanged.
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy activates if activation threshold met", func() {
						// The policy should be inactive from previous setup.
						expectedPolicyState := expectedRules[1].BugManagementState.PolicyState["exoneration-policy"]
						So(expectedPolicyState.IsActive, ShouldBeFalse)

						// Update metrics so that policy should activate.
						bugClusters[1].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{
							OneDay:   metrics.Counts{Residual: 100},
							ThreeDay: metrics.Counts{Residual: 100},
							SevenDay: metrics.Counts{Residual: 100},
						}

						// Act
						err = updateBugsForProject(ctx, opts)

						// Verify policy activates.
						So(err, ShouldBeNil)
						expectedPolicyState.IsActive = true
						expectedPolicyState.LastActivationTime = timestamppb.New(opts.runTimestamp)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
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
						err = updateBugsForProject(ctx, opts)

						// Verify policy activation should be unchanged.
						So(err, ShouldBeNil)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy deactivates if deactivation threshold met", func() {
						// The policy should already be active from previous setup.
						expectedPolicyState := expectedRules[0].BugManagementState.PolicyState["exoneration-policy"]
						So(expectedPolicyState.IsActive, ShouldBeTrue)

						// Update metrics so that policy should de-activate.
						bugClusters[0].MetricValues[metrics.CriticalFailuresExonerated.ID] = metrics.TimewiseCounts{}

						// Act
						err = updateBugsForProject(ctx, opts)

						// Verify policy deactivated.
						So(err, ShouldBeNil)
						expectedPolicyState.IsActive = false
						expectedPolicyState.LastDeactivationTime = timestamppb.New(opts.runTimestamp)
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("policy configuration changes are handled", func() {
						// Delete the existing policy named "exoneration-policy", and replace it with a new policy,
						// "new-exoneration-policy". Activation and de-activation criteria remain the same.
						projectCfg.BugManagement.Policies[0].Id = "new-exoneration-policy"

						// Act
						err = updateBugsForProject(ctx, opts)

						// Verify state for the old policy is deleted, and state for the new policy is added.
						So(err, ShouldBeNil)
						expectedRules[0].BugManagementState.PolicyState = map[string]*bugspb.BugManagementState_PolicyState{
							"new-exoneration-policy": {
								// The new policy should activate, because the metrics justify its activation.
								IsActive:           true,
								LastActivationTime: timestamppb.New(opts.runTimestamp),
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
				Convey("priority updates", func() {
					Convey("buganizer", func() {
						// Select a Buganizer issue.
						issue := buganizerStore.Issues[1]
						originalPriority := issue.Issue.IssueState.Priority
						originalStatus := issue.Issue.IssueState.Status
						So(originalStatus, ShouldEqual, issuetracker.Issue_NEW)

						// Get the corresponding rule, confirming we got the right one.
						rule := rs[0]
						So(rule.BugID.ID, ShouldEqual, fmt.Sprintf("%v", issue.Issue.IssueId))

						// Increase cluster impact to P0.
						So(originalPriority, ShouldNotEqual, issuetracker.Issue_P0)
						SetResidualMetrics(bugClusters[0], bugs.P0Impact())

						Convey("increasing cluster impact to P0 increases issue priority", func() {
							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("no bug filing thresholds, but still update existing bug priority", func() {
							projectCfg.BugFilingThresholds = nil

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P0)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("decreasing cluster impact to P3 decreases issue priority", func() {
							// Reduce cluster impact to P3.
							So(originalPriority, ShouldNotEqual, issuetracker.Issue_P3)
							SetResidualMetrics(bugClusters[0], bugs.P3Impact())

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_NEW)
							So(issue.Issue.IssueState.Priority, ShouldEqual, issuetracker.Issue_P3)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("disabling IsManagingBug prevents priority updates", func() {
							// Set IsManagingBug to false on the rule.
							rule.IsManagingBug = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
							So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)

							// Check the rules have not changed except for the IsManagingBug change.
							expectedRules[0].IsManagingBug = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("disabling IsManagingBugPriority prevents priority updates", func() {
							// Set IsManagingBugPriority to false on the rule.
							rule.IsManagingBugPriority = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = updateBugsForProject(ctx, opts)

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

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
							So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
							expectedRules[0].IsManagingBugPriority = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							Convey("further updates leave no comments", func() {
								initialComments := len(issue.Comments)

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								So(len(issue.Comments), ShouldEqual, initialComments)
								So(issue.Issue.IssueState.Status, ShouldEqual, originalStatus)
								So(issue.Issue.IssueState.Priority, ShouldEqual, originalPriority)
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							})
						})
						Convey("cluster disappearing closes issue", func() {
							// Drop the corresponding bug cluster. This is consistent with
							// no more failures in the cluster occuring.
							bugClusters = bugClusters[1:]
							analysisClient.clusters = append(suggestedClusters, bugClusters...)

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].IsActive = false
							expectedRules[0].BugManagementState.PolicyState["exoneration-policy"].LastDeactivationTime = timestamppb.New(opts.runTimestamp)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)

							Convey("rule automatically archived after 30 days", func() {
								tc.Add(time.Hour * 24 * 30)

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[0].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_VERIFIED)
							})
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

						// Increase cluster impact to P0.
						So(originalPriority, ShouldNotEqual, "0")
						SetResidualMetrics(bugClusters[ruleIndex], bugs.P0Impact())

						Convey("increasing cluster impact to P0 increases issue priority", func() {
							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "0")
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("no bug filing thresholds, but still update existing bug priority", func() {
							projectCfg.BugFilingThresholds = nil

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "0")
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("decreasing cluster impact to P3 decreases issue priority", func() {
							// Reduce cluster impact to P3.
							So(originalPriority, ShouldNotEqual, issuetracker.Issue_P3)
							SetResidualMetrics(bugClusters[ruleIndex], bugs.P3Impact())

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, "3")
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("disabling IsManagingBug prevents priority updates", func() {
							// Set IsManagingBug to false on the rule.
							rule.IsManagingBug = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)

							// Check the rules have not changed except for the IsManagingBug change.
							expectedRules[ruleIndex].IsManagingBug = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("disabling IsManagingBugPriority prevents priority updates", func() {
							// Set IsManagingBugPriority to false on the rule.
							rule.IsManagingBugPriority = false
							So(rules.SetForTesting(ctx, rs), ShouldBeNil)

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Check that the bug priority and status has not changed.
							So(issue.Issue.Status.Status, ShouldEqual, originalStatus)
							So(monorail.ChromiumTestIssuePriority(issue.Issue), ShouldEqual, originalPriority)

							// Check the rules have not changed except for the IsManagingBugPriority change.
							expectedRules[ruleIndex].IsManagingBugPriority = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
						})
						Convey("cluster disappearing closes issue", func() {
							// Drop the corresponding bug cluster. This is consistent with
							// no more failures in the cluster occuring.
							newBugClusters := []*analysis.Cluster{}
							newBugClusters = append(newBugClusters, bugClusters[0:ruleIndex]...)
							newBugClusters = append(newBugClusters, bugClusters[ruleIndex+1:]...)
							bugClusters = newBugClusters
							analysisClient.clusters = append(suggestedClusters, bugClusters...)

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
							So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)

							Convey("rule automatically archived after 30 days", func() {
								tc.Add(time.Hour * 24 * 30)

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[ruleIndex].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
								So(issue.Issue.Status.Status, ShouldEqual, monorail.VerifiedStatus)
							})
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
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, 2)
							So(issueOne.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[1].Comment, ShouldContainSubstring, expectedRules[2].RuleID)

							So(issueTwo.Comments, ShouldHaveLength, 2)
							So(issueTwo.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
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
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							expectedRules[0].IsActive = false
							expectedRules[1].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, 1)
							So(issueTwo.Comments, ShouldHaveLength, 1)
						})
						Convey("happy path, bug marked as duplicate of bug without a rule in this project", func() {
							// Setup
							issueOne.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
							issueOne.Issue.IssueState.CanonicalIssueId = 1234

							Convey("bug managed by a rule in another project", func() {
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
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[0].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
								expectedRules[0].IsManagingBug = false // The other rule should continue to manage the bug.
								for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
									// Should reset because of the change in associated bug.
									policyState.ActivationNotified = false
								}
								expectedRules = append(expectedRules, extraRule)
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								So(issueOne.Comments, ShouldHaveLength, 2)
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, expectedRules[0].RuleID)
							})
							Convey("bug not managed by a rule in any project", func() {
								buganizerStore.StoreIssue(ctx, buganizer.NewFakeIssue(1234))

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)
								expectedRules[0].BugID = bugs.BugID{System: bugs.BuganizerSystem, ID: "1234"}
								expectedRules[0].IsManagingBug = true
								for _, policyState := range expectedRules[0].BugManagementState.PolicyState {
									// Should reset because of the change in associated bug.
									policyState.ActivationNotified = false
								}
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								So(issueOne.Comments, ShouldHaveLength, 2)
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, expectedRules[1].RuleID)
							})
						})
						Convey("error cases", func() {
							Convey("bugs are in a duplicate bug cycle", func() {
								// Note that this is a simple cycle with only two bugs.
								// The implementation allows for larger cycles, however.
								issueTwo.Issue.IssueState.Status = issuetracker.Issue_DUPLICATE
								issueTwo.Issue.IssueState.CanonicalIssueId = issueOne.Issue.IssueId

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)

								// Issue one kicked out of duplicate status.
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

								// As the cycle is now broken, issue two is merged into
								// issue one.
								expectedRules[0].RuleDefinition = "reason LIKE \"want foo, got bar\" OR\ntest = \"testname-0\""
								expectedRules[1].IsActive = false
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								So(issueOne.Comments, ShouldHaveLength, 3)
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
								So(issueOne.Comments[2].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
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
								err = updateBugsForProject(ctx, opts)

								// Verify
								So(err, ShouldBeNil)

								// Rules should not have changed (except for the update we made).
								expectedRules[0].RuleDefinition = longRule
								So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

								// Issue one kicked out of duplicate status.
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)

								// Comment should appear on the bug.
								So(issueOne.Comments, ShouldHaveLength, 2)
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, "the merged failure association rule would be too long")
							})
							Convey("bug marked as duplicate of bug we cannot access", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify issue one kicked out of duplicate status.
								So(err, ShouldBeNil)
								So(issueOne.Issue.IssueState.Status, ShouldNotEqual, issuetracker.Issue_DUPLICATE)
								So(issueOne.Comments, ShouldHaveLength, 2)
								So(issueOne.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis cannot merge the association rule for this bug into the rule")
							})
							Convey("failed to handle duplicate bug - bug has an assignee", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has an assignee.
								issueOne.Issue.IssueState.Assignee = &issuetracker.User{
									EmailAddress: "user@google.com",
								}
								// Act
								err = updateBugsForProject(ctx, opts)

								// Verify issue is put back to assigned status, instead of New.
								So(err, ShouldBeNil)
								So(issueOne.Issue.IssueState.Status, ShouldEqual, issuetracker.Issue_ASSIGNED)
							})
							Convey("failed to handle duplicate bug - bug has no assignee", func() {
								issueTwo.ShouldReturnAccessPermissionError = true

								// Has no assignee.
								issueOne.Issue.IssueState.Assignee = nil

								// Act
								err = updateBugsForProject(ctx, opts)

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

						Convey("happy path", func() {
							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							issueOneRule.IsActive = false
							issueTwoRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\nreason LIKE \"want foofoofoo, got bar\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, 3)
							So(issueOne.Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[2].Content, ShouldContainSubstring, issueOneRule.RuleID)

							So(issueTwo.Comments, ShouldHaveLength, 3)
							So(issueTwo.Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
						})
						Convey("error case", func() {
							// Note that this is a simple cycle with only two bugs.
							// The implementation allows for larger cycles, however.
							issueTwo.Issue.Status.Status = monorail.DuplicateStatus
							issueTwo.Issue.MergedIntoIssueRef = &mpb.IssueRef{
								Issue: issueOne.Issue.Name,
							}

							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)

							// Issue one kicked out of duplicate status.
							So(issueOne.Issue.Status.Status, ShouldNotEqual, monorail.DuplicateStatus)

							// As the cycle is now broken, issue two is merged into
							// issue one.
							issueOneRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\nreason LIKE \"want foofoofoo, got bar\""
							issueTwoRule.IsActive = false
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, 4)
							So(issueOne.Comments[2].Content, ShouldContainSubstring, "a cycle was detected in the bug merged-into graph")
							So(issueOne.Comments[3].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
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

						Convey("happy path", func() {
							// Act
							err = updateBugsForProject(ctx, opts)

							// Verify
							So(err, ShouldBeNil)
							issueOneRule.IsActive = false
							issueTwoRule.RuleDefinition = "reason LIKE \"want foofoo, got bar\" OR\ntest = \"testname-0\""
							So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)

							So(issueOne.Comments, ShouldHaveLength, 3)
							So(issueOne.Comments[2].Content, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for this bug into the rule for the canonical bug.")
							So(issueOne.Comments[2].Content, ShouldContainSubstring, issueOneRule.RuleID)

							So(issueTwo.Comments, ShouldHaveLength, 2)
							So(issueTwo.Comments[1].Comment, ShouldContainSubstring, "LUCI Analysis has merged the failure association rule for that bug into the rule for this bug.")
						})
					})
				})
				Convey("bug marked as archived should archive rule", func() {
					Convey("buganizer", func() {
						issueOne := buganizerStore.Issues[1].Issue
						issueOne.IsArchived = true

						// Act
						err = updateBugsForProject(ctx, opts)
						So(err, ShouldBeNil)

						// Verify
						expectedRules[0].IsActive = false
						So(verifyRulesResemble(ctx, expectedRules), ShouldBeNil)
					})
					Convey("monorail", func() {
						issue := monorailStore.Issues[0]
						issue.Issue.Status.Status = "Archived"

						// Act
						err = updateBugsForProject(ctx, opts)
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
