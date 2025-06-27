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

package ingestion

import (
	"encoding/hex"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestIngest(t *testing.T) {
	ftt.Run(`With Ingestor`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For rules cache.
		ctx = memory.Use(ctx)                    // For project config in datastore.

		chunkStore := chunkstore.NewFakeClient()
		clusteredFailures := clusteredfailures.NewFakeClient()
		analysis := analysis.NewClusteringHandler(clusteredFailures)
		ingestor := New(chunkStore, analysis)

		opts := Options{
			TaskIndex:     1,
			Project:       "chromium",
			PartitionTime: time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			Realm:         "chromium:ci",
			InvocationID:  "build-123456790123456",
			PresubmitRun: &PresubmitRun{
				ID:     &pb.PresubmitRunId{System: "luci-cv", Id: "cq-run-123"},
				Owner:  "automation",
				Mode:   pb.PresubmitRunMode_FULL_RUN,
				Status: pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
			},
			BuildStatus:            pb.BuildStatus_BUILD_STATUS_FAILURE,
			BuildCritical:          true,
			BuildGardenerRotations: []string{"gardener-rotation1", "gardener-rotation2"},
		}
		testIngestion := func(input []TestVerdict, expectedCFs []*bqpb.ClusteredFailureRow) {
			err := ingestor.Ingest(ctx, opts, input)
			assert.Loosely(t, err, should.BeNil)

			insertions := clusteredFailures.Insertions
			assert.Loosely(t, len(insertions), should.Equal(len(expectedCFs)))

			// Sort both actuals and expectations by key so that we compare corresponding rows.
			sortClusteredFailures(insertions)
			sortClusteredFailures(expectedCFs)
			for i, exp := range expectedCFs {
				actual := insertions[i]
				assert.Loosely(t, actual, should.NotBeNil)

				// Chunk ID and index is assigned by ingestion.
				copyExp := proto.Clone(exp).(*bqpb.ClusteredFailureRow)
				assert.Loosely(t, actual.ChunkId, should.NotBeEmpty)
				assert.Loosely(t, actual.ChunkIndex, should.BeGreaterThanOrEqual(1))
				copyExp.ChunkId = actual.ChunkId
				copyExp.ChunkIndex = actual.ChunkIndex

				// LastUpdated time is assigned by Spanner.
				assert.Loosely(t, actual.LastUpdated, should.NotBeZero)
				copyExp.LastUpdated = actual.LastUpdated

				assert.Loosely(t, actual, should.Match(copyExp))
			}
		}

		// These rules are used to match failures in this test.
		ruleOnReason := rules.NewRule(100).WithProject(opts.Project).WithRuleDefinition(`reason LIKE "Failure reason%"`).Build()
		ruleOnTestID := rules.NewRule(101).WithProject(opts.Project).WithRuleDefinition(`test LIKE "%NeedleTest%"`).Build()
		err := rules.SetForTesting(ctx, t, []*rules.Entry{
			ruleOnReason,
			ruleOnTestID,
		})
		assert.Loosely(t, err, should.BeNil)

		// Setup clustering configuration
		projectCfg := &configpb.ProjectConfig{
			Clustering:  algorithms.TestClusteringConfig(),
			LastUpdated: timestamppb.New(time.Date(2020, time.January, 5, 0, 0, 0, 1, time.UTC)),
		}
		projectCfgs := map[string]*configpb.ProjectConfig{
			"chromium": projectCfg,
		}
		assert.Loosely(t, config.SetTestProjectConfig(ctx, projectCfgs), should.BeNil)

		cfg, err := compiledcfg.NewConfig(projectCfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`Ingest one failure`, func(t *ftt.Test) {
			const uniqifier = 1
			const testRunCount = 1
			const resultsPerTestRun = 1
			tv := newTestVerdict(uniqifier, testRunCount, resultsPerTestRun, nil)
			tvs := []TestVerdict{tv}

			// Expect the test result to be clustered by both reason and test name.
			const testRunNum = 0
			const resultNum = 0
			regexpCF := expectedClusteredFailure(uniqifier, testRunCount, testRunNum, resultsPerTestRun, resultNum)
			setRegexpClustered(cfg, regexpCF)
			testnameCF := expectedClusteredFailure(uniqifier, testRunCount, testRunNum, resultsPerTestRun, resultNum)
			setTestNameClustered(cfg, testnameCF)
			ruleCF := expectedClusteredFailure(uniqifier, testRunCount, testRunNum, resultsPerTestRun, resultNum)
			setRuleClustered(ruleCF, ruleOnReason)
			expectedCFs := []*bqpb.ClusteredFailureRow{regexpCF, testnameCF, ruleCF}

			t.Run(`Unexpected failure`, func(t *ftt.Test) {
				tv.Verdict.Results[0].Result.Status = rdbpb.TestStatus_FAIL
				tv.Verdict.Results[0].Result.Expected = false

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Expected failure`, func(t *ftt.Test) {
				tv.Verdict.Results[0].Result.Status = rdbpb.TestStatus_FAIL
				tv.Verdict.Results[0].Result.Expected = true

				// Expect no test results ingested for an expected
				// failure.
				expectedCFs = nil

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.BeZero)
			})
			t.Run(`Unexpected pass`, func(t *ftt.Test) {
				tv.Verdict.Results[0].Result.Status = rdbpb.TestStatus_PASS
				tv.Verdict.Results[0].Result.Expected = false

				// Expect no test results ingested for a passed test
				// (even if unexpected).
				expectedCFs = nil
				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.BeZero)
			})
			t.Run(`Unexpected skip`, func(t *ftt.Test) {
				tv.Verdict.Results[0].Result.Status = rdbpb.TestStatus_SKIP
				tv.Verdict.Results[0].Result.Expected = false

				// Expect no test results ingested for a skipped test
				// (even if unexpected).
				expectedCFs = nil

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.BeZero)
			})
			t.Run(`Failure with no tags`, func(t *ftt.Test) {
				// Tests are allowed to have no tags.
				tv.Verdict.Results[0].Result.Tags = nil

				for _, cf := range expectedCFs {
					cf.Tags = nil
					cf.BugTrackingComponent = nil
				}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure without variant`, func(t *ftt.Test) {
				// Tests are allowed to have no variant.
				tv.Verdict.Variant = nil
				tv.Verdict.Results[0].Result.Variant = nil

				for _, cf := range expectedCFs {
					cf.Variant = nil
					cf.TestIdStructured.ModuleVariant = `{}`
					cf.TestIdStructured.ModuleVariantHash = pbutil.VariantHash(pbutil.Variant()) // hash of the empty variant.
				}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure without failure reason`, func(t *ftt.Test) {
				// Failures may not have a failure reason.
				tv.Verdict.Results[0].Result.FailureReason = nil
				testnameCF.FailureReason = nil

				// As the test result does not match any rules, the
				// test result is included in the suggested cluster
				// with high priority.
				testnameCF.IsIncludedWithHighPriority = true
				expectedCFs = []*bqpb.ClusteredFailureRow{testnameCF}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure without presubmit run`, func(t *ftt.Test) {
				opts.PresubmitRun = nil
				for _, cf := range expectedCFs {
					cf.PresubmitRunId = nil
					cf.PresubmitRunMode = ""
					cf.PresubmitRunOwner = ""
					cf.PresubmitRunStatus = ""
					cf.BuildCritical = false
				}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure with multiple exoneration`, func(t *ftt.Test) {
				tv.Verdict.Exonerations = []*rdbpb.TestExoneration{
					{
						Name:            fmt.Sprintf("invocations/testrun-mytestrun/tests/test-name-%v/exonerations/exon-1", uniqifier),
						TestId:          tv.Verdict.TestId,
						Variant:         proto.Clone(tv.Verdict.Variant).(*rdbpb.Variant),
						VariantHash:     "hash",
						ExonerationId:   "exon-1",
						ExplanationHtml: "<p>Some description</p>",
						Reason:          rdbpb.ExonerationReason_OCCURS_ON_MAINLINE,
					},
					{
						Name:            fmt.Sprintf("invocations/testrun-mytestrun/tests/test-name-%v/exonerations/exon-1", uniqifier),
						TestId:          tv.Verdict.TestId,
						Variant:         proto.Clone(tv.Verdict.Variant).(*rdbpb.Variant),
						VariantHash:     "hash",
						ExonerationId:   "exon-1",
						ExplanationHtml: "<p>Some description</p>",
						Reason:          rdbpb.ExonerationReason_OCCURS_ON_OTHER_CLS,
					},
				}

				for _, cf := range expectedCFs {
					cf.Exonerations = []*bqpb.ClusteredFailureRow_TestExoneration{
						{
							Reason: pb.ExonerationReason_OCCURS_ON_MAINLINE,
						}, {
							Reason: pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
						},
					}
				}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure with only suggested clusters`, func(t *ftt.Test) {
				reason := &pb.FailureReason{
					PrimaryErrorMessage: "Should not match rule",
				}
				tv.Verdict.Results[0].Result.FailureReason = &rdbpb.FailureReason{
					PrimaryErrorMessage: "Should not match rule",
				}
				testnameCF.FailureReason = reason
				regexpCF.FailureReason = reason

				// Recompute the cluster ID to reflect the different
				// failure reason.
				setRegexpClustered(cfg, regexpCF)

				// As the test result does not match any rules, the
				// test result should be included in the suggested clusters
				// with high priority.
				testnameCF.IsIncludedWithHighPriority = true
				regexpCF.IsIncludedWithHighPriority = true
				expectedCFs = []*bqpb.ClusteredFailureRow{testnameCF, regexpCF}

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure without test metadata`, func(t *ftt.Test) {
				// Confirm nothing blows up due to assuming this is always set.
				tv.Verdict.TestMetadata = nil

				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure with prior test ID`, func(t *ftt.Test) {
				// Test setting the previous test ID triggers matching an additional rule.
				tv.Verdict.TestMetadata.PreviousTestId = "MyNeedleTest"

				additionalRuleCF := expectedClusteredFailure(uniqifier, testRunCount, testRunNum, resultsPerTestRun, resultNum)
				setRuleClustered(additionalRuleCF, ruleOnTestID)
				expectedCFs = append(expectedCFs, additionalRuleCF)

				for _, cf := range expectedCFs {
					cf.PreviousTestId = "MyNeedleTest"
				}
				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
			t.Run(`Failure with bug component metadata`, func(t *ftt.Test) {
				t.Run(`With monorail bug system`, func(t *ftt.Test) {
					tv.Verdict.TestMetadata.BugComponent = &rdbpb.BugComponent{
						System: &rdbpb.BugComponent_Monorail{
							Monorail: &rdbpb.MonorailComponent{
								Project: "chromium",
								Value:   "Blink>Component",
							},
						},
					}
					for _, cf := range expectedCFs {
						cf.BugTrackingComponent.System = "monorail"
						cf.BugTrackingComponent.Component = "Blink>Component"
					}
					testIngestion(tvs, expectedCFs)
					assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
				})
				t.Run(`With Buganizer bug system`, func(t *ftt.Test) {
					tv.Verdict.TestMetadata.BugComponent = &rdbpb.BugComponent{
						System: &rdbpb.BugComponent_IssueTracker{
							IssueTracker: &rdbpb.IssueTrackerComponent{
								ComponentId: 12345,
							},
						},
					}
					for _, cf := range expectedCFs {
						cf.BugTrackingComponent.System = "buganizer"
						cf.BugTrackingComponent.Component = "12345"
					}
					testIngestion(tvs, expectedCFs)
					assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
				})
				t.Run(`No BugComponent metadata, but public_buganizer_component tag present`, func(t *ftt.Test) {
					tv.Verdict.TestMetadata.BugComponent = nil

					for _, result := range tv.Verdict.Results {
						result.Result.Tags = []*rdbpb.StringPair{
							{
								Key:   "public_buganizer_component",
								Value: "654321",
							},
						}
					}
					for _, cf := range expectedCFs {
						cf.BugTrackingComponent.System = "buganizer"
						cf.BugTrackingComponent.Component = "654321"
						cf.Tags = []*pb.StringPair{
							{
								Key:   "public_buganizer_component",
								Value: "654321",
							},
						}
					}
					testIngestion(tvs, expectedCFs)
					assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
				})
				t.Run(`No BugComponent metadata, both public_buganizer_component and monorail_component present`, func(t *ftt.Test) {
					tv.Verdict.TestMetadata.BugComponent = nil

					for _, result := range tv.Verdict.Results {
						result.Result.Tags = []*rdbpb.StringPair{
							{
								Key:   "monorail_component",
								Value: "Component>MyComponent",
							},
							{
								Key:   "public_buganizer_component",
								Value: "654321",
							},
						}
					}
					for _, cf := range expectedCFs {
						cf.Tags = []*pb.StringPair{
							{
								Key:   "monorail_component",
								Value: "Component>MyComponent",
							},
							{
								Key:   "public_buganizer_component",
								Value: "654321",
							},
						}
					}

					t.Run("With monorail as preferred system", func(t *ftt.Test) {
						opts.PreferBuganizerComponents = false

						for _, cf := range expectedCFs {
							cf.BugTrackingComponent.System = "monorail"
							cf.BugTrackingComponent.Component = "Component>MyComponent"
						}
						testIngestion(tvs, expectedCFs)
						assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
					})
					t.Run("With buganizer as preferred system", func(t *ftt.Test) {
						opts.PreferBuganizerComponents = true

						for _, cf := range expectedCFs {
							cf.BugTrackingComponent.System = "buganizer"
							cf.BugTrackingComponent.Component = "654321"
						}
						testIngestion(tvs, expectedCFs)
						assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
					})
				})
			})
			t.Run(`Failure with no sources`, func(t *ftt.Test) {
				tv.Sources = nil
				for _, cf := range expectedCFs {
					cf.Sources = nil
					cf.SourceRef = nil
					cf.SourceRefHash = "<fix me>"
				}
				// No sources also means no test variant branch analysis.
				tv.TestVariantBranch = nil
			})
		})
		t.Run(`Ingest multiple failures`, func(t *ftt.Test) {
			const uniqifier = 1
			const testRunsPerVariant = 2
			const resultsPerTestRun = 2
			tv := newTestVerdict(uniqifier, testRunsPerVariant, resultsPerTestRun, nil)
			tvs := []TestVerdict{tv}

			// Setup a scenario as follows:
			// - A test was run four times in total, consisting of two test
			//   runs with two tries each.
			// - The test failed on all tries.
			var expectedCFs []*bqpb.ClusteredFailureRow
			var expectedCFsByTestRun [][]*bqpb.ClusteredFailureRow
			for t := range testRunsPerVariant {
				var testRunExp []*bqpb.ClusteredFailureRow
				for j := range resultsPerTestRun {
					regexpCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
					setRegexpClustered(cfg, regexpCF)
					testnameCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
					setTestNameClustered(cfg, testnameCF)
					ruleCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
					setRuleClustered(ruleCF, ruleOnReason)
					testRunExp = append(testRunExp, regexpCF, testnameCF, ruleCF)
				}
				expectedCFsByTestRun = append(expectedCFsByTestRun, testRunExp)
				expectedCFs = append(expectedCFs, testRunExp...)
			}

			// Expectation: all test results show both the test run and
			// invocation blocked by failures.
			for _, exp := range expectedCFs {
				exp.IsIngestedInvocationBlocked = true
				exp.IsTestRunBlocked = true
			}

			t.Run(`Some test runs blocked and presubmit run not blocked`, func(t *ftt.Test) {
				// Let the last retry of the last test run pass.
				tv.Verdict.Results[testRunsPerVariant*resultsPerTestRun-1].Result.Status = rdbpb.TestStatus_PASS
				// Drop the expected clustered failures for the last test result.
				expectedCFs = expectedCFs[0 : (testRunsPerVariant*resultsPerTestRun-1)*3]

				// First test run should be blocked.
				for _, exp := range expectedCFsByTestRun[0] {
					exp.IsIngestedInvocationBlocked = false
					exp.IsTestRunBlocked = true
				}
				// Last test run should not be blocked.
				for _, exp := range expectedCFsByTestRun[testRunsPerVariant-1] {
					exp.IsIngestedInvocationBlocked = false
					exp.IsTestRunBlocked = false
				}
				testIngestion(tvs, expectedCFs)
				assert.Loosely(t, len(chunkStore.Contents), should.Equal(1))
			})
		})
		t.Run(`Ingest many failures`, func(t *ftt.Test) {
			var tvs []TestVerdict
			var expectedCFs []*bqpb.ClusteredFailureRow

			const variantCount = 20
			const testRunsPerVariant = 10
			const resultsPerTestRun = 10
			for uniqifier := range variantCount {
				tv := newTestVerdict(uniqifier, testRunsPerVariant, resultsPerTestRun, nil)
				tvs = append(tvs, tv)
				for t := range testRunsPerVariant {
					for j := range resultsPerTestRun {
						regexpCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
						setRegexpClustered(cfg, regexpCF)
						testnameCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
						setTestNameClustered(cfg, testnameCF)
						ruleCF := expectedClusteredFailure(uniqifier, testRunsPerVariant, t, resultsPerTestRun, j)
						setRuleClustered(ruleCF, ruleOnReason)
						expectedCFs = append(expectedCFs, regexpCF, testnameCF, ruleCF)
					}
				}
			}
			// Verify more than one chunk is ingested.
			testIngestion(tvs, expectedCFs)
			assert.Loosely(t, len(chunkStore.Contents), should.BeGreaterThan(1))
		})
	})
}

func setTestNameClustered(cfg *compiledcfg.ProjectConfig, e *bqpb.ClusteredFailureRow) {
	e.ClusterAlgorithm = testname.AlgorithmName
	e.ClusterId = hex.EncodeToString((&testname.Algorithm{}).Cluster(cfg, &clustering.Failure{
		TestID: e.TestId,
	}))
}

func setRegexpClustered(cfg *compiledcfg.ProjectConfig, e *bqpb.ClusteredFailureRow) {
	e.ClusterAlgorithm = failurereason.AlgorithmName
	e.ClusterId = hex.EncodeToString((&failurereason.Algorithm{}).Cluster(cfg, &clustering.Failure{
		Reason: &pb.FailureReason{PrimaryErrorMessage: e.FailureReason.PrimaryErrorMessage},
	}))
}

func setRuleClustered(e *bqpb.ClusteredFailureRow, rule *rules.Entry) {
	e.ClusterAlgorithm = rulesalgorithm.AlgorithmName
	e.ClusterId = rule.RuleID
	e.IsIncludedWithHighPriority = true
}

func sortClusteredFailures(cfs []*bqpb.ClusteredFailureRow) {
	sort.Slice(cfs, func(i, j int) bool {
		return clusteredFailureKey(cfs[i]) < clusteredFailureKey(cfs[j])
	})
}

func clusteredFailureKey(cf *bqpb.ClusteredFailureRow) string {
	return fmt.Sprintf("%q/%q/%q/%q", cf.ClusterAlgorithm, cf.ClusterId, cf.TestResultSystem, cf.TestResultId)
}

func newTestVerdict(uniqifier, testRunCount, resultsPerTestRun int, bugComponent *rdbpb.BugComponent) TestVerdict {
	testID := fmt.Sprintf(":module!scheme:coarse:fine#case%v", uniqifier)
	variant := &rdbpb.Variant{
		Def: map[string]string{
			"k1": "v1",
		},
	}
	rdbVerdict := &rdbpb.TestVariant{
		TestId:       testID,
		Variant:      variant,
		VariantHash:  "hash",
		Status:       rdbpb.TestVariantStatus_UNEXPECTED,
		Exonerations: nil,
		TestMetadata: &rdbpb.TestMetadata{
			BugComponent: bugComponent,
		},
	}
	for i := range testRunCount {
		for j := range resultsPerTestRun {
			tr := newTestResult(uniqifier, i, j)
			// Test ID, Variant, VariantHash are not populated on the test
			// results of a Test Variant as it is present on the parent record.
			tr.TestId = ""
			tr.Variant = nil
			tr.VariantHash = ""
			rdbVerdict.Results = append(rdbVerdict.Results, &rdbpb.TestResultBundle{Result: tr})
		}
	}
	return TestVerdict{
		Verdict: rdbVerdict,
		Sources: testSources(),
		TestVariantBranch: &clusteringpb.TestVariantBranch{
			FlakyVerdicts_24H:      111,
			UnexpectedVerdicts_24H: 222,
			TotalVerdicts_24H:      555,
		},
	}
}

func newTestResult(uniqifier, testRunNum, resultNum int) *rdbpb.TestResult {
	return newFakeTestResult(uniqifier, testRunNum, resultNum)
}

func newFakeTestResult(uniqifier, testRunNum, resultNum int) *rdbpb.TestResult {
	resultID := fmt.Sprintf("result-%v-%v", testRunNum, resultNum)
	return &rdbpb.TestResult{
		Name:        fmt.Sprintf("invocations/testrun-%v/tests/test-name-%v/results/%s", testRunNum, uniqifier, resultID),
		ResultId:    resultID,
		Expected:    false,
		Status:      rdbpb.TestStatus_CRASH,
		SummaryHtml: "<p>Some SummaryHTML</p>",
		StartTime:   timestamppb.New(time.Date(2022, time.February, 12, 0, 0, 0, 0, time.UTC)),
		Duration:    durationpb.New(time.Second * 10),
		Tags: []*rdbpb.StringPair{
			{
				Key:   "monorail_component",
				Value: "Component>MyComponent",
			},
		},
		FailureReason: &rdbpb.FailureReason{
			PrimaryErrorMessage: "Failure reason.",
		},
	}
}

func testSources() *pb.Sources {
	result := &pb.Sources{
		GitilesCommit: &pb.GitilesCommit{
			Host:       "chromium.googlesource.com",
			Project:    "infra/infra",
			Ref:        "refs/heads/main",
			CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
			Position:   12345,
		},
		IsDirty: true,
		Changelists: []*pb.GerritChange{
			{
				Host:     "chromium-review.googlesource.com",
				Project:  "myproject",
				Change:   87654,
				Patchset: 321,
			},
		},
	}
	return result
}

func expectedClusteredFailure(uniqifier, testRunCount, testRunNum, resultsPerTestRun, resultNum int) *bqpb.ClusteredFailureRow {
	resultID := fmt.Sprintf("result-%v-%v", testRunNum, resultNum)
	return &bqpb.ClusteredFailureRow{
		ClusterAlgorithm:           "", // Determined by clustering algorithm.
		ClusterId:                  "", // Determined by clustering algorithm.
		TestResultSystem:           "resultdb",
		TestResultId:               fmt.Sprintf("invocations/testrun-%v/tests/test-name-%v/results/%s", testRunNum, uniqifier, resultID),
		LastUpdated:                nil, // Only known at runtime, Spanner commit timestamp.
		Project:                    "chromium",
		PartitionTime:              timestamppb.New(time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)),
		IsIncluded:                 true,
		IsIncludedWithHighPriority: false,

		ChunkId:    "",
		ChunkIndex: 0, // To be set by caller as needed.

		Realm: "chromium:ci",
		TestIdStructured: &bqpb.TestIdentifier{
			ModuleName:        "module",
			ModuleScheme:      "scheme",
			ModuleVariant:     `{"k1":"v1"}`,
			ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k1", "v1")),
			CoarseName:        "coarse",
			FineName:          "fine",
			CaseName:          fmt.Sprintf("case%v", uniqifier),
		},
		TestId: fmt.Sprintf(":module!scheme:coarse:fine#case%v", uniqifier),
		Tags: []*pb.StringPair{
			{
				Key:   "monorail_component",
				Value: "Component>MyComponent",
			},
		},
		Variant: []*pb.StringPair{
			{
				Key:   "k1",
				Value: "v1",
			},
		},
		VariantHash:   "hash",
		FailureReason: &pb.FailureReason{PrimaryErrorMessage: "Failure reason."},
		BugTrackingComponent: &pb.BugTrackingComponent{
			System:    "monorail",
			Component: "Component>MyComponent",
		},
		StartTime:    timestamppb.New(time.Date(2022, time.February, 12, 0, 0, 0, 0, time.UTC)),
		Duration:     10.0,
		Exonerations: nil,

		PresubmitRunId:                &pb.PresubmitRunId{System: "luci-cv", Id: "cq-run-123"},
		PresubmitRunOwner:             "automation",
		PresubmitRunMode:              "FULL_RUN", // pb.PresubmitRunMode_FULL_RUN
		PresubmitRunStatus:            "FAILED",   // pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_FAILED,
		BuildStatus:                   "FAILURE",  // pb.BuildStatus_BUILD_STATUS_FAILURE
		BuildCritical:                 true,
		IngestedInvocationId:          "build-123456790123456",
		IngestedInvocationResultIndex: int64(testRunNum*resultsPerTestRun + resultNum),
		IngestedInvocationResultCount: int64(testRunCount * resultsPerTestRun),
		IsIngestedInvocationBlocked:   true,
		TestRunId:                     fmt.Sprintf("testrun-%v", testRunNum),
		TestRunResultIndex:            int64(resultNum),
		TestRunResultCount:            int64(resultsPerTestRun),
		IsTestRunBlocked:              true,

		SourceRefHash: `923c0d1af67f8ef8`,
		SourceRef: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "chromium.googlesource.com",
					Project: "infra/infra",
					Ref:     "refs/heads/main",
				},
			},
		},
		Sources:                testSources(),
		BuildGardenerRotations: []string{"gardener-rotation1", "gardener-rotation2"},
		TestVariantBranch: &bqpb.ClusteredFailureRow_TestVariantBranch{
			FlakyVerdicts_24H:      111,
			UnexpectedVerdicts_24H: 222,
			TotalVerdicts_24H:      555,
		},
	}
}
