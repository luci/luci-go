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

package rpc

import (
	"context"
	"encoding/hex"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestClusters(t *testing.T) {
	ftt.Run("With a clusters server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)
		analysisClient := newFakeAnalysisClient()
		server := NewClustersServer(analysisClient)

		configVersion := time.Date(2025, time.August, 12, 0, 1, 2, 3, time.UTC)
		projectCfg := config.CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)
		projectCfg.LastUpdated = timestamppb.New(configVersion)
		projectCfg.BugManagement.Monorail.DisplayPrefix = "crbug.com"
		projectCfg.BugManagement.Monorail.MonorailHostname = "bugs.chromium.org"
		configs := make(map[string]*configpb.ProjectConfig)
		configs["testproject"] = projectCfg
		err := config.SetTestProjectConfig(ctx, configs)
		assert.Loosely(t, err, should.BeNil)

		compiledTestProjectCfg, err := compiledcfg.NewConfig(projectCfg)
		assert.Loosely(t, err, should.BeNil)

		// Rules version is in microsecond granularity, consistent with
		// the granularity of Spanner commit timestamps.
		rulesVersion := time.Date(2021, time.February, 12, 1, 2, 4, 5000, time.UTC)
		rs := []*rules.Entry{
			rules.NewRule(0).
				WithProject("testproject").
				WithRuleDefinition(`test LIKE "%TestSuite.TestName%"`).
				WithPredicateLastUpdateTime(rulesVersion.Add(-1 * time.Hour)).
				WithBug(bugs.BugID{
					System: "monorail",
					ID:     "chromium/7654321",
				}).Build(),
			rules.NewRule(1).
				WithProject("testproject").
				WithRuleDefinition(`reason LIKE "my_file.cc(%): Check failed: false."`).
				WithPredicateLastUpdateTime(rulesVersion).
				WithBug(bugs.BugID{
					System: "buganizer",
					ID:     "82828282",
				}).Build(),
			rules.NewRule(2).
				WithProject("testproject").
				WithRuleDefinition(`test LIKE "%Other%"`).
				WithPredicateLastUpdateTime(rulesVersion.Add(-2 * time.Hour)).
				WithBug(bugs.BugID{
					System: "monorail",
					ID:     "chromium/912345",
				}).Build(),
		}
		err = rules.SetForTesting(ctx, t, rs)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Unauthorized requests are rejected", func(t *ftt.Test) {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.ClusterRequest{
				Project: "testproject",
			}

			rule, err := server.Cluster(ctx, request)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("not a member of luci-analysis-access"))
			assert.Loosely(t, rule, should.BeNil)
		})
		t.Run("Cluster", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetClustersByFailure,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRule,
				},
			}

			request := &pb.ClusterRequest{
				Project: "testproject",
				TestResults: []*pb.ClusterRequest_TestResult{
					{
						RequestTag: "my tag 1",
						TestId:     "ninja://chrome/test:interactive_ui_tests/TestSuite.TestName",
						FailureReason: &pb.FailureReason{
							PrimaryErrorMessage: "my_file.cc(123): Check failed: false.",
						},
					},
					{
						RequestTag: "my tag 2",
						TestId:     "Other_test",
					},
				},
			}
			t.Run("Not authorised to cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetClustersByFailure)

				response, err := server.Cluster(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.getByFailure"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to get rule", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRule)

				response, err := server.Cluster(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.rules.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				// Run
				response, err := server.Cluster(ctx, request)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Resemble(&pb.ClusterResponse{
					ClusteredTestResults: []*pb.ClusterResponse_ClusteredTestResult{
						{
							RequestTag: "my tag 1",
							Clusters: sortClusterEntries([]*pb.ClusterResponse_ClusteredTestResult_ClusterEntry{
								{
									ClusterId: &pb.ClusterId{
										Algorithm: "rules",
										Id:        rs[0].RuleID,
									},
									Bug: &pb.AssociatedBug{
										System:   "monorail",
										Id:       "chromium/7654321",
										LinkText: "crbug.com/7654321",
										Url:      "https://bugs.chromium.org/p/chromium/issues/detail?id=7654321",
									},
								}, {
									ClusterId: &pb.ClusterId{
										Algorithm: "rules",
										Id:        rs[1].RuleID,
									},
									Bug: &pb.AssociatedBug{
										System:   "buganizer",
										Id:       "82828282",
										LinkText: "b/82828282",
										Url:      "https://issuetracker.google.com/issues/82828282",
									},
								},
								failureReasonClusterEntry(compiledTestProjectCfg, "my_file.cc(123): Check failed: false."),
								testNameClusterEntry(compiledTestProjectCfg, "ninja://chrome/test:interactive_ui_tests/TestSuite.TestName"),
							}),
						},
						{
							RequestTag: "my tag 2",
							Clusters: sortClusterEntries([]*pb.ClusterResponse_ClusteredTestResult_ClusterEntry{
								{
									ClusterId: &pb.ClusterId{
										Algorithm: "rules",
										Id:        rs[2].RuleID,
									},
									Bug: &pb.AssociatedBug{
										System:   "monorail",
										Id:       "chromium/912345",
										LinkText: "crbug.com/912345",
										Url:      "https://bugs.chromium.org/p/chromium/issues/detail?id=912345",
									},
								},
								testNameClusterEntry(compiledTestProjectCfg, "Other_test"),
							}),
						},
					},
					ClusteringVersion: &pb.ClusteringVersion{
						AlgorithmsVersion: algorithms.AlgorithmsVersion,
						RulesVersion:      timestamppb.New(rulesVersion),
						ConfigVersion:     timestamppb.New(configVersion),
					},
				}))
			})
			t.Run("With no monorail configuration", func(t *ftt.Test) {
				// Setup
				projectCfg.BugManagement.Monorail = nil
				configs := make(map[string]*configpb.ProjectConfig)
				configs["testproject"] = projectCfg
				err := config.SetTestProjectConfig(ctx, configs)
				assert.Loosely(t, err, should.BeNil)
				// Run
				response, err := server.Cluster(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response.ClusteredTestResults[0].Clusters[1].Bug.Url, should.BeEmpty)
			})
			t.Run("With missing test ID", func(t *ftt.Test) {
				request.TestResults[1].TestId = ""

				// Run
				response, err := server.Cluster(ctx, request)

				// Verify
				assert.Loosely(t, response, should.BeNil)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("test result 1: test ID must not be empty"))
			})
			t.Run("With too many test results", func(t *ftt.Test) {
				var testResults []*pb.ClusterRequest_TestResult
				for i := 0; i < 1001; i++ {
					testResults = append(testResults, &pb.ClusterRequest_TestResult{
						TestId: "AnotherTest",
					})
				}
				request.TestResults = testResults

				// Run
				response, err := server.Cluster(ctx, request)

				// Verify
				assert.Loosely(t, response, should.BeNil)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("too many test results: at most 1000 test results can be clustered in one request"))
			})
			t.Run("With project not configured", func(t *ftt.Test) {
				err := config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})
				assert.Loosely(t, err, should.BeNil)

				// Run
				response, err := server.Cluster(ctx, request)

				// Verify
				assert.That(t, response.ClusteringVersion.ConfigVersion.AsTime(), should.Match(config.StartingEpoch))
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("Get", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetCluster,
				},
				{
					Realm:      "testproject:realm1",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "testproject:realm3",
					Permission: rdbperms.PermListTestResults,
				},
			}

			example := &clustering.Failure{
				TestID: "TestID_Example",
				Reason: &pb.FailureReason{
					PrimaryErrorMessage: "Example failure reason 123.",
				},
			}
			a := &failurereason.Algorithm{}
			reasonClusterID := a.Cluster(compiledTestProjectCfg, example)

			analysisClient.clustersByProject["testproject"] = []*analysis.Cluster{}

			request := &pb.GetClusterRequest{
				Name: "projects/testproject/clusters/rules/22222200000000000000000000000000",
			}

			t.Run("Not authorised to get cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetCluster)

				response, err := server.Get(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				analysisClient.clustersByProject["testproject"] = []*analysis.Cluster{
					{
						ClusterID: clustering.ClusterID{
							Algorithm: rulesalgorithm.AlgorithmName,
							ID:        "11111100000000000000000000000000",
						},
						MetricValues: map[metrics.ID]metrics.TimewiseCounts{
							metrics.HumanClsFailedPresubmit.ID: {
								OneDay:   metrics.Counts{Nominal: 1},
								ThreeDay: metrics.Counts{Nominal: 2},
								SevenDay: metrics.Counts{Nominal: 3},
							},
							metrics.CriticalFailuresExonerated.ID: {
								OneDay:   metrics.Counts{Nominal: 4},
								ThreeDay: metrics.Counts{Nominal: 5},
								SevenDay: metrics.Counts{Nominal: 6},
							},
							metrics.Failures.ID: {
								OneDay:   metrics.Counts{Nominal: 7},
								ThreeDay: metrics.Counts{Nominal: 8},
								SevenDay: metrics.Counts{Nominal: 9},
							},
						},
						DistinctUserCLsWithFailures7d:  metrics.Counts{Nominal: 13},
						PostsubmitBuildsWithFailures7d: metrics.Counts{Nominal: 14},
						ExampleFailureReason:           bigquery.NullString{Valid: true, StringVal: "Example failure reason."},
						TopTestIDs: []analysis.TopCount{
							{Value: "TestID 1", Count: 2},
							{Value: "TestID 2", Count: 1},
						},
						Realms: []string{"testproject:realm1", "testproject:realm2"},
					},
				}
				request := &pb.GetClusterRequest{
					Name: "projects/testproject/clusters/rules/11111100000000000000000000000000",
				}
				expectedResponse := &pb.Cluster{
					Name:       "projects/testproject/clusters/rules/11111100000000000000000000000000",
					HasExample: true,
					Metrics: map[string]*pb.Cluster_TimewiseCounts{
						metrics.HumanClsFailedPresubmit.ID.String(): {
							OneDay:   &pb.Cluster_Counts{Nominal: 1},
							ThreeDay: &pb.Cluster_Counts{Nominal: 2},
							SevenDay: &pb.Cluster_Counts{Nominal: 3},
						},
						metrics.CriticalFailuresExonerated.ID.String(): {
							OneDay:   &pb.Cluster_Counts{Nominal: 4},
							ThreeDay: &pb.Cluster_Counts{Nominal: 5},
							SevenDay: &pb.Cluster_Counts{Nominal: 6},
						},
						metrics.Failures.ID.String(): {
							OneDay:   &pb.Cluster_Counts{Nominal: 7},
							ThreeDay: &pb.Cluster_Counts{Nominal: 8},
							SevenDay: &pb.Cluster_Counts{Nominal: 9},
						},
					},
				}

				t.Run("Rule with clustered failures", func(t *ftt.Test) {
					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("Rule without clustered failures", func(t *ftt.Test) {
					analysisClient.clustersByProject["testproject"] = []*analysis.Cluster{}

					expectedResponse.HasExample = false
					expectedResponse.Metrics = map[string]*pb.Cluster_TimewiseCounts{}
					for _, metric := range metrics.ComputedMetrics {
						expectedResponse.Metrics[metric.ID.String()] = emptyMetricValues()
					}

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("Suggested cluster with example failure matching cluster definition", func(t *ftt.Test) {
					// Suggested cluster for which there are clustered failures, and
					// the cluster ID matches the example provided for the cluster.
					analysisClient.clustersByProject["testproject"] = []*analysis.Cluster{
						{
							ClusterID: clustering.ClusterID{
								Algorithm: failurereason.AlgorithmName,
								ID:        hex.EncodeToString(reasonClusterID),
							},
							MetricValues: map[metrics.ID]metrics.TimewiseCounts{
								metrics.HumanClsFailedPresubmit.ID: {
									SevenDay: metrics.Counts{Nominal: 15},
								},
							},
							ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: "Example failure reason 123."},
							TopTestIDs: []analysis.TopCount{
								{Value: "TestID_Example", Count: 10},
							},
							Realms: []string{"testproject:realm1", "testproject:realm3"},
						},
					}

					request := &pb.GetClusterRequest{
						Name: "projects/testproject/clusters/" + failurereason.AlgorithmName + "/" + hex.EncodeToString(reasonClusterID),
					}
					expectedResponse := &pb.Cluster{
						Name:       "projects/testproject/clusters/" + failurereason.AlgorithmName + "/" + hex.EncodeToString(reasonClusterID),
						Title:      "Example failure reason %.",
						HasExample: true,
						Metrics: map[string]*pb.Cluster_TimewiseCounts{
							metrics.HumanClsFailedPresubmit.ID.String(): {
								OneDay:   &pb.Cluster_Counts{},
								ThreeDay: &pb.Cluster_Counts{},
								SevenDay: &pb.Cluster_Counts{Nominal: 15},
							},
						},
						EquivalentFailureAssociationRule: `reason LIKE "Example failure reason %."`,
					}

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))

					t.Run("No test result list permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

						// Run
						response, err := server.Get(ctx, request)

						// Verify
						expectedResponse.Title = ""
						expectedResponse.EquivalentFailureAssociationRule = ""
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response, should.Resemble(expectedResponse))
					})
				})
				t.Run("Suggested cluster with example failure not matching cluster definition", func(t *ftt.Test) {
					// Suggested cluster for which there are clustered failures,
					// but cluster ID mismatches the example provided for the cluster.
					// This could be because clustering configuration has changed and
					// re-clustering is not yet complete.
					analysisClient.clustersByProject["testproject"] = []*analysis.Cluster{
						{
							ClusterID: clustering.ClusterID{
								Algorithm: testname.AlgorithmName,
								ID:        "cccccc00000000000000000000000001",
							},
							MetricValues: map[metrics.ID]metrics.TimewiseCounts{
								metrics.HumanClsFailedPresubmit.ID: {
									SevenDay: metrics.Counts{Nominal: 11},
								},
							},
							ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: "Example failure reason 2."},
							TopTestIDs: []analysis.TopCount{
								{Value: "TestID 3", Count: 2},
							},
							Realms: []string{"testproject:realm2", "testproject:realm3"},
						},
					}

					request := &pb.GetClusterRequest{
						Name: "projects/testproject/clusters/" + testname.AlgorithmName + "/cccccc00000000000000000000000001",
					}
					expectedResponse := &pb.Cluster{
						Name:       "projects/testproject/clusters/" + testname.AlgorithmName + "/cccccc00000000000000000000000001",
						Title:      "(definition unavailable due to ongoing reclustering)",
						HasExample: true,
						Metrics: map[string]*pb.Cluster_TimewiseCounts{
							metrics.HumanClsFailedPresubmit.ID.String(): {
								OneDay:   &pb.Cluster_Counts{},
								ThreeDay: &pb.Cluster_Counts{},
								SevenDay: &pb.Cluster_Counts{Nominal: 11},
							},
						},
						EquivalentFailureAssociationRule: ``,
					}

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("Suggested cluster without clustered failures", func(t *ftt.Test) {
					// Suggested cluster for which no impact data exists.
					request := &pb.GetClusterRequest{
						Name: "projects/testproject/clusters/reason-v3/cccccc0000000000000000000000ffff",
					}
					expectedResponse := &pb.Cluster{
						Name:       "projects/testproject/clusters/reason-v3/cccccc0000000000000000000000ffff",
						HasExample: false,
						Metrics:    map[string]*pb.Cluster_TimewiseCounts{},
					}
					for _, metric := range metrics.ComputedMetrics {
						expectedResponse.Metrics[metric.ID.String()] = emptyMetricValues()
					}

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("With project not configured", func(t *ftt.Test) {
					err := config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})
					assert.Loosely(t, err, should.BeNil)

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, response, should.Resemble(expectedResponse))
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("With invalid request", func(t *ftt.Test) {
				t.Run("No name specified", func(t *ftt.Test) {
					request.Name = ""

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: must be specified"))
				})
				t.Run("Invalid name", func(t *ftt.Test) {
					request.Name = "invalid"

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: invalid cluster name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}"))
				})
				t.Run("Invalid cluster algorithm in name", func(t *ftt.Test) {
					request.Name = "projects/blah/clusters/reason/cccccc00000000000000000000000001"

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: invalid cluster identity: algorithm not valid"))
				})
				t.Run("Invalid cluster ID in name", func(t *ftt.Test) {
					request.Name = "projects/blah/clusters/reason-v3/123"

					// Run
					response, err := server.Get(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: invalid cluster identity: ID is not valid lowercase hexadecimal bytes"))
				})
			})
		})
		t.Run("QueryClusterSummaries", func(t *ftt.Test) {
			authState.IdentityPermissions = listTestResultsPermissions(
				"testproject:realm1",
				"testproject:realm2",
				"otherproject:realm3",
			)
			authState.IdentityPermissions = append(authState.IdentityPermissions, []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermListClusters,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}...)

			analysisClient.clusterMetricsByProject["testproject"] = []*analysis.ClusterSummary{
				{
					ClusterID: clustering.ClusterID{
						Algorithm: rulesalgorithm.AlgorithmName,
						ID:        rs[0].RuleID,
					},
					MetricValues: map[metrics.ID]*analysis.MetricValue{
						metrics.HumanClsFailedPresubmit.ID: {
							Value:          1,
							DailyBreakdown: []int64{1, 0, 0, 0, 0, 0, 0},
						},
						metrics.CriticalFailuresExonerated.ID: {
							Value:          2,
							DailyBreakdown: []int64{1, 1, 0, 0, 0, 0, 0},
						},
						metrics.Failures.ID: {
							Value:          3,
							DailyBreakdown: []int64{1, 1, 1, 0, 0, 0, 0},
						},
					},
					ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: "Example failure reason."},
					ExampleTestID:        "TestID 1",
				},
				{
					ClusterID: clustering.ClusterID{
						Algorithm: "reason-v3",
						ID:        "cccccc00000000000000000000000001",
					},
					MetricValues: map[metrics.ID]*analysis.MetricValue{
						metrics.HumanClsFailedPresubmit.ID: {
							Value:          4,
							DailyBreakdown: []int64{1, 1, 1, 1, 0, 0, 0},
						},
						metrics.CriticalFailuresExonerated.ID: {
							Value:          5,
							DailyBreakdown: []int64{1, 1, 1, 1, 1, 0, 0},
						},
						metrics.Failures.ID: {
							Value:          6,
							DailyBreakdown: []int64{1, 1, 1, 1, 1, 1, 0},
						},
					},
					ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: "Example failure reason 2."},
					ExampleTestID:        "TestID 3",
				},
				{
					ClusterID: clustering.ClusterID{
						// Rule that is no longer active.
						Algorithm: rulesalgorithm.AlgorithmName,
						ID:        "01234567890abcdef01234567890abcdef",
					},
					MetricValues: map[metrics.ID]*analysis.MetricValue{
						metrics.HumanClsFailedPresubmit.ID: {
							Value:          7,
							DailyBreakdown: []int64{1, 1, 1, 1, 1, 1, 1},
						},
						metrics.CriticalFailuresExonerated.ID: {
							Value:          8,
							DailyBreakdown: []int64{2, 1, 1, 1, 1, 1, 1},
						},
						metrics.Failures.ID: {
							Value:          9,
							DailyBreakdown: []int64{2, 2, 1, 1, 1, 1, 1},
						},
					},
					ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: "Example failure reason."},
					ExampleTestID:        "TestID 1",
				},
			}
			analysisClient.expectedRealmsQueried = []string{"testproject:realm1", "testproject:realm2"}

			now := clock.Now(ctx)
			request := &pb.QueryClusterSummariesRequest{
				Project:       "testproject",
				FailureFilter: "test_id:\"pita.Boot\" failure_reason:\"failed to boot\"",
				OrderBy:       "metrics.`human-cls-failed-presubmit`.value desc, metrics.`critical-failures-exonerated`.value desc, metrics.failures.value desc",
				Metrics: []string{
					"projects/testproject/metrics/human-cls-failed-presubmit",
					"projects/testproject/metrics/critical-failures-exonerated",
					"projects/testproject/metrics/failures",
				},
				TimeRange: &pb.TimeRange{
					Earliest: timestamppb.New(now.Add(-24 * time.Hour)),
					Latest:   timestamppb.New(now),
				},
			}
			t.Run("Invalid time range", func(t *ftt.Test) {
				request.TimeRange = &pb.TimeRange{
					Earliest: timestamppb.New(now),
					Latest:   timestamppb.New(now.Add(-24 * time.Hour)),
				}

				response, err := server.QueryClusterSummaries(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("time_range: earliest must be before latest"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list clusters", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListClusters)

				response, err := server.QueryClusterSummaries(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.list"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to get rules", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRule)

				response, err := server.QueryClusterSummaries(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.rules.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test results in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

				response, err := server.QueryClusterSummaries(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test exonerations in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)

				response, err := server.QueryClusterSummaries(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Valid request", func(t *ftt.Test) {
				expectedResponse := &pb.QueryClusterSummariesResponse{
					ClusterSummaries: []*pb.ClusterSummary{
						{
							ClusterId: &pb.ClusterId{
								Algorithm: "rules",
								Id:        rs[0].RuleID,
							},
							Title: rs[0].RuleDefinition,
							Bug: &pb.AssociatedBug{
								System:   "monorail",
								Id:       "chromium/7654321",
								LinkText: "crbug.com/7654321",
								Url:      "https://bugs.chromium.org/p/chromium/issues/detail?id=7654321",
							},
							Metrics: map[string]*pb.ClusterSummary_MetricValue{
								metrics.HumanClsFailedPresubmit.ID.String(): {
									Value: 1,
								},
								metrics.CriticalFailuresExonerated.ID.String(): {
									Value: 2,
								},
								metrics.Failures.ID.String(): {
									Value: 3,
								},
							},
						},
						{
							ClusterId: &pb.ClusterId{
								Algorithm: "reason-v3",
								Id:        "cccccc00000000000000000000000001",
							},
							Title: `Example failure reason 2.`,
							Metrics: map[string]*pb.ClusterSummary_MetricValue{
								metrics.HumanClsFailedPresubmit.ID.String(): {
									Value: 4,
								},
								metrics.CriticalFailuresExonerated.ID.String(): {
									Value: 5,
								},
								metrics.Failures.ID.String(): {
									Value: 6,
								},
							},
						},
						{
							ClusterId: &pb.ClusterId{
								Algorithm: "rules",
								Id:        "01234567890abcdef01234567890abcdef",
							},
							Title: `(rule archived)`,
							Metrics: map[string]*pb.ClusterSummary_MetricValue{
								metrics.HumanClsFailedPresubmit.ID.String(): {
									Value: 7,
								},
								metrics.CriticalFailuresExonerated.ID.String(): {
									Value: 8,
								},
								metrics.Failures.ID.String(): {
									Value: 9,
								},
							},
						},
					},
				}

				t.Run("With filters and order by", func(t *ftt.Test) {
					response, err := server.QueryClusterSummaries(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("Without filters or order", func(t *ftt.Test) {
					request.FailureFilter = ""
					request.OrderBy = ""

					response, err := server.QueryClusterSummaries(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("With full view", func(t *ftt.Test) {
					request.View = pb.ClusterSummaryView_FULL

					expectedFullResponse := &pb.QueryClusterSummariesResponse{
						ClusterSummaries: []*pb.ClusterSummary{
							{
								ClusterId: &pb.ClusterId{
									Algorithm: "rules",
									Id:        rs[0].RuleID,
								},
								Title: rs[0].RuleDefinition,
								Bug: &pb.AssociatedBug{
									System:   "monorail",
									Id:       "chromium/7654321",
									LinkText: "crbug.com/7654321",
									Url:      "https://bugs.chromium.org/p/chromium/issues/detail?id=7654321",
								},
								Metrics: map[string]*pb.ClusterSummary_MetricValue{
									metrics.HumanClsFailedPresubmit.ID.String(): {
										Value:          1,
										DailyBreakdown: []int64{1, 0, 0, 0, 0, 0, 0},
									},
									metrics.CriticalFailuresExonerated.ID.String(): {
										Value:          2,
										DailyBreakdown: []int64{1, 1, 0, 0, 0, 0, 0},
									},
									metrics.Failures.ID.String(): {
										Value:          3,
										DailyBreakdown: []int64{1, 1, 1, 0, 0, 0, 0},
									},
								},
							},
							{
								ClusterId: &pb.ClusterId{
									Algorithm: "reason-v3",
									Id:        "cccccc00000000000000000000000001",
								},
								Title: `Example failure reason 2.`,
								Metrics: map[string]*pb.ClusterSummary_MetricValue{
									metrics.HumanClsFailedPresubmit.ID.String(): {
										Value:          4,
										DailyBreakdown: []int64{1, 1, 1, 1, 0, 0, 0},
									},
									metrics.CriticalFailuresExonerated.ID.String(): {
										Value:          5,
										DailyBreakdown: []int64{1, 1, 1, 1, 1, 0, 0},
									},
									metrics.Failures.ID.String(): {
										Value:          6,
										DailyBreakdown: []int64{1, 1, 1, 1, 1, 1, 0},
									},
								},
							},
							{
								ClusterId: &pb.ClusterId{
									Algorithm: "rules",
									Id:        "01234567890abcdef01234567890abcdef",
								},
								Title: `(rule archived)`,
								Metrics: map[string]*pb.ClusterSummary_MetricValue{
									metrics.HumanClsFailedPresubmit.ID.String(): {
										Value:          7,
										DailyBreakdown: []int64{1, 1, 1, 1, 1, 1, 1},
									},
									metrics.CriticalFailuresExonerated.ID.String(): {
										Value:          8,
										DailyBreakdown: []int64{2, 1, 1, 1, 1, 1, 1},
									},
									metrics.Failures.ID.String(): {
										Value:          9,
										DailyBreakdown: []int64{2, 2, 1, 1, 1, 1, 1},
									},
								},
							},
						},
					}

					response, err := server.QueryClusterSummaries(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedFullResponse))
				})
				t.Run("Without rule definition get permission", func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

					// The RPC cannot return the rule definition as the
					// cluster title as the user is not authorised to see it.
					// Instead, it should generate a description of the
					// content of the cluster based on what the user can see.
					expectedResponse.ClusterSummaries[0].Title = "Selected failures in TestID 1"

					response, err := server.QueryClusterSummaries(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("Without metrics", func(t *ftt.Test) {
					request.Metrics = []string{}
					request.OrderBy = ""

					for _, item := range expectedResponse.ClusterSummaries {
						item.Metrics = make(map[string]*pb.ClusterSummary_MetricValue)
					}

					response, err := server.QueryClusterSummaries(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
			})
			t.Run("Invalid request", func(t *ftt.Test) {
				t.Run("Failure filter syntax is invalid", func(t *ftt.Test) {
					request.FailureFilter = "test_id::"

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("failure_filter: expected arg after :"))
				})
				t.Run("Failure filter references non-existant column", func(t *ftt.Test) {
					request.FailureFilter = `test:"pita.Boot"`

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`failure_filter: no filterable field "test"`))
				})
				t.Run("Failure filter references unimplemented feature", func(t *ftt.Test) {
					request.FailureFilter = "test_id<=\"blah\""

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("failure_filter: comparator operator not implemented yet"))
				})
				t.Run("Metrics references non-existent metric", func(t *ftt.Test) {
					request.Metrics = []string{"projects/testproject/metrics/not-exists"}
					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`metrics: no metric with ID "not-exists"`))
				})
				t.Run("Metrics references metric in another project", func(t *ftt.Test) {
					request.Metrics = []string{"projects/anotherproject/metrics/failures"}
					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`metrics: metric projects/anotherproject/metrics/failures cannot be used as it is from a different LUCI Project`))
				})
				t.Run("Order by references metric that is not selected", func(t *ftt.Test) {
					request.Metrics = []string{"projects/testproject/metrics/failures"}
					request.OrderBy = "metrics.`human-cls-failed-presubmit`.value desc"

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("order_by: no sortable field named \"metrics.`human-cls-failed-presubmit`.value\", valid fields are metrics.failures.value"))
				})
				t.Run("Order by syntax invalid", func(t *ftt.Test) {
					// To sort in ascending order, "desc" should be omittted. "asc" is not valid syntax.
					request.OrderBy = "metrics.`human-cls-failed-presubmit`.value asc"

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`order_by: syntax error: 1:44: unexpected token "asc"`))
				})
				t.Run("Order by syntax references invalid column", func(t *ftt.Test) {
					request.OrderBy = "not_exists desc"

					// Run
					response, err := server.QueryClusterSummaries(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`order_by: no sortable field named "not_exists"`))
				})
			})
		})
		t.Run("GetReclusteringProgress", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{{
				Realm:      "testproject:@project",
				Permission: perms.PermGetCluster,
			}}

			request := &pb.GetReclusteringProgressRequest{
				Name: "projects/testproject/reclusteringProgress",
			}
			t.Run("Not authorised to get cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetCluster)

				response, err := server.GetReclusteringProgress(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				rulesVersion := time.Date(2021, time.January, 1, 1, 0, 0, 0, time.UTC)
				reference := time.Date(2020, time.February, 1, 1, 0, 0, 0, time.UTC)
				configVersion := time.Date(2019, time.March, 1, 1, 0, 0, 0, time.UTC)
				rns := []*runs.ReclusteringRun{
					runs.NewRun(0).
						WithProject("testproject").
						WithAttemptTimestamp(reference.Add(-5 * time.Minute)).
						WithRulesVersion(rulesVersion).
						WithAlgorithmsVersion(2).
						WithConfigVersion(configVersion).
						WithNoReportedProgress().
						Build(),
					runs.NewRun(1).
						WithProject("testproject").
						WithAttemptTimestamp(reference.Add(-10 * time.Minute)).
						WithRulesVersion(rulesVersion).
						WithAlgorithmsVersion(2).
						WithConfigVersion(configVersion).
						WithReportedProgress(500).
						Build(),
					runs.NewRun(2).
						WithProject("testproject").
						WithAttemptTimestamp(reference.Add(-20 * time.Minute)).
						WithRulesVersion(rulesVersion.Add(-1 * time.Hour)).
						WithAlgorithmsVersion(1).
						WithConfigVersion(configVersion.Add(-1 * time.Hour)).
						WithCompletedProgress().
						Build(),
				}
				err := runs.SetRunsForTesting(ctx, t, rns)
				assert.Loosely(t, err, should.BeNil)

				// Run
				response, err := server.GetReclusteringProgress(ctx, request)

				// Verify.
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Resemble(&pb.ReclusteringProgress{
					Name:             "projects/testproject/reclusteringProgress",
					ProgressPerMille: 500,
					Last: &pb.ClusteringVersion{
						AlgorithmsVersion: 1,
						ConfigVersion:     timestamppb.New(configVersion.Add(-1 * time.Hour)),
						RulesVersion:      timestamppb.New(rulesVersion.Add(-1 * time.Hour)),
					},
					Next: &pb.ClusteringVersion{
						AlgorithmsVersion: 2,
						ConfigVersion:     timestamppb.New(configVersion),
						RulesVersion:      timestamppb.New(rulesVersion),
					},
				}))
			})
			t.Run("With an invalid request", func(t *ftt.Test) {
				t.Run("Invalid name", func(t *ftt.Test) {
					request.Name = "invalid"

					// Run
					response, err := server.GetReclusteringProgress(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: invalid reclustering progress name, expected format: projects/{project}/reclusteringProgress"))
				})
			})
		})
		t.Run("QueryClusterFailures", func(t *ftt.Test) {
			authState.IdentityPermissions = listTestResultsPermissions(
				"testproject:realm1",
				"testproject:realm2",
				"otherproject:realm3",
			)
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "testproject:@project",
				Permission: perms.PermGetCluster,
			})

			request := &pb.QueryClusterFailuresRequest{
				Parent: "projects/testproject/clusters/reason-v1/cccccc00000000000000000000000001/failures",
			}
			t.Run("Not authorised to get cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetCluster)

				response, err := server.QueryClusterFailures(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test results in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

				response, err := server.QueryClusterFailures(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test exonerations in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)

				response, err := server.QueryClusterFailures(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				analysisClient.expectedRealmsQueried = []string{"testproject:realm1", "testproject:realm2"}
				analysisClient.failuresByProjectAndCluster["testproject"] = map[clustering.ClusterID][]*analysis.ClusterFailure{
					{
						Algorithm: "reason-v1",
						ID:        "cccccc00000000000000000000000001",
					}: {
						{
							TestID: bqString("testID-1"),
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value1"),
								},
								{
									Key:   bqString("key2"),
									Value: bqString("value2"),
								},
							},
							PresubmitRunID: &analysis.PresubmitRunID{
								System: bqString("luci-cv"),
								ID:     bqString("123456789"),
							},
							PresubmitRunOwner: bqString("user"),
							PresubmitRunMode:  bqString(analysis.ToBQPresubmitRunMode(pb.PresubmitRunMode_QUICK_DRY_RUN)),
							Changelists: []*analysis.Changelist{
								{
									Host:     bqString("testproject.googlesource.com"),
									Change:   bigquery.NullInt64{Int64: 100006, Valid: true},
									Patchset: bigquery.NullInt64{Int64: 106, Valid: true},
								},
								{
									Host:     bqString("testproject-internal.googlesource.com"),
									Change:   bigquery.NullInt64{Int64: 100007, Valid: true},
									Patchset: bigquery.NullInt64{Int64: 107, Valid: true},
								},
							},
							PartitionTime: bigquery.NullTimestamp{Timestamp: time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC), Valid: true},
							Exonerations: []*analysis.Exoneration{
								{
									Reason: bqString(pb.ExonerationReason_OCCURS_ON_MAINLINE.String()),
								},
								{
									Reason: bqString(pb.ExonerationReason_NOT_CRITICAL.String()),
								},
							},
							BuildStatus:                 bqString(analysis.ToBQBuildStatus(pb.BuildStatus_BUILD_STATUS_FAILURE)),
							IsBuildCritical:             bigquery.NullBool{Bool: true, Valid: true},
							IngestedInvocationID:        bqString("build-1234567890"),
							IsIngestedInvocationBlocked: bigquery.NullBool{Bool: true, Valid: true},
							Count:                       15,
						},
						{
							TestID: bigquery.NullString{StringVal: "testID-2"},
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value2"),
								},
								{
									Key:   bqString("key3"),
									Value: bqString("value3"),
								},
							},
							PresubmitRunID:              nil,
							PresubmitRunOwner:           bigquery.NullString{},
							PresubmitRunMode:            bigquery.NullString{},
							Changelists:                 nil,
							PartitionTime:               bigquery.NullTimestamp{Timestamp: time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC), Valid: true},
							BuildStatus:                 bqString(analysis.ToBQBuildStatus(pb.BuildStatus_BUILD_STATUS_CANCELED)),
							IsBuildCritical:             bigquery.NullBool{},
							IngestedInvocationID:        bqString("build-9888887771"),
							IsIngestedInvocationBlocked: bigquery.NullBool{Bool: true, Valid: true},
							Count:                       1,
						},
					},
				}

				expectedResponse := &pb.QueryClusterFailuresResponse{
					Failures: []*pb.DistinctClusterFailure{
						{
							TestId:        "testID-1",
							Variant:       pbutil.Variant("key1", "value1", "key2", "value2"),
							PartitionTime: timestamppb.New(time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC)),
							PresubmitRun: &pb.DistinctClusterFailure_PresubmitRun{
								PresubmitRunId: &pb.PresubmitRunId{
									System: "luci-cv",
									Id:     "123456789",
								},
								Owner: "user",
								Mode:  pb.PresubmitRunMode_QUICK_DRY_RUN,
							},
							IsBuildCritical: true,
							Exonerations: []*pb.DistinctClusterFailure_Exoneration{{
								Reason: pb.ExonerationReason_OCCURS_ON_MAINLINE,
							}, {
								Reason: pb.ExonerationReason_NOT_CRITICAL,
							}},
							BuildStatus:                 pb.BuildStatus_BUILD_STATUS_FAILURE,
							IngestedInvocationId:        "build-1234567890",
							IsIngestedInvocationBlocked: true,
							Changelists: []*pb.Changelist{
								{
									Host:     "testproject.googlesource.com",
									Change:   100006,
									Patchset: 106,
								},
								{
									Host:     "testproject-internal.googlesource.com",
									Change:   100007,
									Patchset: 107,
								},
							},
							Count: 15,
						},
						{
							TestId:                      "testID-2",
							Variant:                     pbutil.Variant("key1", "value2", "key3", "value3"),
							PartitionTime:               timestamppb.New(time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC)),
							PresubmitRun:                nil,
							IsBuildCritical:             false,
							Exonerations:                nil,
							BuildStatus:                 pb.BuildStatus_BUILD_STATUS_CANCELED,
							IngestedInvocationId:        "build-9888887771",
							IsIngestedInvocationBlocked: true,
							Count:                       1,
						},
					},
				}

				t.Run("Without metric filter", func(t *ftt.Test) {
					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify.
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("With metric filter", func(t *ftt.Test) {
					request.MetricFilter = "projects/testproject/metrics/human-cls-failed-presubmit"
					metric, err := metrics.ByID(metrics.HumanClsFailedPresubmit.ID)
					assert.Loosely(t, err, should.BeNil)
					analysisClient.expectedMetricFilter = &metrics.Definition{}
					*analysisClient.expectedMetricFilter = metric.AdaptToProject("testproject", projectCfg.Metrics)

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify.
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))

				})
			})
			t.Run("With an invalid request", func(t *ftt.Test) {
				t.Run("Invalid parent", func(t *ftt.Test) {
					request.Parent = "blah"

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: invalid cluster failures name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/failures"))
				})
				t.Run("Invalid cluster algorithm in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason/cccccc00000000000000000000000001/failures"

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: algorithm not valid"))
				})
				t.Run("Invalid cluster ID in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason-v3/123/failures"

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: ID is not valid lowercase hexadecimal bytes"))
				})
				t.Run("Invalid metric ID format", func(t *ftt.Test) {
					request.MetricFilter = "metrics/human-cls-failed-presubmit"

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("filter_metric: invalid project metric name, expected format: projects/{project}/metrics/{metric_id}"))
				})
				t.Run("Filter metric references non-existant metric", func(t *ftt.Test) {
					request.MetricFilter = "projects/testproject/metrics/not-exists"

					// Run
					response, err := server.QueryClusterFailures(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`filter_metric: no metric with ID "not-exists"`))
				})
			})
		})

		t.Run("QueryExoneratedTestVariants", func(t *ftt.Test) {
			authState.IdentityPermissions = listTestResultsPermissions(
				"testproject:realm1",
				"testproject:realm2",
				"otherproject:realm3",
			)
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "testproject:@project",
				Permission: perms.PermGetCluster,
			})

			request := &pb.QueryClusterExoneratedTestVariantsRequest{
				Parent: "projects/testproject/clusters/reason-v1/cccccc00000000000000000000000001/exoneratedTestVariants",
			}
			t.Run("Not authorised to get cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetCluster)

				response, err := server.QueryExoneratedTestVariants(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test results in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

				response, err := server.QueryExoneratedTestVariants(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test exonerations in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)

				response, err := server.QueryExoneratedTestVariants(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				analysisClient.expectedRealmsQueried = []string{"testproject:realm1", "testproject:realm2"}
				analysisClient.exoneratedTVsByProjectAndCluster["testproject"] = map[clustering.ClusterID][]*analysis.ExoneratedTestVariant{
					{
						Algorithm: "reason-v1",
						ID:        "cccccc00000000000000000000000001",
					}: {
						{
							TestID: bqString("testID-1"),
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value1"),
								},
								{
									Key:   bqString("key2"),
									Value: bqString("value2"),
								},
							},
							CriticalFailuresExonerated: 51,
							LastExoneration:            bigquery.NullTimestamp{Timestamp: time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC), Valid: true},
						},
						{
							TestID: bigquery.NullString{StringVal: "testID-2"},
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value2"),
								},
								{
									Key:   bqString("key3"),
									Value: bqString("value3"),
								},
							},
							CriticalFailuresExonerated: 172,
							LastExoneration:            bigquery.NullTimestamp{Timestamp: time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC), Valid: true},
						},
					},
				}

				expectedResponse := &pb.QueryClusterExoneratedTestVariantsResponse{
					TestVariants: []*pb.ClusterExoneratedTestVariant{
						{
							TestId:                     "testID-1",
							Variant:                    pbutil.Variant("key1", "value1", "key2", "value2"),
							CriticalFailuresExonerated: 51,
							LastExoneration:            timestamppb.New(time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC)),
						},
						{
							TestId:                     "testID-2",
							Variant:                    pbutil.Variant("key1", "value2", "key3", "value3"),
							CriticalFailuresExonerated: 172,
							LastExoneration:            timestamppb.New(time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC)),
						},
					},
				}

				// Run
				response, err := server.QueryExoneratedTestVariants(ctx, request)

				// Verify.
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Resemble(expectedResponse))
			})
			t.Run("With an invalid request", func(t *ftt.Test) {
				t.Run("Invalid parent", func(t *ftt.Test) {
					request.Parent = "blah"

					// Run
					response, err := server.QueryExoneratedTestVariants(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: invalid resource name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/exoneratedTestVariants"))
				})
				t.Run("Invalid cluster algorithm in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason/cccccc00000000000000000000000001/exoneratedTestVariants"

					// Run
					response, err := server.QueryExoneratedTestVariants(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: algorithm not valid"))
				})
				t.Run("Invalid cluster ID in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason-v3/123/exoneratedTestVariants"

					// Run
					response, err := server.QueryExoneratedTestVariants(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: ID is not valid lowercase hexadecimal bytes"))
				})
			})
		})

		t.Run("QueryExoneratedTestVariantBranches", func(t *ftt.Test) {
			authState.IdentityPermissions = listTestResultsPermissions(
				"testproject:realm1",
				"testproject:realm2",
				"otherproject:realm3",
			)
			authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
				Realm:      "testproject:@project",
				Permission: perms.PermGetCluster,
			})

			request := &pb.QueryClusterExoneratedTestVariantBranchesRequest{
				Parent: "projects/testproject/clusters/reason-v1/cccccc00000000000000000000000001/exoneratedTestVariantBranches",
			}
			t.Run("Not authorised to get cluster", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetCluster)

				response, err := server.QueryExoneratedTestVariantBranches(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.clusters.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test results in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestResults)

				response, err := server.QueryExoneratedTestVariantBranches(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Not authorised to list test exonerations in any realm", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, rdbperms.PermListTestExonerations)

				response, err := server.QueryExoneratedTestVariantBranches(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permissions [resultdb.testResults.list resultdb.testExonerations.list] in any realm in project \"testproject\""))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("With a valid request", func(t *ftt.Test) {
				analysisClient.expectedRealmsQueried = []string{"testproject:realm1", "testproject:realm2"}
				analysisClient.exoneratedTVBsByProjectAndCluster["testproject"] = map[clustering.ClusterID][]*analysis.ExoneratedTestVariantBranch{
					{
						Algorithm: "reason-v1",
						ID:        "cccccc00000000000000000000000001",
					}: {
						{
							Project: bqString("testproject"),
							TestID:  bqString("testID-1"),
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value1"),
								},
								{
									Key:   bqString("key2"),
									Value: bqString("value2"),
								},
							},
							SourceRef: analysis.SourceRef{
								Gitiles: &analysis.GitilesRef{
									Host:    bqString("myproject.googlesource.com"),
									Project: bqString("myproject/src"),
									Ref:     bqString("refs/heads/main"),
								},
							},
							CriticalFailuresExonerated: 51,
							LastExoneration:            bigquery.NullTimestamp{Timestamp: time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC), Valid: true},
						},
						{
							Project: bqString("testproject"),
							TestID:  bigquery.NullString{StringVal: "testID-2"},
							Variant: []*analysis.Variant{
								{
									Key:   bqString("key1"),
									Value: bqString("value2"),
								},
								{
									Key:   bqString("key3"),
									Value: bqString("value3"),
								},
							},
							SourceRef: analysis.SourceRef{
								Gitiles: &analysis.GitilesRef{
									Host:    bqString("myproject2.googlesource.com"),
									Project: bqString("myproject2/src"),
									Ref:     bqString("refs/heads/main2"),
								},
							},
							CriticalFailuresExonerated: 172,
							LastExoneration:            bigquery.NullTimestamp{Timestamp: time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC), Valid: true},
						},
					},
				}

				expectedResponse := &pb.QueryClusterExoneratedTestVariantBranchesResponse{
					TestVariantBranches: []*pb.ClusterExoneratedTestVariantBranch{
						{
							Project: "testproject",
							TestId:  "testID-1",
							Variant: pbutil.Variant("key1", "value1", "key2", "value2"),
							SourceRef: &pb.SourceRef{
								System: &pb.SourceRef_Gitiles{
									Gitiles: &pb.GitilesRef{
										Host:    "myproject.googlesource.com",
										Project: "myproject/src",
										Ref:     "refs/heads/main",
									},
								},
							},
							CriticalFailuresExonerated: 51,
							LastExoneration:            timestamppb.New(time.Date(2123, time.April, 1, 2, 3, 4, 5, time.UTC)),
						},
						{
							Project: "testproject",
							TestId:  "testID-2",
							Variant: pbutil.Variant("key1", "value2", "key3", "value3"),
							SourceRef: &pb.SourceRef{
								System: &pb.SourceRef_Gitiles{
									Gitiles: &pb.GitilesRef{
										Host:    "myproject2.googlesource.com",
										Project: "myproject2/src",
										Ref:     "refs/heads/main2",
									},
								},
							},
							CriticalFailuresExonerated: 172,
							LastExoneration:            timestamppb.New(time.Date(2124, time.May, 2, 3, 4, 5, 6, time.UTC)),
						},
					},
				}

				// Run
				response, err := server.QueryExoneratedTestVariantBranches(ctx, request)

				// Verify.
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Resemble(expectedResponse))
			})
			t.Run("With an invalid request", func(t *ftt.Test) {
				t.Run("Invalid parent", func(t *ftt.Test) {
					request.Parent = "blah"

					// Run
					response, err := server.QueryExoneratedTestVariantBranches(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: invalid resource name, expected format: projects/{project}/clusters/{cluster_alg}/{cluster_id}/exoneratedTestVariantBranches"))
				})
				t.Run("Invalid cluster algorithm in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason/cccccc00000000000000000000000001/exoneratedTestVariantBranches"

					// Run
					response, err := server.QueryExoneratedTestVariantBranches(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: algorithm not valid"))
				})
				t.Run("Invalid cluster ID in parent", func(t *ftt.Test) {
					request.Parent = "projects/blah/clusters/reason-v3/123/exoneratedTestVariantBranches"

					// Run
					response, err := server.QueryExoneratedTestVariantBranches(ctx, request)

					// Verify
					assert.Loosely(t, response, should.BeNil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("parent: cluster id: ID is not valid lowercase hexadecimal bytes"))
				})
			})
		})
	})
}

func bqString(value string) bigquery.NullString {
	return bigquery.NullString{StringVal: value, Valid: true}
}

func listTestResultsPermissions(realms ...string) []authtest.RealmPermission {
	var result []authtest.RealmPermission
	for _, r := range realms {
		result = append(result, authtest.RealmPermission{
			Realm:      r,
			Permission: rdbperms.PermListTestResults,
		})
		result = append(result, authtest.RealmPermission{
			Realm:      r,
			Permission: rdbperms.PermListTestExonerations,
		})
	}
	return result
}

func removePermission(perms []authtest.RealmPermission, permission realms.Permission) []authtest.RealmPermission {
	var result []authtest.RealmPermission
	for _, p := range perms {
		if p.Permission != permission {
			result = append(result, p)
		}
	}
	return result
}

func emptyMetricValues() *pb.Cluster_TimewiseCounts {
	return &pb.Cluster_TimewiseCounts{
		OneDay:   &pb.Cluster_Counts{},
		ThreeDay: &pb.Cluster_Counts{},
		SevenDay: &pb.Cluster_Counts{},
	}
}

func failureReasonClusterEntry(projectcfg *compiledcfg.ProjectConfig, primaryErrorMessage string) *pb.ClusterResponse_ClusteredTestResult_ClusterEntry {
	alg := &failurereason.Algorithm{}
	clusterID := alg.Cluster(projectcfg, &clustering.Failure{
		Reason: &pb.FailureReason{
			PrimaryErrorMessage: primaryErrorMessage,
		},
	})
	return &pb.ClusterResponse_ClusteredTestResult_ClusterEntry{
		ClusterId: &pb.ClusterId{
			Algorithm: failurereason.AlgorithmName,
			Id:        hex.EncodeToString(clusterID),
		},
	}
}

func testNameClusterEntry(projectcfg *compiledcfg.ProjectConfig, testID string) *pb.ClusterResponse_ClusteredTestResult_ClusterEntry {
	alg := &testname.Algorithm{}
	clusterID := alg.Cluster(projectcfg, &clustering.Failure{
		TestID: testID,
	})
	return &pb.ClusterResponse_ClusteredTestResult_ClusterEntry{
		ClusterId: &pb.ClusterId{
			Algorithm: testname.AlgorithmName,
			Id:        hex.EncodeToString(clusterID),
		},
	}
}

// sortClusterEntries sorts clusters by ascending Cluster ID.
func sortClusterEntries(entries []*pb.ClusterResponse_ClusteredTestResult_ClusterEntry) []*pb.ClusterResponse_ClusteredTestResult_ClusterEntry {
	result := make([]*pb.ClusterResponse_ClusteredTestResult_ClusterEntry, len(entries))
	copy(result, entries)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ClusterId.Algorithm != result[j].ClusterId.Algorithm {
			return result[i].ClusterId.Algorithm < result[j].ClusterId.Algorithm
		}
		return result[i].ClusterId.Id < result[j].ClusterId.Id
	})
	return result
}

type fakeAnalysisClient struct {
	clustersByProject                 map[string][]*analysis.Cluster
	failuresByProjectAndCluster       map[string]map[clustering.ClusterID][]*analysis.ClusterFailure
	exoneratedTVsByProjectAndCluster  map[string]map[clustering.ClusterID][]*analysis.ExoneratedTestVariant
	exoneratedTVBsByProjectAndCluster map[string]map[clustering.ClusterID][]*analysis.ExoneratedTestVariantBranch
	clusterMetricsByProject           map[string][]*analysis.ClusterSummary
	clusterMetricBreakdownsByProject  map[string][]*analysis.ClusterMetricBreakdown
	expectedRealmsQueried             []string
	expectedMetricFilter              *metrics.Definition
}

func newFakeAnalysisClient() *fakeAnalysisClient {
	return &fakeAnalysisClient{
		clustersByProject:                 make(map[string][]*analysis.Cluster),
		failuresByProjectAndCluster:       make(map[string]map[clustering.ClusterID][]*analysis.ClusterFailure),
		exoneratedTVsByProjectAndCluster:  make(map[string]map[clustering.ClusterID][]*analysis.ExoneratedTestVariant),
		exoneratedTVBsByProjectAndCluster: make(map[string]map[clustering.ClusterID][]*analysis.ExoneratedTestVariantBranch),
		clusterMetricsByProject:           make(map[string][]*analysis.ClusterSummary),
		clusterMetricBreakdownsByProject:  make(map[string][]*analysis.ClusterMetricBreakdown),
	}
}

func (f *fakeAnalysisClient) ReadCluster(ctx context.Context, project string, clusterID clustering.ClusterID) (*analysis.Cluster, error) {
	clusters, ok := f.clustersByProject[project]
	if !ok {
		return nil, nil
	}

	var result *analysis.Cluster
	for _, c := range clusters {
		if c.ClusterID == clusterID {
			result = c
			break
		}
	}
	if result == nil {
		return analysis.EmptyCluster(clusterID), nil
	}
	return result, nil
}

func (f *fakeAnalysisClient) QueryClusterSummaries(ctx context.Context, project string, options *analysis.QueryClusterSummariesOptions) ([]*analysis.ClusterSummary, error) {
	clusters, ok := f.clusterMetricsByProject[project]
	if !ok {
		return nil, nil
	}

	set := stringset.NewFromSlice(options.Realms...)
	if set.Len() != len(f.expectedRealmsQueried) || !set.HasAll(f.expectedRealmsQueried...) {
		panic("realms passed to QueryClusterSummaries do not match expected")
	}

	_, _, err := analysis.ClusteredFailuresTable.WhereClause(options.FailureFilter, "w_")
	if err != nil {
		return nil, analysis.InvalidArgumentTag.Apply(errors.Annotate(err, "failure_filter").Err())
	}
	_, err = analysis.ClusterSummariesTable(options.Metrics).OrderByClause(options.OrderBy)
	if err != nil {
		return nil, analysis.InvalidArgumentTag.Apply(errors.Annotate(err, "order_by").Err())
	}

	var results []*analysis.ClusterSummary
	for _, c := range clusters {
		results = append(results, copyClusterSummary(c, options.Metrics, options.IncludeMetricBreakdown))
	}
	return results, nil
}

func copyClusterSummary(cs *analysis.ClusterSummary, queriedMetrics []metrics.Definition, includeMetricBreakdown bool) *analysis.ClusterSummary {
	result := &analysis.ClusterSummary{
		ClusterID:            cs.ClusterID,
		ExampleFailureReason: cs.ExampleFailureReason,
		ExampleTestID:        cs.ExampleTestID,
		UniqueTestIDs:        cs.UniqueTestIDs,
		MetricValues:         make(map[metrics.ID]*analysis.MetricValue),
	}
	for _, m := range queriedMetrics {
		metricValue := &analysis.MetricValue{
			Value: cs.MetricValues[m.ID].Value,
		}
		if includeMetricBreakdown {
			metricValue.DailyBreakdown = cs.MetricValues[m.ID].DailyBreakdown
		}
		result.MetricValues[m.ID] = metricValue
	}
	return result
}

func (f *fakeAnalysisClient) ReadClusterFailures(ctx context.Context, options analysis.ReadClusterFailuresOptions) ([]*analysis.ClusterFailure, error) {
	failuresByCluster, ok := f.failuresByProjectAndCluster[options.Project]
	if !ok {
		return nil, nil
	}
	if f.expectedMetricFilter != nil && (options.MetricFilter == nil || *options.MetricFilter != *f.expectedMetricFilter) ||
		f.expectedMetricFilter == nil && options.MetricFilter != nil {
		panic("filter metric passed to ReadClusterFailures does not match expected")
	}

	set := stringset.NewFromSlice(options.Realms...)
	if set.Len() != len(f.expectedRealmsQueried) || !set.HasAll(f.expectedRealmsQueried...) {
		panic("realms passed to ReadClusterFailures do not match expected")
	}

	return failuresByCluster[options.ClusterID], nil
}

func (f *fakeAnalysisClient) ReadClusterExoneratedTestVariants(ctx context.Context, options analysis.ReadClusterExoneratedTestVariantsOptions) ([]*analysis.ExoneratedTestVariant, error) {
	exoneratedTVsByCluster, ok := f.exoneratedTVsByProjectAndCluster[options.Project]
	if !ok {
		return nil, nil
	}

	set := stringset.NewFromSlice(options.Realms...)
	if set.Len() != len(f.expectedRealmsQueried) || !set.HasAll(f.expectedRealmsQueried...) {
		panic("realms passed to ReadClusterExoneratedTestVariants do not match expected")
	}

	return exoneratedTVsByCluster[options.ClusterID], nil
}

func (f *fakeAnalysisClient) ReadClusterExoneratedTestVariantBranches(ctx context.Context, options analysis.ReadClusterExoneratedTestVariantBranchesOptions) ([]*analysis.ExoneratedTestVariantBranch, error) {
	exoneratedTVBsByCluster, ok := f.exoneratedTVBsByProjectAndCluster[options.Project]
	if !ok {
		return nil, nil
	}

	set := stringset.NewFromSlice(options.Realms...)
	if set.Len() != len(f.expectedRealmsQueried) || !set.HasAll(f.expectedRealmsQueried...) {
		panic("realms passed to ReadClusterExoneratedTestVariantBranches do not match expected")
	}

	return exoneratedTVBsByCluster[options.ClusterID], nil
}

func (f *fakeAnalysisClient) ReadClusterHistory(ctx context.Context, options analysis.ReadClusterHistoryOptions) (ret []*analysis.ReadClusterHistoryDay, err error) {
	return nil, nil
}
