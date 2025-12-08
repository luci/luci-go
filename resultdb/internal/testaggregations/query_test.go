// Copyright 2025 The LUCI Authors.
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

package testaggregations

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestQuery(t *testing.T) {
	ftt.Run("Query", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")

		// Prepare root Invocation
		rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()

		moduleID := func(id string) *pb.ModuleIdentifier {
			return &pb.ModuleIdentifier{
				ModuleName:    id,
				ModuleVariant: pbutil.Variant("key", "value"),
				ModuleScheme:  "junit",
			}
		}

		// Prepare work units.
		workUnits := []*workunits.WorkUnitRow{
			// M1: Should be marked succeeded since at least one attempt in the only shard succeeded.
			// This is despite an earlier failure, skip, cancellation, and a pending and running attempt.
			workunits.NewBuilder(rootInvID, "wu-m1-a1").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_FAILED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m1-a2").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_SUCCEEDED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m1-a3").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_SKIPPED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m1-a4").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_CANCELLED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m1-a5").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_PENDING).Build(),
			workunits.NewBuilder(rootInvID, "wu-m1-a6").WithModuleID(moduleID("m1")).WithState(pb.WorkUnit_RUNNING).Build(),
			// M2: Should be marked running since a retry is in progress.
			workunits.NewBuilder(rootInvID, "wu-m2-a1").WithModuleID(moduleID("m2")).WithState(pb.WorkUnit_FAILED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m2-a2").WithModuleID(moduleID("m2")).WithState(pb.WorkUnit_RUNNING).Build(),
			// M3: Should be marked failed since one shard failed. This is despite one succeeding,
			// another still being in progress, one being cancelled, one being pending, and one
			// being skipped.
			workunits.NewBuilder(rootInvID, "wu-m3-s1").WithModuleID(moduleID("m3")).WithModuleShardKey("s1").WithState(pb.WorkUnit_FAILED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m3-s2").WithModuleID(moduleID("m3")).WithModuleShardKey("s2").WithState(pb.WorkUnit_RUNNING).Build(),
			workunits.NewBuilder(rootInvID, "wu-m3-s3").WithModuleID(moduleID("m3")).WithModuleShardKey("s3").WithState(pb.WorkUnit_SKIPPED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m3-s4").WithModuleID(moduleID("m3")).WithModuleShardKey("s4").WithState(pb.WorkUnit_SUCCEEDED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m3-s5").WithModuleID(moduleID("m3")).WithModuleShardKey("s5").WithState(pb.WorkUnit_CANCELLED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m3-s6").WithModuleID(moduleID("m3")).WithModuleShardKey("s6").WithState(pb.WorkUnit_PENDING).Build(),
			// M4: Should be marked pending, despite an earlier failure.
			workunits.NewBuilder(rootInvID, "wu-m4-a1").WithModuleID(moduleID("m4")).WithState(pb.WorkUnit_PENDING).Build(),
			workunits.NewBuilder(rootInvID, "wu-m4-a2").WithModuleID(moduleID("m4")).WithState(pb.WorkUnit_FAILED).Build(),
			// M5: Should be marked skipped.
			workunits.NewBuilder(rootInvID, "wu-m5").WithModuleID(moduleID("m5")).WithState(pb.WorkUnit_SKIPPED).Build(),
			// M6: Should be marked cancelled, despite another shard succeeding.
			workunits.NewBuilder(rootInvID, "wu-m6-s1").WithModuleID(moduleID("m6")).WithModuleShardKey("s1").WithState(pb.WorkUnit_CANCELLED).Build(),
			workunits.NewBuilder(rootInvID, "wu-m6-s2").WithModuleID(moduleID("m6")).WithModuleShardKey("s2").WithState(pb.WorkUnit_SUCCEEDED).Build(),
		}

		// Prepare test results.

		// Pick a few shards. In reality each test may be in a different shard.
		shard := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 5}
		shard2 := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 11}
		baseBuilder := func() *testresultsv2.Builder {
			return testresultsv2.NewBuilder().WithRootInvocationShardID(shard).
				WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
		}
		baseExonerationBuilder := func() *testexonerationsv2.Builder {
			return testexonerationsv2.NewBuilder().WithRootInvocationShardID(shard).
				WithModuleName("m1").WithModuleScheme("junit").WithModuleVariant(pbutil.Variant("key", "value")).WithCoarseName("c1").WithFineName("f1").WithCaseName("t1")
		}

		// Scenario 1: Passed, Failed, Exonerated, Flaky, Skipped, Error, Precluded
		results := []*testresultsv2.TestResultRow{
			// Passed
			baseBuilder().WithCaseName("passed_test").WithStatusV2(pb.TestResult_PASSED).Build(),
			// Failed
			baseBuilder().WithCaseName("failed_test").WithStatusV2(pb.TestResult_FAILED).Build(),
			// Exonerated
			baseBuilder().WithFineName("f2").WithCaseName("exonerated_test").WithStatusV2(pb.TestResult_FAILED).Build(),
			// Flaky (in another shard)
			baseBuilder().WithRootInvocationShardID(shard2).WithFineName("f3").WithCaseName("flaky_test").WithResultID("fr1").WithStatusV2(pb.TestResult_PASSED).Build(),
			baseBuilder().WithRootInvocationShardID(shard2).WithFineName("f3").WithCaseName("flaky_test").WithResultID("fr2").WithStatusV2(pb.TestResult_FAILED).Build(),
			// Skipped
			baseBuilder().WithCoarseName("c2").WithCaseName("skipped_test").WithStatusV2(pb.TestResult_SKIPPED).Build(),
			// Execution Errored
			baseBuilder().WithModuleName("m2").WithCaseName("execution_errored_test").WithStatusV2(pb.TestResult_EXECUTION_ERRORED).Build(),
			// Precluded
			baseBuilder().WithModuleName("m3").WithCaseName("precluded_test").WithStatusV2(pb.TestResult_PRECLUDED).Build(),
		}
		exonerations := []*testexonerationsv2.TestExonerationRow{
			// Exonerate one of the failed tests
			baseExonerationBuilder().WithFineName("f2").WithCaseName("exonerated_test").Build(),
		}

		// Insert data
		ms := insert.RootInvocationWithRootWorkUnit(rootInvRow)
		for _, r := range workUnits {
			ms = append(ms, workunits.InsertForTesting(r)...)
		}
		for _, r := range results {
			ms = append(ms, testresultsv2.InsertForTesting(r))
		}
		for _, e := range exonerations {
			ms = append(ms, testexonerationsv2.InsertForTesting(e))
		}
		testutil.MustApply(ctx, t, ms...)

		query := &SingleLevelQuery{
			rootInvocationID: rootInvID,
			level:            pb.AggregationLevel_FINE,
			pageSize:         100,
		}

		fetchAll := func(query *SingleLevelQuery) []*pb.TestAggregation {
			expectedPageSize := query.pageSize
			var results []*pb.TestAggregation
			var token string
			for {
				aggs, nextToken, err := query.Fetch(span.Single(ctx), token)
				assert.Loosely(t, err, should.BeNil, truth.LineContext(1))
				results = append(results, aggs...)
				if nextToken == "" {
					assert.Loosely(t, len(aggs), should.BeLessThanOrEqual(expectedPageSize))
					break
				} else {
					// While AIPs do not require it, the current implementation
					// should always return full pages unless the next page token
					// is empty.
					assert.Loosely(t, len(aggs), should.Equal(expectedPageSize))
				}
				token = nextToken
			}
			return results
		}

		t.Run("Root Invocation Level", func(t *ftt.Test) {
			expected := &pb.TestAggregation{
				Id: &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_INVOCATION,
					Id:    &pb.TestIdentifier{},
				},
				VerdictCounts: &pb.TestAggregation_VerdictCounts{
					Failed:               1,
					Flaky:                1,
					Passed:               1,
					Skipped:              1,
					ExecutionErrored:     1,
					Precluded:            1,
					Exonerated:           1,
					FailedBase:           2,
					FlakyBase:            1,
					PassedBase:           1,
					SkippedBase:          1,
					ExecutionErroredBase: 1,
					PrecludedBase:        1,
				},
				ModuleStatusCounts: &pb.TestAggregation_ModuleStatusCounts{
					Failed:    1,
					Running:   1,
					Pending:   1,
					Cancelled: 1,
					Succeeded: 1,
					Skipped:   1,
				},
			}

			query.level = pb.AggregationLevel_INVOCATION
			aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nextToken, should.Equal(""))
			assert.Loosely(t, aggs, should.HaveLength(1))
			assert.Loosely(t, aggs[0], should.Match(expected))
		})

		t.Run("Module Level", func(t *ftt.Test) {
			query.level = pb.AggregationLevel_MODULE
			expected := []*pb.TestAggregation{
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Failed:      1,
						Flaky:       1,
						Passed:      1,
						Skipped:     1,
						Exonerated:  1,
						FailedBase:  2,
						FlakyBase:   1,
						PassedBase:  1,
						SkippedBase: 1,
					},
					ModuleStatus: pb.TestAggregation_SUCCEEDED,
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m2",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						ExecutionErrored:     1,
						ExecutionErroredBase: 1,
					},
					ModuleStatus: pb.TestAggregation_RUNNING,
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m3",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Precluded:     1,
						PrecludedBase: 1,
					},
					ModuleStatus: pb.TestAggregation_ERRORED,
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m4",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{},
					ModuleStatus:  pb.TestAggregation_PENDING,
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m5",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{},
					ModuleStatus:  pb.TestAggregation_SKIPPED,
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_MODULE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m6",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{},
					ModuleStatus:  pb.TestAggregation_CANCELLED,
				},
			}

			t.Run("Default sorting", func(t *ftt.Test) {
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 7 // More than large enough for all modules.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.uiSortOrder = true
				// Update the expected order.
				// Cancelled sorts before pending and skipped.
				expected[3], expected[4], expected[5] = expected[5], expected[3], expected[4]
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 7 // Enough for all modules, plus one.
					query.level = pb.AggregationLevel_MODULE
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("With module-level filter", func(t *ftt.Test) {
				query.testPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:        "m2",
						ModuleScheme:      "junit",
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					},
				}
				t.Run("With module variant", func(t *ftt.Test) {
					query.testPrefixFilter.Id.ModuleVariant = pbutil.Variant("key", "value")
					query.testPrefixFilter.Id.ModuleVariantHash = ""
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("With module variant hash", func(t *ftt.Test) {
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})

		t.Run("Coarse Level", func(t *ftt.Test) {
			query.level = pb.AggregationLevel_COARSE
			expected := []*pb.TestAggregation{
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_COARSE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Failed:     1,
						Flaky:      1,
						Passed:     1,
						Exonerated: 1,
						FailedBase: 2,
						FlakyBase:  1,
						PassedBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_COARSE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c2",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Skipped:     1,
						SkippedBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_COARSE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m2",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						ExecutionErrored:     1,
						ExecutionErroredBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_COARSE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m3",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Precluded:     1,
						PrecludedBase: 1,
					},
				},
			}

			t.Run("Default sorting", func(t *ftt.Test) {
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 5 // Enough for all items, plus one.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.uiSortOrder = true
				// Update the expected order.
				// Modules with failures sort first, then execution errored and precluded.
				expectedUI := make([]*pb.TestAggregation, len(expected))
				copy(expectedUI, expected)
				expectedUI[1] = expected[2]
				expectedUI[2] = expected[3]
				expectedUI[3] = expected[1]
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 5 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expectedUI))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expectedUI))
				})
			})
			t.Run("With filters", func(t *ftt.Test) {
				query.testPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:    "m1",
						ModuleScheme:  "junit",
						ModuleVariant: pbutil.Variant("key", "value"),
					},
				}

				t.Run("module-level filter", func(t *ftt.Test) {
					expected = expected[0:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("coarse name-level filter", func(t *ftt.Test) {
					query.testPrefixFilter.Level = pb.AggregationLevel_COARSE
					query.testPrefixFilter.Id.CoarseName = "c2"
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})

		t.Run("Fine level", func(t *ftt.Test) {
			query.level = pb.AggregationLevel_FINE
			expected := []*pb.TestAggregation{
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Failed:     1,
						Passed:     1,
						FailedBase: 1,
						PassedBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f2",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Exonerated: 1,
						FailedBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f3",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Flaky:     1,
						FlakyBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m1",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c2",
							FineName:          "f1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Skipped:     1,
						SkippedBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m2",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						ExecutionErrored:     1,
						ExecutionErroredBase: 1,
					},
				},
				{
					Id: &pb.TestIdentifierPrefix{
						Level: pb.AggregationLevel_FINE,
						Id: &pb.TestIdentifier{
							ModuleName:        "m3",
							ModuleScheme:      "junit",
							ModuleVariant:     pbutil.Variant("key", "value"),
							ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
							CoarseName:        "c1",
							FineName:          "f1",
						},
					},
					VerdictCounts: &pb.TestAggregation_VerdictCounts{
						Precluded:     1,
						PrecludedBase: 1,
					},
				},
			}

			t.Run("Default sorting", func(t *ftt.Test) {
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 7 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expected))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expected))
				})
			})
			t.Run("UI sorting", func(t *ftt.Test) {
				query.uiSortOrder = true
				// Update the expected order.
				// Modules with failures sort first, then execution errored and precluded,
				// then flaky, then exonerations.
				expectedUI := make([]*pb.TestAggregation, len(expected))
				copy(expectedUI, expected)
				expectedUI[1], expectedUI[2], expectedUI[3], expectedUI[4], expectedUI[5] = expectedUI[4], expectedUI[5], expectedUI[2], expectedUI[1], expectedUI[3]
				t.Run("Without pagination", func(t *ftt.Test) {
					query.pageSize = 7 // More than large enough for all items.
					aggs, nextToken, err := query.Fetch(span.Single(ctx), "")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nextToken, should.Equal(""))
					assert.Loosely(t, aggs, should.Match(expectedUI))
				})
				t.Run("With pagination", func(t *ftt.Test) {
					query.pageSize = 1
					all := fetchAll(query)
					assert.Loosely(t, all, should.Match(expectedUI))
				})
			})
			t.Run("With filters", func(t *ftt.Test) {
				query.testPrefixFilter = &pb.TestIdentifierPrefix{
					Level: pb.AggregationLevel_MODULE,
					Id: &pb.TestIdentifier{
						ModuleName:        "m1",
						ModuleScheme:      "junit",
						ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("key", "value")),
					},
				}

				t.Run("module-level filter", func(t *ftt.Test) {
					expected = expected[0:4]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("coarse name-level filter", func(t *ftt.Test) {
					query.testPrefixFilter.Level = pb.AggregationLevel_COARSE
					query.testPrefixFilter.Id.CoarseName = "c1"
					expected = expected[0:3]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
				t.Run("fine name-level filter", func(t *ftt.Test) {
					query.testPrefixFilter.Level = pb.AggregationLevel_FINE
					query.testPrefixFilter.Id.CoarseName = "c1"
					query.testPrefixFilter.Id.FineName = "f2"
					expected = expected[1:2]
					assert.Loosely(t, fetchAll(query), should.Match(expected))
				})
			})
		})
	})
}
