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

// Package metrics provides a framework for cluster-based metrics.
package metrics

import (
	"fmt"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

// MetricIDPattern defines the valid format for metric identifiers.
// Metric identifiers must be valid google.aip.dev/122 resource ID segments.
const MetricIDPattern = `[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?`

// metricIDRE defines a regular expression that matches valid metrc identifiers.
var metricIDRE = regexp.MustCompile(`^` + MetricIDPattern + `$`)

// Standard metrics.
var (
	// The number of distinct developer changelists that failed at least one
	// presubmit (CQ) run because of failure(s) in this cluster. Excludes
	// changelists authored by automation.
	HumanClsFailedPresubmit = metricBuilder{
		ID:                "human-cls-failed-presubmit",
		HumanReadableName: "User Cls Failed Presubmit",
		Description:       "The number of distinct developer changelists that failed at least one presubmit (CQ) run because of failure(s) in a cluster.",
		DefaultConfig: Configuration{
			SortPriority:          400,
			IsDefault:             true,
			ShowInMetricsSelector: true,
		},
		// Human presubmit full-runs failed due to a failure in the cluster.
		//
		// TODO(meiring): Remove `f.presubmit_run_status = 'FAILED'` criteria
		// and filter to latest LUCI CV tryjob tries once LUCI CV exposes
		// try data. Current criteria captures critical failing tryjobs in
		// failing CV runs even if those tryjobs were later retried and passed
		// (and hence not the cause of CV run failure).
		FilterSQL: `f.is_ingested_invocation_blocked AND COALESCE(ARRAY_LENGTH(f.exonerations) = 0, TRUE) AND f.build_status = 'FAILURE' ` +
			`AND f.build_critical AND f.presubmit_run_mode = 'FULL_RUN' AND f.presubmit_run_owner='user' AND f.presubmit_run_status = 'FAILED'`,
		// Distinct CLs. Note that indexing with SAFE_OFFSET returns
		// NULL if there is no such element, and CONCAT returns NULL
		// if any argument is NULL.
		CountSQL: `CONCAT(f.sources.changelists[SAFE_OFFSET(0)].host, '/', f.sources.changelists[SAFE_OFFSET(0)].change)`,
	}.Build()

	// The number of verdicts on test variants which were configured to be
	// presubmit-blocking, which were exonerated (i.e. did not actually block
	// presubmit) because infrastructure determined the test variant to be
	// failing or too flaky at tip-of-tree. If this number is non-zero, it
	// means a test variant which was configured to be presubmit-blocking is
	// not stable enough to do so, and should be fixed or made non-blocking.
	CriticalFailuresExonerated = metricBuilder{
		ID:                "critical-failures-exonerated",
		HumanReadableName: "Presubmit-blocking Verdicts Exonerated",
		Description:       "The number of presubmit-blocking test verdicts which were exonerated (i.e. did not actually block presubmit) because infrastructure determined the test variant to be failing or too flaky at tip-of-tree.",
		DefaultConfig: Configuration{
			SortPriority:          500,
			IsDefault:             true,
			ShowInMetricsSelector: true,
		},

		// Exonerated for a reason other than NOT_CRITICAL or UNEXPECTED_PASS.
		// Passes are not ingested by LUCI Analysis, but if a test has both an unexpected pass
		// and an unexpected failure, it will be exonerated for the unexpected pass.
		// TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
		FilterSQL: `f.build_critical AND (EXISTS (SELECT TRUE FROM UNNEST(f.exonerations) e WHERE e.Reason = 'OCCURS_ON_OTHER_CLS'))`,

		// Distinct test verdicts.
		CountSQL: `CONCAT(f.ingested_invocation_id, '/', f.test_id, '/', f.variant_hash)`,
	}.Build()

	// The number of test runs that failed. Test runs are generally
	// equivalent to swarming tasks.
	TestRunsFailed = metricBuilder{
		ID:                "test-runs-failed",
		HumanReadableName: "Test Runs Failed",
		Description:       "The number of distinct test runs (i.e. swarming tasks or builds) failed due to failures in a cluster.",
		DefaultConfig: Configuration{
			SortPriority:          200,
			ShowInMetricsSelector: true,
		},
		FilterSQL: `f.is_test_run_blocked`,
		CountSQL:  `f.test_run_id`,
	}.Build()

	// The total number of test results in this cluster. LUCI Analysis only
	// clusters test results which are failed. (We also exclude unexpectedly
	// passed web tests).
	Failures = metricBuilder{
		ID:                "failures",
		HumanReadableName: "Test Results Failed",
		Description:       "The total number of test results in a cluster. LUCI Analysis only clusters failed test results.",
		DefaultConfig: Configuration{
			SortPriority:          100,
			IsDefault:             true,
			ShowInMetricsSelector: true,
		},
	}.Build()

	BuildsWithTestRunsFailedDueToFlakyTests = metricBuilder{
		// N.B. The metric ID does not quite match teh definition as it deliberately re-uses the ID of a
		// metric we wish to replace.
		ID:                "builds-failed-due-to-flaky-tests",
		HumanReadableName: "Builds with Test Runs Failed by Flaky Test Variants",
		Description: "The number of builds monitored by a gardener rotation which had failing test runs " +
			"(i.e. all attempts of a test variant within a single swarming task failed) because of flaky test variants. " +
			" To be considered flaky, the test variant must have seen at least one flaky verdict on the same branch in the last 24 hours.",
		DefaultConfig: Configuration{
			SortPriority:          300,
			ShowInMetricsSelector: true,
		},
		// Criteria:
		// - The test result's build is part of a gardener rotation, and
		// - The test had only unexpected non-passed results
		//   (excluding skips) in a test run, and
		// - The verdict was not exonerated, and
		// - There was a flaky verdict in the last 24 hours.
		FilterSQL: "ARRAY_LENGTH(f.build_gardener_rotations) > 0 AND " +
			"f.is_test_run_blocked AND " +
			"(f.exonerations IS NULL OR ARRAY_LENGTH(f.exonerations) = 0) AND " +
			"f.test_variant_branch.flaky_verdicts_24h > 0",
		// Count distinct builds.
		CountSQL: "f.ingested_invocation_id",
	}.Build()

	FailuresWithAttributedFilteredTestRuns = metricBuilder{
		ID:                "failures-with-attributed-filtered-test-runs",
		HumanReadableName: "Failures with Attributed Filtered Test Runs",
		Description: "The number of failures in this cluster that has filtered test runs being attributed to them," +
			" which means those failures caused some tests to be filtered out in the test scheduler.",
		DefaultConfig: Configuration{
			SortPriority:          350,
			ShowInMetricsSelector: true,
		},
		RequireAttrs: true,
		FilterSQL:    "attrs.attributed_filtered_run_count > 0",
	}.Build()

	// The number of builds that had flakes in presubmit in this cluster.
	BuildsWithFlakesInPresubmit = metricBuilder{
		ID:                "builds-with-flakes-in-presubmit",
		HumanReadableName: "Builds with Flakes in Presubmit",
		Description:       "The total number of builds with at least one flaky test verdict in presubmit, due to this cluster.",
		DefaultConfig: Configuration{
			SortPriority:          250,
			ShowInMetricsSelector: false,
		},
		// Criteria:
		// - A test fails and then passes upon retry in the same invocation
		// - It is also necessary to ignore test failures from tasks where more than 6 tests failed,
		//   because they likely all have the same underlying cause and should not be considered
		//   separate flakes. This can be done by checking for the flake_analysis_ignore tag.
		// - Bugs should also only be filed for top level tests: check for the is_top_level_test tag.
		FilterSQL: `NOT f.is_ingested_invocation_blocked AND f.presubmit_run_id.id IS NOT NULL AND NOT EXISTS(SELECT key, value FROM UNNEST(f.tags) WHERE key = 'flake_analysis_ignore') AND EXISTS(SELECT key, value FROM UNNEST(f.tags) WHERE key = 'is_top_level_test')`,
		// Count distinct builds.
		CountSQL: `f.ingested_invocation_id`,
	}.Build()

	// The number of gardened builds that had failing test runs.
	MonitoredBuildsWithTestRunFailures = metricBuilder{
		ID:                "monitored-builds-with-test-run-failures",
		HumanReadableName: "Monitored Builds with Test Run Failures",
		Description:       "The number of gardened builds that had failing test runs.",
		DefaultConfig: Configuration{
			SortPriority:          600,
			ShowInMetricsSelector: false,
		},
		// Criteria:
		// - A test run fails in a gardened builder
		// - It is also necessary to ignore test failures from tasks where more than 6 tests failed,
		//   because they likely all have the same underlying cause and should not be considered
		//   separate flakes. This can be done by checking for the flake_analysis_ignore tag.
		// - Bugs should also only be filed for top level tests: check for the is_top_level_test tag.
		FilterSQL: `f.is_test_run_blocked AND ARRAY_LENGTH(f.build_gardener_rotations) > 0 AND NOT EXISTS(SELECT key, value FROM UNNEST(f.tags) WHERE key = 'flake_analysis_ignore') AND EXISTS(SELECT key, value FROM UNNEST(f.tags) WHERE key = 'is_top_level_test')`,
		// Count distinct builds.
		CountSQL: `f.ingested_invocation_id`,
	}.Build()

	// ComputedMetrics is the set of metrics computed for each cluster and
	// stored on the cluster summaries table.
	ComputedMetrics = []BaseDefinition{
		HumanClsFailedPresubmit,
		CriticalFailuresExonerated,
		TestRunsFailed,
		Failures,
		BuildsWithTestRunsFailedDueToFlakyTests,
		FailuresWithAttributedFilteredTestRuns,
		BuildsWithFlakesInPresubmit,
		MonitoredBuildsWithTestRunFailures,
	}
)

// ByID returns the metric with the given ID, if any.
func ByID(id ID) (BaseDefinition, error) {
	for _, metric := range ComputedMetrics {
		if metric.ID == id {
			return metric, nil
		}
	}
	return BaseDefinition{}, errors.Fmt("no metric with ID %q", id.String())
}

// MustByID returns the metric with the given ID and panic if no metric with the id exists.
func MustByID(id ID) BaseDefinition {
	for _, metric := range ComputedMetrics {
		if metric.ID == id {
			return metric
		}
	}
	panic(fmt.Sprintf("no metric with ID %q", id.String()))
}

// metricBuilder provides a way of building a new metric definition.
type metricBuilder struct {
	// ID is the identifier of the metric. It must be a valid AIP-122
	// Resource ID Segment (matching `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`),
	// as it will be prefixed by "metrics/" to become the metric's
	// resource name.
	ID string

	// A human readable name for the metric. Appears on the user interface.
	HumanReadableName string

	// A human readable descripton of the metric. Appears on the user interface
	// behind a help tooltip.
	Description string

	// The default values of metric properties that can be overriden by
	// individual LUCI Projects.
	DefaultConfig Configuration

	// RequireAttrs indicates whether calculating this metric requires joining the
	// failure_attributes table. See Definition.RequireAttrs for more details.
	RequireAttrs bool

	// A predicate on failures, that defines which failures are included in
	// the metric. See Definition.FilterSQL for more details.
	FilterSQL string

	// An expression that defines the distinct items to count.
	// See Definition.CountSQL for more details.
	CountSQL string
}

func (m metricBuilder) Build() BaseDefinition {
	if !metricIDRE.MatchString(m.ID) {
		panic(fmt.Sprintf("metric ID %q does not match expected pattern", m.ID))
	}
	// The UI makes assumptions that these fields will be set.
	if m.HumanReadableName == "" {
		panic("human readable name must be set")
	}
	if m.Description == "" {
		panic("description must be set")
	}
	if m.DefaultConfig.SortPriority <= 0 {
		panic("sort priority must be set to a positive integer")
	}
	return BaseDefinition{
		ID:                ID(m.ID),
		HumanReadableName: m.HumanReadableName,
		Description:       m.Description,
		DefaultConfig:     m.DefaultConfig,
		BaseColumnName:    strings.ReplaceAll(m.ID, "-", "_"),
		RequireAttrs:      m.RequireAttrs,
		FilterSQL:         m.FilterSQL,
		CountSQL:          m.CountSQL,
	}
}

// ID is an identifier of a metric. For example, "human-cls-failed-presubmit".
// It should be a valid AIP-122 Resource ID Segment.
type ID string

func (i ID) String() string {
	return string(i)
}

// Configuration represents properties of a metric that can be overriden
// by a LUCI project.
type Configuration struct {
	// SortPriority is a number that defines the order by which metrics are
	// sorted by default.The metric with the highest sort priority
	// will define the primary sort order, followed by the metric with the
	// second highest sort priority, and so on.
	SortPriority int

	// IsDefault indicates whether this metric is shown by default in the UI.
	IsDefault bool

	// ShowInMetricsSelector indicates whether this metric will be shown in the metrics selector in the LUCI milo UI.
	ShowInMetricsSelector bool
}

// BaseDefinition represents the built-in definition of a metric.
// It does not include the parts of the metric definition which can
// be overriden by individual LUCI Project.
type BaseDefinition struct {
	// ID is the identifier of the metric, for example "human-cls-failed-presubmit".
	// This is the same as the AIP-122 resource name of the metric,
	// excluding "metrics/".
	ID ID

	// A human readable name for the metric. Appears on the user interface.
	HumanReadableName string

	// A human readable descripton of the metric. Appears on the user interface
	// behind a help tooltip.
	Description string

	// BaseColumnName is the name of the metric to use in SQL column names.
	// It is the same as the identifier ID, but with hypens (-) replaced with
	// underscores.
	BaseColumnName string

	// RequireAttrs indicates whether calculating this metric requires joining the
	// failure_attributes table. When set to true, fields on the
	// failure_attributes table may be accessed via the prefix "attrs." by the
	// metric definition SQLs.
	RequireAttrs bool

	// The predicate on failures, that defines when an item is eligible to be
	// counted. Fields on the clustered_failures table may be accessed via the
	// prefix "f.". If `RequireAttrs` is set to true, fields on the
	// failure_attributes table may be accessed via the prefix "attrs.".
	//
	// For example, to count over failures on critical builds, use:
	// "f.build_critical".
	//
	// If no filtering is desired, this is left blank.
	FilterSQL string

	// An expression that defines the distinct items to count. Fields on the
	// clustered_failures table may be accessed via the prefix "f.". If
	// `RequireAttrs` is set to true, fields on the failure_attributes table may
	// be accessed via the prefix "attrs.".
	//
	// For example, to count distinct changelists, use:
	// `IF(ARRAY_LENGTH(f.sources.changelists)>0,
	//	  CONCAT(f.sources.changelists[OFFSET(0)].host, f.sources.changelists[OFFSET(0)].change),
	// NULL)`
	// While this may return NULL for items not to be counted, it is generally
	// preferred to use FilterSQL for that purpose.
	//
	// If failures are to be counted instead of distinct items, this is left blank.
	CountSQL string

	// DefaultConfig represents the default configuration for the metric.
	DefaultConfig Configuration
}

// ColumnName returns a column name to use for the metric in queries, with
// metric_ prefix used to namespace the column name to avoid name collisions
// with other predefined columns, and the specified suffix used to namespace
// the column name within other columns for the same metric.
func (m BaseDefinition) ColumnName(suffix string) string {
	return "metric_" + m.BaseColumnName + "_" + suffix
}

// AdaptToProject completes the definition of a built-in metric, by
// attaching LUCI Project-specific configuration.
func (m BaseDefinition) AdaptToProject(project string, cfg *configpb.Metrics) Definition {
	var overrides *configpb.Metrics_MetricOverride
	for _, o := range cfg.GetOverrides() {
		if o.MetricId == string(m.ID) {
			overrides = o
			break
		}
	}
	config := m.DefaultConfig
	if overrides != nil {
		if overrides.IsDefault != nil {
			config.IsDefault = *overrides.IsDefault
		}
		if overrides.SortPriority != nil {
			config.SortPriority = int(*overrides.SortPriority)
		}
		if overrides.ShowInMetricsSelector != nil {
			config.ShowInMetricsSelector = *overrides.ShowInMetricsSelector
		}
	}
	return Definition{
		Name:           fmt.Sprintf("projects/%s/metrics/%s", project, string(m.ID)),
		BaseDefinition: m,
		Config:         config,
	}
}

// Definition represents the complete definition of a metric for a LUCI Project.
// It includes the values of metric properties that the project can override.
type Definition struct {
	BaseDefinition

	// The AIP-122 resource name of the metric.
	// E.g. "projects/chromium/metrics/human-cls-failed-presubmit"
	Name string

	// Config represents the configuration of the metric for the LUCI Project.
	Config Configuration
}
