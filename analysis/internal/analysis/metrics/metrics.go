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
)

// metricIDRE defines the valid format for metric identifiers.
// Metric identifiers must be valid google.aip.dev/122 resource ID segments.
var metricIDRE = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Standard metrics.
var (
	// The number of distinct developer changelists that failed at least one
	// presubmit (CQ) run because of failure(s) in this cluster. Excludes
	// changelists authored by automation.
	HumanClsFailedPresubmit = metricBuilder{
		ID: "human-cls-failed-presubmit",
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
		ID: "critical-failures-exonerated",
		// Exonerated for a reason other than NOT_CRITICAL or UNEXPECTED_PASS.
		// Passes are not ingested by LUCI Analysis, but if a test has both an unexpected pass
		// and an unexpected failure, it will be exonerated for the unexpected pass.
		// TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
		FilterSQL: `f.build_critical AND (EXISTS (SELECT TRUE FROM UNNEST(f.exonerations) e WHERE e.Reason = 'OCCURS_ON_OTHER_CLS'))`,
	}.Build()

	// The number of test runs that failed. Test runs are generally
	// equivalent to swarming tasks.
	TestRunsFailed = metricBuilder{
		ID:        "test-runs-failed",
		FilterSQL: `f.is_test_run_blocked`,
		CountSQL:  `f.test_run_id`,
	}.Build()

	// The total number of test results in this cluster. LUCI Analysis only
	// clusters test results which are unexpected and have a status of crash,
	// abort or fail, so by definition the only test results counted here
	// will be an unexpected fail/crash/abort.
	Failures = metricBuilder{
		ID: "failures",
	}.Build()

	// ComputedMetrics is the set of metrics computed for each cluster and
	// stored on the cluster summaries table.
	ComputedMetrics = []Definition{HumanClsFailedPresubmit, CriticalFailuresExonerated, TestRunsFailed, Failures}

	// DefaultMetrics is the list of all standard (system-defined) metrics
	// shown on UI surfaces.
	DefaultMetrics = []Definition{HumanClsFailedPresubmit, CriticalFailuresExonerated, Failures}
)

// metricBuilder provides a way of building a new metric definition.
type metricBuilder struct {
	// ID is the identifier of the metric. It must be a valid AIP-122
	// Resource ID Segment (matching `^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`),
	// as it will be prefixed by "metrics/" to become the metric's
	// resource name.
	ID string

	// The predicate on failures, that defines when an item is eligible to be
	// counted. Fields on the clustered_failures table may be accessed via the
	// prefix "f.".
	// For example, to count over failures on critical builds, use:
	// "f.build_critical".
	// If no filtering is desired, leave this blank.
	FilterSQL string

	// An expression that defines the distinct items to count. Fields on the
	// clustered_failures table may be accessed via the prefix "f.".
	//
	// For example, to count distinct changelists, use:
	// `IF(ARRAY_LENGTH(f.changelists)>0,
	//	  CONCAT(f.changelists[OFFSET(0)].host, f.changelists[OFFSET(0)].change),
	// NULL)`
	//
	// While this may return NULL for items not to be counted, it is generally
	// preferred to use FilterSQL for that purpose.
	//
	// If failures are to be counted instead of distinct items, leave this blank.
	CountSQL string
}

func (m metricBuilder) Build() Definition {
	if !metricIDRE.MatchString(m.ID) {
		panic(fmt.Sprintf("metric ID %q does not match expected pattern", m.ID))
	}
	return Definition{
		ID:             ID(m.ID),
		Name:           fmt.Sprintf("metrics/%s", m.ID),
		BaseColumnName: strings.ReplaceAll(m.ID, "-", "_"),
		FilterSQL:      m.FilterSQL,
		CountSQL:       m.CountSQL,
	}
}

// ID is an identifier of a metric. For example, "human-cls-failed-presubmit".
// It should be a valid AIP-122 Resource ID Segment.
type ID string

// Definition describes a metric.
type Definition struct {
	// ID is the identifier of the metric, for example "human-cls-failed-presubmit".
	// This is the same as the AIP-122 resource name of the metric,
	// excluding "metrics/".
	ID ID

	// The AIP-122 resource name of the metric, starting with "metrics/".
	// E.g. "metrics/human-cls-failed-presubmit"
	Name string

	// BaseColumnName is the name of the metric to use in SQL column names.
	// It is the same as the identifier ID, but with hypens (-) replaced with
	// underscores.
	BaseColumnName string

	// A predicate on failures, that defines which failures are included in
	// the metric. See MetricSpec.FilterSQL for more details.
	FilterSQL string

	// An expression that defines the distinct items to count.
	// See MetricSpec.CountSQL for more details.
	CountSQL string
}

// ColumnName returns a column name to use for the metric in queries, with
// metric_ prefix used to namespace the column name to avoid name collisions
// with other predefined columns, and the specified suffix used to namespace
// the column name within other columns for the same metric.
func (m Definition) ColumnName(suffix string) string {
	return "metric_" + m.BaseColumnName + "_" + suffix
}
