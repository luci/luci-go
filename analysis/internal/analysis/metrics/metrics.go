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
		SortPriority:      30,
		IsDefault:         true,

		// Human presubmit full-runs failed due to a failure in the cluster.
		FilterSQL: `f.is_ingested_invocation_blocked AND COALESCE(ARRAY_LENGTH(f.exonerations) = 0, TRUE) AND f.build_status = 'FAILURE' ` +
			`AND f.build_critical AND f.presubmit_run_mode = 'FULL_RUN' AND f.presubmit_run_owner='user'`,
		// Distinct CLs. Note that indexing with SAFE_OFFSET returns
		// NULL if there is no such element, and CONCAT returns NULL
		// if any argument is NULL.
		CountSQL: `CONCAT(f.changelists[SAFE_OFFSET(0)].host, '/', f.changelists[SAFE_OFFSET(0)].change)`,
	}.Build()

	// The number of failures on test variants which were configured to be
	// presubmit-blocking, which were exonerated (i.e. did not actually block
	// presubmit) because infrastructure determined the test variant to be
	// failing or too flaky at tip-of-tree. If this number is non-zero, it
	// means a test variant which was configured to be presubmit-blocking is
	// not stable enough to do so, and should be fixed or made non-blocking.
	CriticalFailuresExonerated = metricBuilder{
		ID:                "critical-failures-exonerated",
		HumanReadableName: "Presubmit-blocking Failures Exonerated",
		Description:       "The number of failures on test variants which were configured to be presubmit-blocking, which were exonerated (i.e. did not actually block presubmit) because infrastructure determined the test variant to be failing or too flaky at tip-of-tree. If this number is non-zero, it means a test variant which was configured to be presubmit-blocking is not stable enough to do so, and should be fixed or made non-blocking.",
		SortPriority:      40,
		IsDefault:         true,

		// Exonerated for a reason other than NOT_CRITICAL or UNEXPECTED_PASS.
		// Passes are not ingested by LUCI Analysis, but if a test has both an unexpected pass
		// and an unexpected failure, it will be exonerated for the unexpected pass.
		// TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
		FilterSQL: `f.build_critical AND (EXISTS (SELECT TRUE FROM UNNEST(f.exonerations) e WHERE e.Reason = 'OCCURS_ON_OTHER_CLS'))`,
	}.Build()

	// The number of test runs that failed. Test runs are generally
	// equivalent to swarming tasks.
	TestRunsFailed = metricBuilder{
		ID:                "test-runs-failed",
		HumanReadableName: "Test Runs Failed",
		Description:       "The number of distinct test runs (i.e. swarming tasks or builds) failed due to failures in a cluster.",
		SortPriority:      20,

		FilterSQL: `f.is_test_run_blocked`,
		CountSQL:  `f.test_run_id`,
	}.Build()

	// The total number of test results in this cluster. LUCI Analysis only
	// clusters test results which are unexpected and have a status of crash,
	// abort or fail, so by definition the only test results counted here
	// will be an unexpected fail/crash/abort.
	Failures = metricBuilder{
		ID:                "failures",
		HumanReadableName: "Test Results Failed",
		Description:       "The total number of test results in a cluster. LUCI Analysis only clusters test results which are unexpected and have a status of crash, abort or fail.",
		SortPriority:      10,
		IsDefault:         true,
	}.Build()

	BuildsFailedDueToFlakyTests = metricBuilder{
		ID:                "builds-failed-due-to-flaky-tests",
		HumanReadableName: "Builds Failed by Flaky Test Variants",
		Description: "The number of builds monitored by a gardener rotation which failed because of flaky test variants. To be considered flaky," +
			" the test variant must have seen at least one flaky verdict on the same branch in the last 24 hours.",
		SortPriority: 25,
		// Criteria:
		// - The test result's build is part of a gardener rotation, and
		// - The verdict was only unexpected non-passed results
		//   (excluding skips), and
		// - The verdict was not exonerated, and
		// - There was a flaky verdict in the last 24 hours.
		FilterSQL: "ARRAY_LENGTH(f.build_gardener_rotations) > 0 AND " +
			"f.is_ingested_invocation_blocked AND " +
			"(f.exonerations IS NULL OR ARRAY_LENGTH(f.exonerations) = 0) AND " +
			"f.test_variant_branch.flaky_verdicts_24h > 0",
		// Count distinct builds.
		CountSQL: "f.ingested_invocation_id",
	}.Build()

	// ComputedMetrics is the set of metrics computed for each cluster and
	// stored on the cluster summaries table.
	ComputedMetrics = []Definition{HumanClsFailedPresubmit, CriticalFailuresExonerated, TestRunsFailed, Failures, BuildsFailedDueToFlakyTests}
)

// ByID returns the metric with the given ID, if any.
func ByID(id ID) (Definition, error) {
	for _, metric := range ComputedMetrics {
		if metric.ID == id {
			return metric, nil
		}
	}
	return Definition{}, errors.Reason("no metric with ID %q", id.String()).Err()
}

// MustByID returns the metric with the given ID and panic if no metric with the id exists.
func MustByID(id ID) Definition {
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

	// Used to define the default sort order for metrics.
	// See Definition.SortPriority.
	SortPriority int

	// IsDefault indicates whether this metric is shown by default in the UI.
	IsDefault bool

	// A predicate on failures, that defines which failures are included in
	// the metric. See Definition.FilterSQL for more details.
	FilterSQL string

	// An expression that defines the distinct items to count.
	// See Definition.CountSQL for more details.
	CountSQL string
}

func (m metricBuilder) Build() Definition {
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
	if m.SortPriority <= 0 {
		panic("sort priority must be set to a positive integer")
	}
	return Definition{
		ID:                ID(m.ID),
		HumanReadableName: m.HumanReadableName,
		Description:       m.Description,
		SortPriority:      m.SortPriority,
		IsDefault:         m.IsDefault,
		Name:              fmt.Sprintf("metrics/%s", m.ID),
		BaseColumnName:    strings.ReplaceAll(m.ID, "-", "_"),
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

// Definition describes a metric.
type Definition struct {
	// ID is the identifier of the metric, for example "human-cls-failed-presubmit".
	// This is the same as the AIP-122 resource name of the metric,
	// excluding "metrics/".
	ID ID

	// The AIP-122 resource name of the metric, starting with "metrics/".
	// E.g. "metrics/human-cls-failed-presubmit"
	Name string

	// A human readable name for the metric. Appears on the user interface.
	HumanReadableName string

	// A human readable descripton of the metric. Appears on the user interface
	// behind a help tooltip.
	Description string

	// SortPriority is a number that defines the order by which metrics are
	// sorted by default. By default, the metric with the highest sort priority
	// will define the primary sort order, followed by the metric with the
	// second highest sort priority, and so on.
	SortPriority int

	// IsDefault indicates whether this metric is shown by default in the UI.
	IsDefault bool

	// BaseColumnName is the name of the metric to use in SQL column names.
	// It is the same as the identifier ID, but with hypens (-) replaced with
	// underscores.
	BaseColumnName string

	// The predicate on failures, that defines when an item is eligible to be
	// counted. Fields on the clustered_failures table may be accessed via the
	// prefix "f.".
	// For example, to count over failures on critical builds, use:
	// "f.build_critical".
	//
	// If no filtering is desired, this is left blank.
	FilterSQL string

	// An expression that defines the distinct items to count. Fields on the
	// clustered_failures table may be accessed via the prefix "f.".
	//
	// For example, to count distinct changelists, use:
	// `IF(ARRAY_LENGTH(f.changelists)>0,
	//	  CONCAT(f.changelists[OFFSET(0)].host, f.changelists[OFFSET(0)].change),
	// NULL)`
	// While this may return NULL for items not to be counted, it is generally
	// preferred to use FilterSQL for that purpose.
	//
	// If failures are to be counted instead of distinct items, this is left blank.
	CountSQL string
}

// ColumnName returns a column name to use for the metric in queries, with
// metric_ prefix used to namespace the column name to avoid name collisions
// with other predefined columns, and the specified suffix used to namespace
// the column name within other columns for the same metric.
func (m Definition) ColumnName(suffix string) string {
	return "metric_" + m.BaseColumnName + "_" + suffix
}
