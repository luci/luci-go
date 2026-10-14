// Copyright 2026 The LUCI Authors.
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

package analysis

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type mockRulesClient struct {
	analysispb.RulesClient
	lookupBug func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error)
	get       func(ctx context.Context, in *analysispb.GetRuleRequest) (*analysispb.Rule, error)
}

func (m *mockRulesClient) LookupBug(ctx context.Context, in *analysispb.LookupBugRequest, opts ...grpc.CallOption) (*analysispb.LookupBugResponse, error) {
	return m.lookupBug(ctx, in)
}

func (m *mockRulesClient) Get(ctx context.Context, in *analysispb.GetRuleRequest, opts ...grpc.CallOption) (*analysispb.Rule, error) {
	return m.get(ctx, in)
}

type mockClustersClient struct {
	analysispb.ClustersClient
	get                  func(ctx context.Context, in *analysispb.GetClusterRequest) (*analysispb.Cluster, error)
	queryClusterFailures func(ctx context.Context, in *analysispb.QueryClusterFailuresRequest) (*analysispb.QueryClusterFailuresResponse, error)
}

func (m *mockClustersClient) Get(ctx context.Context, in *analysispb.GetClusterRequest, opts ...grpc.CallOption) (*analysispb.Cluster, error) {
	if m.get == nil {
		return &analysispb.Cluster{Name: in.Name}, nil
	}
	return m.get(ctx, in)
}

func (m *mockClustersClient) QueryClusterFailures(ctx context.Context, in *analysispb.QueryClusterFailuresRequest, opts ...grpc.CallOption) (*analysispb.QueryClusterFailuresResponse, error) {
	if m.queryClusterFailures == nil {
		return &analysispb.QueryClusterFailuresResponse{}, nil
	}
	return m.queryClusterFailures(ctx, in)
}

func TestValidationAndResourceNames(t *testing.T) {
	ftt.Run(`validateBugID`, t, func(t *ftt.Test) {
		for _, valid := range []string{"553974454", "123", "1"} {
			assert.Loosely(t, validateBugID(valid), should.BeNil)
		}
		for _, invalid := range []string{"", "b/553974454", "http://b/123", "not-a-number", "-123", "12a"} {
			assert.Loosely(t, validateBugID(invalid), should.NotBeNil)
		}
	})

	ftt.Run(`validateBareID`, t, func(t *ftt.Test) {
		for _, valid := range []string{"chromium", "rules", "reason-v6", "32ae75255a4aa807897d8fa20137e7d5"} {
			assert.Loosely(t, validateBareID("-flag", valid), should.BeNil)
		}
		for _, invalid := range []string{"", "projects/chromium", "a/b", "a:b"} {
			assert.Loosely(t, validateBareID("-flag", invalid), should.NotBeNil)
		}
	})

	ftt.Run(`parseRuleResourceName`, t, func(t *ftt.Test) {
		proj, rID, err := parseRuleResourceName("projects/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, proj, should.Equal("turquoise"))
		assert.Loosely(t, rID, should.Equal("32ae75255a4aa807897d8fa20137e7d5"))

		for _, bad := range []string{"", "projects/turquoise", "projects//rules/abc", "projects/turquoise/clusters/abc"} {
			_, _, err := parseRuleResourceName(bad)
			assert.Loosely(t, err, should.NotBeNil)
		}
	})
}

func TestTargetResolution(t *testing.T) {
	ctx := context.Background()

	ftt.Run(`resolveRuleTargets validation and error cases`, t, func(t *ftt.Test) {
		rulesMock := &mockRulesClient{
			lookupBug: func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error) {
				if in.Id == "404" {
					return &analysispb.LookupBugResponse{}, nil
				}
				if in.Id == "500" {
					return nil, errors.New("backend unavailable")
				}
				return &analysispb.LookupBugResponse{
					Rules: []string{"projects/turquoise/rules/r1"},
				}, nil
			},
		}

		// Mutual exclusion: both -bugid and -ruleid
		_, err := resolveRuleTargets(ctx, rulesMock, "553974454", "chromium", "rule123")
		assert.Loosely(t, err, should.NotBeNil)

		// Missing both -bugid and -ruleid
		_, err = resolveRuleTargets(ctx, rulesMock, "", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Missing -project with bare -ruleid
		_, err = resolveRuleTargets(ctx, rulesMock, "", "", "rule123")
		assert.Loosely(t, err, should.NotBeNil)

		// Invalid -ruleid containing slashes
		_, err = resolveRuleTargets(ctx, rulesMock, "", "chromium", "projects/chromium/rules/rule123")
		assert.Loosely(t, err, should.NotBeNil)

		// Bare -ruleid with -project
		targets, err := resolveRuleTargets(ctx, rulesMock, "", "chromium", "rule123")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, targets[0].Project, should.Equal("chromium"))
		assert.Loosely(t, targets[0].RuleID, should.Equal("rule123"))
		assert.Loosely(t, targets[0].ResourceName, should.Equal("projects/chromium/rules/rule123"))

		// Valid -bugid
		targets, err = resolveRuleTargets(ctx, rulesMock, "553974454", "", "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, targets[0].Project, should.Equal("turquoise"))
		assert.Loosely(t, targets[0].RuleID, should.Equal("r1"))

		// Invalid -bugid format
		_, err = resolveRuleTargets(ctx, rulesMock, "b/553974454", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Empty LookupBug response
		_, err = resolveRuleTargets(ctx, rulesMock, "404", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// LookupBug RPC failure
		_, err = resolveRuleTargets(ctx, rulesMock, "500", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Filtered out by non-matching -project
		_, err = resolveRuleTargets(ctx, rulesMock, "553974454", "chromium", "")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run(`resolveClusterTarget validation and cluster formats`, t, func(t *ftt.Test) {
		rulesMock := &mockRulesClient{
			lookupBug: func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error) {
				return &analysispb.LookupBugResponse{
					Rules: []string{"projects/turquoise/rules/r1"},
				}, nil
			},
		}

		// Explicit cluster mode (rules)
		proj, alg, cID, _, err := resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "rules", "abc123")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, proj, should.Equal("chromium"))
		assert.Loosely(t, alg, should.Equal("rules"))
		assert.Loosely(t, cID, should.Equal("abc123"))

		// Explicit cluster mode (non-rule cluster algorithm)
		proj, alg, cID, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "reason-v6", "deadbeef")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, proj, should.Equal("chromium"))
		assert.Loosely(t, alg, should.Equal("reason-v6"))
		assert.Loosely(t, cID, should.Equal("deadbeef"))

		// Rule mode
		proj, alg, cID, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "abc123", "", "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, proj, should.Equal("chromium"))
		assert.Loosely(t, alg, should.Equal("rules"))
		assert.Loosely(t, cID, should.Equal("abc123"))

		// Bug mode
		proj, alg, cID, _, err = resolveClusterTarget(ctx, rulesMock, "553974454", "", "", "", "")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, proj, should.Equal("turquoise"))
		assert.Loosely(t, alg, should.Equal("rules"))
		assert.Loosely(t, cID, should.Equal("r1"))

		// Error cases: missing all target flags
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: conflicting modes (bug + rule)
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "553974454", "chromium", "abc123", "", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: conflicting modes (rule + cluster)
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "abc123", "rules", "abc123")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: cluster missing project
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "", "", "rules", "abc123")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: cluster missing clusterid
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "rules", "")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: cluster missing clusteralgorithm
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "", "abc123")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: non-bare cluster algorithm
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "rules/abc", "abc123")
		assert.Loosely(t, err, should.NotBeNil)

		// Error cases: non-bare cluster id
		_, _, _, _, err = resolveClusterTarget(ctx, rulesMock, "", "chromium", "", "rules", "abc/123")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run(`parseVariantFilters`, t, func(t *ftt.Test) {
		m, err := parseVariantFilters([]string{"board:vim3", "os=Ubuntu"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, m, should.Resemble(map[string]string{"board": "vim3", "os": "Ubuntu"}))

		_, err = parseVariantFilters([]string{"invalid-entry"})
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestRuleGet(t *testing.T) {
	ftt.Run(`fetchRuleDetails and printRuleDetails`, t, func(t *ftt.Test) {
		ctx := context.Background()
		lookupCalled := 0

		rulesMock := &mockRulesClient{
			lookupBug: func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error) {
				lookupCalled++
				assert.Loosely(t, in.System, should.Equal("buganizer"))
				assert.Loosely(t, in.Id, should.Equal("553974454"))
				return &analysispb.LookupBugResponse{
					Rules: []string{"projects/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5"},
				}, nil
			},
			get: func(ctx context.Context, in *analysispb.GetRuleRequest) (*analysispb.Rule, error) {
				assert.Loosely(t, in.Name, should.Equal("projects/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5"))
				return &analysispb.Rule{
					Name:                  in.Name,
					Project:               "turquoise",
					RuleId:                "32ae75255a4aa807897d8fa20137e7d5",
					RuleDefinition:        `test = "boot_test" AND reason LIKE "%serial timeout%"`,
					IsActive:              true,
					IsManagingBug:         true,
					IsManagingBugPriority: false,
					SourceCluster: &analysispb.ClusterId{
						Algorithm: "testname-v4",
						Id:        "0011223344",
					},
					Bug: &analysispb.AssociatedBug{
						System:   "buganizer",
						Id:       "553974454",
						LinkText: "b/553974454",
						Url:      "https://issuetracker.google.com/issues/553974454",
					},
					BugManagementState: &analysispb.BugManagementState{
						PolicyState: []*analysispb.BugManagementState_PolicyState{
							{PolicyId: "exoneration-v2", IsActive: true},
						},
					},
				}, nil
			},
		}

		clustersMock := &mockClustersClient{
			get: func(ctx context.Context, in *analysispb.GetClusterRequest) (*analysispb.Cluster, error) {
				assert.Loosely(t, in.Name, should.Equal("projects/turquoise/clusters/rules/32ae75255a4aa807897d8fa20137e7d5"))
				return &analysispb.Cluster{
					Name: in.Name,
					Metrics: map[string]*analysispb.Cluster_TimewiseCounts{
						"failures": {
							OneDay:   &analysispb.Cluster_Counts{Nominal: 20},
							ThreeDay: &analysispb.Cluster_Counts{Nominal: 65},
							SevenDay: &analysispb.Cluster_Counts{Nominal: 140},
						},
						"human-cls-failed-presubmit": {
							OneDay:   &analysispb.Cluster_Counts{Nominal: 2},
							ThreeDay: &analysispb.Cluster_Counts{Nominal: 5},
							SevenDay: &analysispb.Cluster_Counts{Nominal: 11},
						},
						"builds-with-test-runs-failed-due-to-flaky-tests": {
							OneDay:   &analysispb.Cluster_Counts{Nominal: 1},
							ThreeDay: &analysispb.Cluster_Counts{Nominal: 3},
							SevenDay: &analysispb.Cluster_Counts{Nominal: 7},
						},
					},
				}, nil
			},
			queryClusterFailures: func(ctx context.Context, in *analysispb.QueryClusterFailuresRequest) (*analysispb.QueryClusterFailuresResponse, error) {
				assert.Loosely(t, in.Parent, should.Equal("projects/turquoise/clusters/rules/32ae75255a4aa807897d8fa20137e7d5/failures"))
				return &analysispb.QueryClusterFailuresResponse{
					Failures: []*analysispb.DistinctClusterFailure{
						{
							IngestedInvocationId: "build-8700000000000000009",
							TestId:               "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm",
							PartitionTime:        timestamppb.New(time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)),
							Count:                3,
							BuildStatus:          analysispb.BuildStatus_BUILD_STATUS_FAILURE,
							Variant:              &analysispb.Variant{Def: map[string]string{"board": "vim3"}},
							FailureReasonPrefix:  "serial socket timed out waiting for prompt",
						},
					},
				}, nil
			},
		}

		details, err := fetchRuleDetails(ctx, rulesMock, clustersMock, "553974454", "", "", 3)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, lookupCalled, should.Equal(1))
		assert.Loosely(t, details, should.HaveLength(1))
		assert.Loosely(t, details[0].Examples, should.HaveLength(1))

		var textBuf bytes.Buffer
		assert.Loosely(t, printRuleDetails(&textBuf, details), should.BeNil)
		textOut := textBuf.String()
		assert.Loosely(t, textOut, should.ContainSubstring("Project:         turquoise\n"))
		assert.Loosely(t, textOut, should.ContainSubstring("Rule ID:         32ae75255a4aa807897d8fa20137e7d5\n"))
		assert.Loosely(t, textOut, should.ContainSubstring("Bug:             b/553974454 (https://issuetracker.google.com/issues/553974454)\n"))
		assert.Loosely(t, textOut, should.ContainSubstring("Active Policies: exoneration-v2\n"))
		assert.Loosely(t, textOut, should.ContainSubstring("Source Cluster:  testname-v4/0011223344\n"))
		assert.Loosely(t, textOut, should.ContainSubstring("20 / 65 / 140"))
		assert.Loosely(t, textOut, should.ContainSubstring("Example Failures (showing 1 of 1 distinct groups):"))
		assert.Loosely(t, textOut, should.ContainSubstring(`-invocationid build-8700000000000000009 -testid "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm"`))
		assert.Loosely(t, textOut, should.ContainSubstring("luci verdict get -invocationid build-8700000000000000009"))
		assert.Loosely(t, textOut, should.ContainSubstring("luci analysis failures -project turquoise -ruleid 32ae75255a4aa807897d8fa20137e7d5"))

		// Verify -limit 0 skips example failures lookup
		noExDetails, err := fetchRuleDetails(ctx, rulesMock, clustersMock, "553974454", "", "", 0)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, noExDetails[0].Examples, should.BeEmpty)

		// Test redacted rule definition and multiple rules output
		multi := []ruleDetail{
			details[0],
			{
				Rule: &analysispb.Rule{
					Name:    "projects/chromium/rules/redacted-id",
					Project: "chromium",
					RuleId:  "redacted-id",
					Bug:     &analysispb.AssociatedBug{System: "buganizer", Id: "553974454"},
				},
				Cluster: &analysispb.Cluster{Name: "projects/chromium/clusters/rules/redacted-id"},
			},
		}
		var multiText bytes.Buffer
		assert.Loosely(t, printRuleDetails(&multiText, multi), should.BeNil)
		assert.Loosely(t, multiText.String(), should.ContainSubstring("Rule Definition: <redacted or unset>\n"))
		assert.Loosely(t, multiText.String(), should.ContainSubstring("Bug:             buganizer:553974454\n"))
	})
}

func TestFailuresCommand(t *testing.T) {
	ftt.Run(`fetchClusterFailures single-turn -bugid shortcut and variant distribution`, t, func(t *ftt.Test) {
		ctx := context.Background()

		rulesMock := &mockRulesClient{
			lookupBug: func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error) {
				assert.Loosely(t, in.System, should.Equal("buganizer"))
				assert.Loosely(t, in.Id, should.Equal("553974454"))
				return &analysispb.LookupBugResponse{
					Rules: []string{"projects/turquoise/rules/32ae75255a4aa807897d8fa20137e7d5"},
				}, nil
			},
		}

		t1 := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
		t3 := time.Date(2026, 10, 6, 14, 0, 0, 0, time.UTC)

		clustersMock := &mockClustersClient{
			queryClusterFailures: func(ctx context.Context, in *analysispb.QueryClusterFailuresRequest) (*analysispb.QueryClusterFailuresResponse, error) {
				assert.Loosely(t, in.Parent, should.Equal("projects/turquoise/clusters/rules/32ae75255a4aa807897d8fa20137e7d5/failures"))
				return &analysispb.QueryClusterFailuresResponse{
					Failures: []*analysispb.DistinctClusterFailure{
						{
							IngestedInvocationId: "build-8700000000000000001",
							TestId:               "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm",
							PartitionTime:        timestamppb.New(t1),
							Count:                24,
							BuildStatus:          analysispb.BuildStatus_BUILD_STATUS_FAILURE,
							Variant: &analysispb.Variant{Def: map[string]string{
								"board":   "iris",
								"builder": "bringup.iris-debug",
								"shard":   "s1",
							}},
							FailureReasonPrefix: "ZBI kernel panic during early boot",
						},
						{
							IngestedInvocationId: "build-8700000000000000002",
							TestId:               "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm",
							PartitionTime:        timestamppb.New(t2),
							Count:                38,
							BuildStatus:          analysispb.BuildStatus_BUILD_STATUS_FAILURE,
							Variant: &analysispb.Variant{Def: map[string]string{
								"board":   "vim3",
								"builder": "core.vim3-debug",
								"shard":   "s2",
							}},
							FailureReasonPrefix: "failed to boot vim3 target over fastboot",
						},
						{
							IngestedInvocationId: "8700000000000000003",
							TestId:               "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm",
							PartitionTime:        timestamppb.New(t3),
							Count:                78,
							BuildStatus:          analysispb.BuildStatus_BUILD_STATUS_SUCCESS,
							Exonerations: []*analysispb.DistinctClusterFailure_Exoneration{
								{Reason: analysispb.ExonerationReason_OCCURS_ON_MAINLINE},
							},
							Variant: &analysispb.Variant{Def: map[string]string{
								"board":   "qemu-x64",
								"builder": "core.x64-debug",
								"shard":   "s3",
							}},
							FailureReasonPrefix: "serial socket timed out waiting for prompt",
						},
					},
				}, nil
			},
		}

		// 1. Unfiltered -bugid query summarizes all 3 boards across 140 failures even with -limit 2
		report, err := fetchClusterFailures(ctx, rulesMock, clustersMock, "553974454", "", "", "", "", "", nil, 2, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, report.TotalFailures, should.Equal(int64(140)))
		assert.Loosely(t, report.DistinctGroupsMatched, should.Equal(3))
		assert.Loosely(t, report.Examples, should.HaveLength(2))
		assert.Loosely(t, report.Examples[0].InvocationID, should.Equal("build-8700000000000000003"))

		var buf bytes.Buffer
		assert.Loosely(t, printClusterFailuresReport(&buf, report), should.BeNil)
		out := buf.String()
		assert.Loosely(t, out, should.ContainSubstring("Variant Distribution (140 failures across 3 distinct groups):"))
		assert.Loosely(t, out, should.ContainSubstring("qemu-x64=78 (55.7%), vim3=38 (27.1%), iris=24 (17.1%)"))
		assert.Loosely(t, out, should.ContainSubstring(`-invocationid build-8700000000000000003 -testid "fuchsia-pkg://fuchsia.com/boot-test#meta/boot.cm"`))
		assert.Loosely(t, out, should.ContainSubstring("luci verdict get -invocationid build-8700000000000000003"))

		// 2. Filtering with -variant board:vim3 narrows strictly to physical vim3 failures
		filteredReport, err := fetchClusterFailures(ctx, rulesMock, clustersMock, "553974454", "", "", "", "", "", map[string]string{"board": "vim3"}, 20, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, filteredReport.TotalFailures, should.Equal(int64(38)))
		assert.Loosely(t, filteredReport.Examples, should.HaveLength(1))
		assert.Loosely(t, filteredReport.Examples[0].InvocationID, should.Equal("build-8700000000000000002"))
	})

	ftt.Run(`Suggested cluster metadata (title & equivalent rule) and adaptive <100-line output cap`, t, func(t *ftt.Test) {
		ctx := context.Background()
		rulesMock := &mockRulesClient{}
		clustersMock := &mockClustersClient{
			get: func(ctx context.Context, in *analysispb.GetClusterRequest) (*analysispb.Cluster, error) {
				assert.Loosely(t, in.Name, should.Equal("projects/turquoise/clusters/reason-v6/a383d24efaeb52bc2e136b21413961f5"))
				return &analysispb.Cluster{
					Name:                             in.Name,
					Title:                            "Failed to pass run, expected status 0 got 1",
					EquivalentFailureAssociationRule: `reason LIKE "Failed to pass run, expected status % got %"`,
				}, nil
			},
			queryClusterFailures: func(ctx context.Context, in *analysispb.QueryClusterFailuresRequest) (*analysispb.QueryClusterFailuresResponse, error) {
				return &analysispb.QueryClusterFailuresResponse{
					Failures: []*analysispb.DistinctClusterFailure{
						{
							IngestedInvocationId: "build-8700000000000000010",
							TestId:               "test_a",
							Count:                2,
							Variant:              &analysispb.Variant{Def: map[string]string{"d01": "v1", "d02": "v1", "d03": "v1", "d04": "v1", "d05": "v1", "d06": "v1", "d07": "v1", "d08": "v1", "d09": "v1", "d10": "v1", "d11": "v1", "d12": "v1", "d13": "v1", "d14": "v1"}},
						},
						{
							IngestedInvocationId: "build-8700000000000000011",
							TestId:               "test_b",
							Count:                2,
							Variant:              &analysispb.Variant{Def: map[string]string{"d01": "v2", "d02": "v2", "d03": "v2", "d04": "v2", "d05": "v2", "d06": "v2", "d07": "v2", "d08": "v2", "d09": "v2", "d10": "v2", "d11": "v2", "d12": "v2", "d13": "v2", "d14": "v2"}},
						},
						{
							IngestedInvocationId: "build-8700000000000000012",
							TestId:               "test_c",
							Count:                2,
							Variant:              &analysispb.Variant{Def: map[string]string{"d01": "v3", "d02": "v3", "d03": "v3", "d04": "v3", "d05": "v3", "d06": "v3", "d07": "v3", "d08": "v3", "d09": "v3", "d10": "v3", "d11": "v3", "d12": "v3", "d13": "v3", "d14": "v3"}},
						},
					},
				}, nil
			},
		}

		// With 14 discriminating variant dimensions and adaptive=true, default limit=3 automatically drops to 1 example
		report, err := fetchClusterFailures(ctx, rulesMock, clustersMock, "", "turquoise", "", "reason-v6", "a383d24efaeb52bc2e136b21413961f5", "", nil, defaultFailuresLimit, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, report.Title, should.Equal("Failed to pass run, expected status 0 got 1"))
		assert.Loosely(t, report.SuggestedRule, should.Equal(`reason LIKE "Failed to pass run, expected status % got %"`))
		assert.Loosely(t, report.Examples, should.HaveLength(1))

		var buf bytes.Buffer
		assert.Loosely(t, printClusterFailuresReport(&buf, report), should.BeNil)
		out := buf.String()
		assert.Loosely(t, out, should.ContainSubstring("Title:   Failed to pass run, expected status 0 got 1\n"))
		assert.Loosely(t, out, should.ContainSubstring(`Rule:    reason LIKE "Failed to pass run, expected status % got %"`))
		assert.Loosely(t, out, should.ContainSubstring("... +2 more variant dimensions\n"))
	})

	ftt.Run(`Multi-rule cross-project disambiguation, empty failures, and fallback variant buckets`, t, func(t *ftt.Test) {
		ctx := context.Background()

		rulesMock := &mockRulesClient{
			lookupBug: func(ctx context.Context, in *analysispb.LookupBugRequest) (*analysispb.LookupBugResponse, error) {
				return &analysispb.LookupBugResponse{
					Rules: []string{
						"projects/chrome/rules/inactive-rule-111",
						"projects/chromium/rules/active-rule-222",
					},
				}, nil
			},
			get: func(ctx context.Context, in *analysispb.GetRuleRequest) (*analysispb.Rule, error) {
				return &analysispb.Rule{
					Name:     in.Name,
					IsActive: in.Name == "projects/chromium/rules/active-rule-222",
				}, nil
			},
		}

		clustersMock := &mockClustersClient{
			queryClusterFailures: func(ctx context.Context, in *analysispb.QueryClusterFailuresRequest) (*analysispb.QueryClusterFailuresResponse, error) {
				assert.Loosely(t, in.Parent, should.Equal("projects/chromium/clusters/rules/active-rule-222/failures"))
				assert.Loosely(t, in.MetricFilter, should.Equal("projects/chromium/metrics/human-cls-failed-presubmit"))
				return &analysispb.QueryClusterFailuresResponse{}, nil
			},
		}

		report, err := fetchClusterFailures(ctx, rulesMock, clustersMock, "999999", "", "", "", "", "human-cls-failed-presubmit", nil, 20, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, report.Project, should.Equal("chromium"))
		assert.Loosely(t, report.ClusterID, should.Equal("active-rule-222"))
		assert.Loosely(t, report.AdditionalRules, should.Resemble([]string{"projects/chrome/rules/inactive-rule-111"}))

		var emptyOut bytes.Buffer
		assert.Loosely(t, printClusterFailuresReport(&emptyOut, report), should.BeNil)
		assert.Loosely(t, emptyOut.String(), should.ContainSubstring("No matching cluster failures found."))

		// Verify +N more truncation and fallback variant keys when >6 values exist
		dist := buildVariantDistribution([]*analysispb.DistinctClusterFailure{
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g1"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g2"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g3"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g4"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g5"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g6"}}},
			{Count: 1, Variant: &analysispb.Variant{Def: map[string]string{"gpu": "g7"}}},
		}, 7)
		assert.Loosely(t, dist, should.HaveLength(1))
		assert.Loosely(t, dist[0].Key, should.Equal("gpu"))
		truncReport := &clusterFailuresReport{
			Project:               "chromium",
			ClusterAlgorithm:      "rules",
			ClusterID:             "active-rule-222",
			TotalFailures:         7,
			DistinctGroupsMatched: 7,
			VariantDistribution:   dist,
		}
		var truncOut bytes.Buffer
		assert.Loosely(t, printClusterFailuresReport(&truncOut, truncReport), should.BeNil)
		assert.Loosely(t, truncOut.String(), should.ContainSubstring("+1 more"))
	})
}

func TestSubcommandRunners(t *testing.T) {
	af := &base.AuthFlags{}
	app := &cli.Application{
		Name: "luci",
		Context: func(ctx context.Context) context.Context {
			return ctx
		},
	}

	ftt.Run(`Cmd and RuleCmd help dispatch`, t, func(t *ftt.Test) {
		topCmd := Cmd(af)
		assert.Loosely(t, topCmd.CommandRun().Run(app, []string{"help"}, nil), should.Equal(0))
		assert.Loosely(t, topCmd.CommandRun().Run(app, []string{"rule", "-help"}, nil), should.Equal(0))

		ruleCmd := RuleCmd(af)
		assert.Loosely(t, ruleCmd.CommandRun().Run(app, []string{"help"}, nil), should.Equal(0))
	})

	ftt.Run(`RuleGetCmd and FailuresCmd CLI validation`, t, func(t *ftt.Test) {
		getRun := RuleGetCmd(af).CommandRun().(*ruleGetRun)
		// Reject positional arguments
		assert.Loosely(t, getRun.Run(app, []string{"unexpected"}, nil), should.Equal(1))

		failRun := FailuresCmd(af).CommandRun().(*failuresRun)
		// Reject positional arguments
		assert.Loosely(t, failRun.Run(app, []string{"unexpected"}, nil), should.Equal(1))
		// Reject invalid variant filter
		failRun.variants = []string{"badvariant"}
		assert.Loosely(t, failRun.Run(app, nil, nil), should.Equal(1))
	})
}
