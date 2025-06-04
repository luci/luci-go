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

package config

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.chromium.org/luci/common/errors"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/validate"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname/rules"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

const maxHysteresisPercent = 1000

var (
	// https://cloud.google.com/storage/docs/naming-buckets
	bucketRE             = regexp.MustCompile(`^[a-z0-9][a-z0-9\-_.]{1,220}[a-z0-9]$`)
	bucketMaxLengthBytes = 222

	// From https://source.chromium.org/chromium/infra/infra/+/main:appengine/monorail/project/project_constants.py;l=13.
	monorailProjectRE             = regexp.MustCompile(`^[a-z0-9][-a-z0-9]{0,61}[a-z0-9]$`)
	monorailProjectMaxLengthBytes = 63

	// https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/auth_service/proto/realms_config.proto;l=85;drc=04e290f764a293d642d287b0118e9880df4afb35
	realmRE             = regexp.MustCompile(`^[a-z0-9_\.\-/]{1,400}$`)
	realmMaxLengthBytes = 400

	// Matches valid prefixes to use when displaying bugs.
	// E.g. "crbug.com", "fxbug.dev".
	prefixRE             = regexp.MustCompile(`^[a-z0-9\-.]{0,64}$`)
	prefixMaxLengthBytes = 64

	// hostnameRE excludes most invalid hostnames.
	hostnameRE             = regexp.MustCompile(`^[a-z][a-z9-9\-.]{0,62}[a-z]$`)
	hostnameMaxLengthBytes = 64

	// policyIDRE matches valid bug management policy identifiers.
	policyIDRE             = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,62}[a-z0-9])?$`)
	policyIDMaxLengthBytes = 64

	// policyHumanReadableNameRE matches a valid bug management policy short descriptions.
	policyHumanReadableNameRE         = regexp.MustCompile("^[[:print:]]{1,100}$")
	policyHumanReadableMaxLengthBytes = 100

	// nameRE matches valid rule names.
	ruleNameRE             = regexp.MustCompile(`^[a-zA-Z0-9\-(), ]+$`)
	ruleNameMaxLengthBytes = 100

	// See RFC 3696 Part 3. The syntax below does not allow for spaces
	// or quoted local parts, which are technically allowed by the spec
	// but shouldn't be necessary here.
	ownerEmailRE             = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+-/=?^_`.{|}~]{1,64}@google\\.com$")
	ownerEmailMaxLengthBytes = 64 + len("@google.com")

	// printableASCIIRE matches any input consisting only of printable ASCII
	// characters.
	printableASCIIRE = regexp.MustCompile(`^[[:print:]]+$`)

	// Standard maximum lengths, in bytes. For fields, where there
	// is no obvious maximum length for the input type.
	longMaxLengthBytes     = 10000
	standardMaxLengthBytes = 100

	// Patterns for BigQuery table.
	// https://cloud.google.com/resource-manager/docs/creating-managing-projects
	cloudProjectRE             = regexp.MustCompile(`^[a-z][a-z0-9\-]{4,28}[a-z0-9]$`)
	cloudProjectMaxLengthBytes = 30

	// https://cloud.google.com/bigquery/docs/datasets#dataset-naming
	datasetRE             = regexp.MustCompile(`^[a-zA-Z0-9_]*$`)
	datasetMaxLengthBytes = 1024

	// https://cloud.google.com/bigquery/docs/tables#table_naming
	tableRE             = regexp.MustCompile(`^[\p{L}\p{M}\p{N}\p{Pc}\p{Pd}\p{Zs}]*$`)
	tableMaxLengthBytes = 1024

	// labelRE matches valid monorail labels. Note that label comparison
	// is case-insensitive. Could not find exact validation criteria
	// in monorail, so supporting a conservative subset of labels.
	monorailLabelRE             = regexp.MustCompile(`^[a-zA-Z0-9\-]+$`)
	monorailLabelMaxLengthBytes = 60

	unspecifiedMessage = "must be specified"
)

func validateConfig(ctx *validation.Context, cfg *configpb.Config) {
	validateStringConfig(ctx, "monorail_hostname", cfg.MonorailHostname, hostnameRE, hostnameMaxLengthBytes)
	validateStringConfig(ctx, "chunk_gcs_bucket", cfg.ChunkGcsBucket, bucketRE, bucketMaxLengthBytes)
	// Limit to default max_concurrent_requests of 1000.
	// https://cloud.google.com/appengine/docs/standard/go111/config/queueref
	validateIntegerConfig(ctx, "reclustering_workers", cfg.ReclusteringWorkers, 1, 1000)
}

func validateStringConfig(ctx *validation.Context, name, cfg string, re *regexp.Regexp, maxLengthBytes int) {
	ctx.Enter(name)
	defer ctx.Exit()
	if len(cfg) > maxLengthBytes {
		ctx.Errorf("exceeds maximum allowed length of %v bytes", maxLengthBytes)
		return
	}
	if err := validate.SpecifiedWithRe(re, cfg); err != nil {
		ctx.Error(err)
		return
	}
}

// validateIntegerConfig validates that an integer field is within the
// range [minInclusive, maxInclusive].
func validateIntegerConfig(ctx *validation.Context, name string, cfg, minInclusive, maxInclusive int64) {
	ctx.Enter(name)
	defer ctx.Exit()

	if cfg < minInclusive || cfg > maxInclusive {
		if cfg == 0 {
			ctx.Errorf(unspecifiedMessage)
		} else {
			ctx.Errorf("must be in the range [%v, %v]", minInclusive, maxInclusive)
		}
	}
}

// validateFloat64Config validates that a float64 field is within the
// range [minInclusive, maxInclusive].
func validateFloat64Config(ctx *validation.Context, name string, cfg, minInclusive, maxInclusive float64) {
	ctx.Enter(name)
	defer ctx.Exit()

	if math.IsInf(cfg, 0) || math.IsNaN(cfg) {
		ctx.Errorf("must be a finite number")
		return
	}
	if cfg == 0.0 && (cfg < minInclusive || cfg > maxInclusive) {
		ctx.Errorf(unspecifiedMessage)
		return
	}
	if cfg < minInclusive || cfg > maxInclusive {
		ctx.Errorf("must be in the range [%f, %f]", minInclusive, maxInclusive)
	}
}

// validateProjectConfigRaw deserializes the project-level config message
// and passes it through the validator.
func validateProjectConfigRaw(ctx *validation.Context, project, content string) *configpb.ProjectConfig {
	msg := &configpb.ProjectConfig{}
	if err := luciproto.UnmarshalTextML(content, msg); err != nil {
		ctx.Errorf("failed to unmarshal as text proto: %s", err)
		return nil
	}
	ValidateProjectConfig(ctx, project, msg)
	return msg
}

func ValidateProjectConfig(ctx *validation.Context, project string, cfg *configpb.ProjectConfig) {
	validateClustering(ctx, cfg.Clustering)
	validateMetrics(ctx, cfg.Metrics)
	validateBugManagement(ctx, cfg.BugManagement)
	validateTestStabilityCriteria(ctx, cfg.TestStabilityCriteria)
}

func validateBuganizerDefaultComponent(ctx *validation.Context, component *configpb.BuganizerComponent) {
	ctx.Enter("default_component")
	defer ctx.Exit()

	if component == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	ctx.Enter("id")
	defer ctx.Exit()

	if component.Id == 0 {
		ctx.Errorf(unspecifiedMessage)
	} else if component.Id < 0 {
		ctx.Errorf("must be positive")
	}
}

func validateBuganizerPriority(ctx *validation.Context, priority configpb.BuganizerPriority) {
	ctx.Enter("priority")
	defer ctx.Exit()

	if priority == configpb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED {
		ctx.Errorf(unspecifiedMessage)
		return
	}
}

func validateDefaultFieldValues(ctx *validation.Context, fvs []*configpb.MonorailFieldValue) {
	ctx.Enter("default_field_values")
	defer ctx.Exit()

	if len(fvs) > 50 {
		ctx.Errorf("at most 50 field values may be specified")
		return
	}
	for i, fv := range fvs {
		validateFieldValue(ctx, fmt.Sprintf("[%v]", i), fv)
	}
}

func validateFieldValue(ctx *validation.Context, name string, fv *configpb.MonorailFieldValue) {
	ctx.Enter(name)
	defer ctx.Exit()

	if fv == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	validateFieldID(ctx, "field_id", fv.FieldId)
	validateStringConfig(ctx, "value", fv.Value, printableASCIIRE, standardMaxLengthBytes)
}

func validateFieldID(ctx *validation.Context, fieldName string, fieldID int64) {
	ctx.Enter(fieldName)
	defer ctx.Exit()

	if fieldID == 0 {
		ctx.Errorf(unspecifiedMessage)
	} else if fieldID < 0 {
		ctx.Errorf("must be positive")
	}
}

// validateMetricThreshold a metric threshold message. If mustBeSatisfiable is set,
// the metric threshold must have at least one of one_day, three_day or seven_day set.
func validateMetricThreshold(ctx *validation.Context, fieldName string, t *configpb.MetricThreshold, mustBeSatisfiable bool) {
	ctx.Enter(fieldName)
	defer ctx.Exit()

	if t == nil {
		if mustBeSatisfiable {
			// To be satisfiable, a threshold must be set.
			ctx.Errorf(unspecifiedMessage)
		}
		// Not specified.
		return
	}

	if mustBeSatisfiable && (t.OneDay == nil && t.ThreeDay == nil && t.SevenDay == nil) {
		// To be satisfiable, a threshold must be set.
		ctx.Errorf("at least one of one_day, three_day and seven_day must be set")
	}

	validateThresholdValue(ctx, t.OneDay, "one_day")
	validateThresholdValue(ctx, t.ThreeDay, "three_day")
	validateThresholdValue(ctx, t.SevenDay, "seven_day")
}

func validateThresholdValue(ctx *validation.Context, value *int64, fieldName string) {
	ctx.Enter(fieldName)
	defer ctx.Exit()

	if value != nil && *value <= 0 {
		ctx.Errorf("value must be positive")
	}
	if value != nil && *value >= 1000*1000 {
		ctx.Errorf("value must be less than one million")
	}
}

type validateImplicationOptions struct {
	// A human-readable description of the LHS threshold, e.g.
	// "the bug-filing threshold", for use in error messages.
	lhsDescription string
	// A human-readable statement motivating the threshold implication
	// check, for use in error messages. E.g.
	// "this ensures that bugs which are filed meet the criteria to stay open".
	implicationDescription string
}

// validateMetricThresholdImpliedBy verifies the threshold `lhs` implies
// the threshold `rhs` is satisfied, i.e. lhs => rhs.
// fieldName is the name of the field that contains the `rhs` threshold.
func validateMetricThresholdImpliedBy(ctx *validation.Context, rhsFieldName string, rhs *configpb.MetricThreshold, lhs *configpb.MetricThreshold, opts validateImplicationOptions) {
	ctx.Enter(rhsFieldName)
	defer ctx.Exit()

	if rhs == nil {
		rhs = &configpb.MetricThreshold{}
	}
	if lhs == nil {
		// Bugs are not filed based on this metric. So
		// we do not need to check that bugs filed
		// based on this metric will stay open.
		return
	}

	// Thresholds are met if ANY of the 1, 3 or 7-day thresholds are met
	// (i.e. semantically they are an 'OR' of the day sub-thresholds).
	//
	// This means checking lhs => rhs actually means checking:
	// value(1-day) >= lhs(1-day) OR value(3-day) >= lhs(3-day) OR value(7-day) >= lhs(7-day) =>
	//     value(1-day) >= rhs(1-day) OR value(3-day) >= rhs(3-day) OR value(7-day) >= rhs(7-day)
	// (equation (1)).
	//
	// Where:
	// - value(X-day) is the time-changing variable identifying the metric calculated on X days of data,
	// - lhs(X-day) is the LHS's X-day threshold,
	// - rhs(X-day) is the RHS's X-day threshold.
	// Where lhs or rhs do not have a threshold set for a given day, e.g. lhs.OneDay = nil,
	// this can be taken instead as infinity being the threshold (i.e. lhs(1-day) = infinity).
	//
	// If lhs(1-day) >= rhs(1-day) AND lhs(3-day) >= rhs(3-day) AND lhs(7-day) >= rhs(7-day),
	// i.e. all LHS are strictly stronger than the corresponding RHS thresholds, `lhs => rhs`
	// is trivially shown. However, this is not the only case where `lhs => rhs` can be
	// shown. For example, consider if the LHS is:
	// - User CLs Failed Presubmit (1-day) >= 10
	// and the RHS is:
	// - User CLs Failed Presubmit (3-day) >= 5
	//
	// The LHS still implies the RHS is true. This is because User CLs Failed Presubmit (3-day)
	// >= User CLs Failed Presubmit (1-day). To derive more precise conditions for testing if
	// `lhs => rhs`, we follow a systematic approach, explained below.
	//
	// To show equation (1) above is true, we must show:
	//   value(X-day) >= lhs(X-day)
	//        =>
	//   value(1-day) >= rhs(1-day) OR value(3-day) >= rhs(3-day) OR value(7-day) >= rhs(7-day)
	// for ALL of X = 1, 3, 7 (equation (2)).
	//
	// Let us consider the case for X = 3 days.
	//
	// From the LHS of the implication in equation (2) we are given that `value(3-day) >= lhs(3-day)` (context 1).
	// We must show one of `value(1-day) >= rhs(1-day)` OR `value(3-day) >= rhs(3-day)` OR `value(7-day) >= rhs(7-day)`.
	//
	// - We cannot show value(1-day) >= rhs(1-day) as we only have a bound on the value of value(3-day),
	//   so we proceed to the second 'OR' case.
	// - We can show value(3-day) >= rhs(3-day) if lhs(3-day) >= rhs(3-day).
	//   This follows as we have:
	//    - value(3-day) >= lhs(3-day)    // (from context 1)
	//    - lhs(3-day) >= rhs(3-day)      // IF condition
	//   Alternatively,
	// - We also can show value(7-day) >= rhs(7-day) is true if lhs(3-day) >= rhs(7-day),
	//   This follows as we have:
	//    - value(7-day) >= value(3-day)  // property of the metric values
	//    - value(3-day) >= lhs(3-day)    // (context 1)
	//    - lhs(3-day)   >= rhs(7-day)    // IF condition
	//
	// In summary, to prove implication for the case of X = 3 days, we need
	// lhs(3-day) >= rhs(3-day) OR lhs(3-day) >= rhs(7-day).
	//
	// The same pattern applies to X = 1 days and X = 7 days. The criteria is:
	// For X = 1, lhs(1-day) >= rhs(1-day) OR lhs(1-day) >= rhs(3-day) OR lhs(1-day) >= rhs(7-day).
	// For X = 7, lhs(7-day) >= rhs(7-day).
	//
	// Note that for X = 1, this is the same as
	// `lhs(1-day) >= min(rhs(1-day), rhs(3-day), rhs(7-day))`.
	//
	// Using this simplification, we get we must check:
	// - lhs(1-day) >= min(rhs(1-day), rhs(3-day), rhs(7-day)) AND
	// - lhs(3-day) >= min(rhs(3-day), rhs(7-day)) AND
	// - lhs(7-day) >= rhs(7-day)
	// To show LHS => RHS.

	oneDayRhsThreshold := minOfThresholds(rhs.OneDay, rhs.ThreeDay, rhs.SevenDay)
	threeDayRhsThreshold := minOfThresholds(rhs.ThreeDay, rhs.SevenDay)

	// Attribute the failure of the 1-day criteria to rhs(1-day), even
	// if rhs(3-day) or rhs(7-day) can fix it, as this is more understandable
	// for users.
	validateBugFilingThresholdSatisfiesThresold(ctx, oneDayRhsThreshold, lhs.OneDay, "one_day", opts.lhsDescription, opts.implicationDescription)
	// Attribute the failure of the 3-day criteria to rhs(3-day), even
	// if rhs(7-day) can fix it, as this is more understandable
	// for users.
	validateBugFilingThresholdSatisfiesThresold(ctx, threeDayRhsThreshold, lhs.ThreeDay, "three_day", opts.lhsDescription, opts.implicationDescription)
	validateBugFilingThresholdSatisfiesThresold(ctx, rhs.SevenDay, lhs.SevenDay, "seven_day", opts.lhsDescription, opts.implicationDescription)
}

func minOfThresholds(thresholds ...*int64) *int64 {
	var result *int64
	for _, t := range thresholds {
		if t != nil && (result == nil || *t < *result) {
			result = t
		}
	}
	return result
}

func validateBugFilingThresholdSatisfiesThresold(ctx *validation.Context, rhsThreshold *int64, lhsThres *int64, fieldName, lhsDescription, implicationDescription string) {
	ctx.Enter(fieldName)
	defer ctx.Exit()

	if lhsThres == nil {
		// Bugs are not filed based on this threshold.
		return
	}
	if *lhsThres <= 0 {
		// The bug-filing threshold is invalid. This is already reported as an
		// error elsewhere.
		return
	}
	if rhsThreshold == nil {
		ctx.Errorf("%s threshold must be set, with a value of at most %v (%s); %s", fieldName, *lhsThres, lhsDescription, implicationDescription)
	} else if *rhsThreshold > *lhsThres {
		ctx.Errorf("value must be at most %v (%s); %s", *lhsThres, lhsDescription, implicationDescription)
	}
}

func validateClustering(ctx *validation.Context, ca *configpb.Clustering) {
	ctx.Enter("clustering")
	defer ctx.Exit()

	if ca == nil {
		return
	}
	ctx.Enter("test_name_rules")
	for i, r := range ca.TestNameRules {
		ctx.Enter("[%v]", i)
		validateTestNameRule(ctx, r)
		ctx.Exit()
	}
	ctx.Exit()
	ctx.Enter("reason_mask_patterns")
	for i, p := range ca.ReasonMaskPatterns {
		ctx.Enter("[%v]", i)
		validateReasonMaskPattern(ctx, p)
		ctx.Exit()
	}
	ctx.Exit()
}

func validateTestNameRule(ctx *validation.Context, r *configpb.TestNameClusteringRule) {
	validateStringConfig(ctx, "name", r.Name, ruleNameRE, ruleNameMaxLengthBytes)

	// Check the fields are non-empty. Their structure will be checked
	// by "Compile" below.
	validateStringConfig(ctx, "like_template", r.LikeTemplate, printableASCIIRE, 1024)
	validateStringConfig(ctx, "pattern", r.Pattern, printableASCIIRE, 1024)

	_, err := rules.Compile(r)
	if err != nil {
		ctx.Error(err)
	}
}

func validateReasonMaskPattern(ctx *validation.Context, p string) {
	if p == "" {
		ctx.Errorf("empty pattern is not allowed")
	}
	re, err := regexp.Compile(p)
	if err != nil {
		ctx.Errorf("could not compile pattern: %s", err)
	} else {
		if re.NumSubexp() != 1 {
			ctx.Errorf("pattern must contain exactly one parenthesised capturing subexpression indicating the text to mask")
		}
	}
}

func validateMetrics(ctx *validation.Context, m *configpb.Metrics) {
	ctx.Enter("metrics")
	defer ctx.Exit()

	if m == nil {
		// Allow non-existent metrics section.
		return
	}
	seenIDs := map[string]struct{}{}
	for i, o := range m.Overrides {
		ctx.Enter("[%v]", i)
		validateMetricOverride(ctx, o, seenIDs)
		ctx.Exit()
	}
}

func validateMetricOverride(ctx *validation.Context, o *configpb.Metrics_MetricOverride, seenIDs map[string]struct{}) {
	validateMetricID(ctx, o.MetricId, seenIDs)
	if o.SortPriority != nil {
		validateSortPriority(ctx, int64(*o.SortPriority))
	}
}

func validateMetricID(ctx *validation.Context, metricID string, seenIDs map[string]struct{}) {
	ctx.Enter("metric_id")
	defer ctx.Exit()

	if _, err := metrics.ByID(metrics.ID(metricID)); err != nil {
		ctx.Error(err)
	} else {
		if _, ok := seenIDs[metricID]; ok {
			ctx.Errorf("metric with ID %q appears in collection more than once", metricID)
		}
		seenIDs[metricID] = struct{}{}
	}
}

func validateSortPriority(ctx *validation.Context, value int64) {
	ctx.Enter("sort_priority")
	defer ctx.Exit()

	if value <= 0 {
		ctx.Errorf("value must be positive")
	}
}

func validateBugManagement(ctx *validation.Context, bm *configpb.BugManagement) {
	ctx.Enter("bug_management")
	defer ctx.Exit()

	if bm == nil {
		// Allow non-existent bug managment section.
		return
	}

	validateBugManagementPolicies(ctx, bm.Policies)

	if bm.DefaultBugSystem == configpb.BugSystem_MONORAIL && bm.Monorail == nil {
		ctx.Errorf("monorail section is required when the default_bug_system is Monorail")
		return
	}
	if bm.DefaultBugSystem == configpb.BugSystem_BUGANIZER && bm.Buganizer == nil {
		ctx.Errorf("buganizer section is required when the default_bug_system is Buganizer")
		return
	}
	if bm.Buganizer != nil || bm.Monorail != nil {
		// Default bug system must be specified if either Buganizer or Monorail is configured.
		validateDefaultBugSystem(ctx, bm.DefaultBugSystem)
	}
	validateBuganizer(ctx, bm.Buganizer)
	validateMonorail(ctx, bm.Monorail)
}

func validateDefaultBugSystem(ctx *validation.Context, value configpb.BugSystem) {
	ctx.Enter("default_bug_system")
	defer ctx.Exit()
	if value == configpb.BugSystem_BUG_SYSTEM_UNSPECIFIED {
		ctx.Errorf(unspecifiedMessage)
	}
}

func validateBuganizer(ctx *validation.Context, cfg *configpb.BuganizerProject) {
	ctx.Enter("buganizer")
	defer ctx.Exit()

	if cfg == nil {
		// Allow non-existent buganizer section.
		return
	}
	validateBuganizerDefaultComponent(ctx, cfg.DefaultComponent)
}

func validateMonorail(ctx *validation.Context, cfg *configpb.MonorailProject) {
	ctx.Enter("monorail")
	defer ctx.Exit()

	if cfg == nil {
		// Allow non-existent monorail section.
		return
	}

	validateStringConfig(ctx, "project", cfg.Project, monorailProjectRE, monorailProjectMaxLengthBytes)
	validateDefaultFieldValues(ctx, cfg.DefaultFieldValues)
	validateFieldID(ctx, "priority_field_id", cfg.PriorityFieldId)
	validateStringConfig(ctx, "display_prefix", cfg.DisplayPrefix, prefixRE, prefixMaxLengthBytes)
	validateStringConfig(ctx, "monorail_hostname", cfg.MonorailHostname, hostnameRE, hostnameMaxLengthBytes)
}

func validateBugManagementPolicies(ctx *validation.Context, policies []*configpb.BugManagementPolicy) {
	ctx.Enter("policies")
	defer ctx.Exit()

	if len(policies) > 50 {
		ctx.Errorf("exceeds maximum of 50 policies")
		return
	}

	policyIDs := make(map[string]struct{})
	for i, policy := range policies {
		validateBugManagementPolicy(ctx, fmt.Sprintf("[%v]", i), policy, policyIDs)
	}
}

func validateBugManagementPolicy(ctx *validation.Context, name string, p *configpb.BugManagementPolicy, seenIDs map[string]struct{}) {
	ctx.Enter(name)
	defer ctx.Exit()

	if p == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	validateBugManagementPolicyID(ctx, p.Id, seenIDs)
	validateBugManagementPolicyOwners(ctx, p.Owners)
	validateStringConfig(ctx, "human_readable_name", p.HumanReadableName, policyHumanReadableNameRE, policyHumanReadableMaxLengthBytes)
	validateBuganizerPriority(ctx, p.Priority)
	validateBugManagementPolicyMetrics(ctx, p.Metrics)
	validateBugManagementPolicyExplanation(ctx, p.Explanation)
	validateBugManagementPolicyBugTemplate(ctx, p.BugTemplate)
}

func validateBugManagementPolicyOwners(ctx *validation.Context, owners []string) {
	ctx.Enter("owners")
	defer ctx.Exit()

	if len(owners) == 0 {
		ctx.Errorf("at least one owner must be specified")
		return
	}
	if len(owners) > 10 {
		ctx.Errorf("exceeds maximum of 10 owners")
		return
	}
	for i, owner := range owners {
		validateStringConfig(ctx, fmt.Sprintf("[%v]", i), owner, ownerEmailRE, ownerEmailMaxLengthBytes)
	}
}

func validateBugManagementPolicyMetrics(ctx *validation.Context, metrics []*configpb.BugManagementPolicy_Metric) {
	ctx.Enter("metrics")
	defer ctx.Exit()

	if len(metrics) == 0 {
		ctx.Errorf("at least one metric must be specified")
		return
	}
	if len(metrics) > 10 {
		ctx.Errorf("exceeds maximum of 10 metrics")
		return
	}
	metricIDs := make(map[string]struct{})
	for i, metric := range metrics {
		validateBugManagementPolicyMetric(ctx, fmt.Sprintf("[%v]", i), metric, metricIDs)
	}
}

func validateBugManagementPolicyID(ctx *validation.Context, id string, seenIDs map[string]struct{}) {
	ctx.Enter("id")
	defer ctx.Exit()

	if len(id) > policyIDMaxLengthBytes {
		ctx.Errorf("exceeds maximum allowed length of %v bytes", policyIDMaxLengthBytes)
		return
	}
	if err := validate.SpecifiedWithRe(policyIDRE, id); err != nil {
		ctx.Error(err)
		return
	}
	if _, ok := seenIDs[id]; ok {
		ctx.Errorf("policy with ID %q appears in the collection more than once", id)
	}
	seenIDs[id] = struct{}{}
}

func validateBugManagementPolicyMetric(ctx *validation.Context, name string, m *configpb.BugManagementPolicy_Metric, seenIDs map[string]struct{}) {
	ctx.Enter(name)
	defer ctx.Exit()

	validateMetricID(ctx, m.MetricId, seenIDs)

	// It is permissible for policies to not have an activation threshold. In
	// this case the policy will not activate on new rules, but existing activations
	// can de-activate.
	mustBeSatifiable := false
	validateMetricThreshold(ctx, "activation_threshold", m.ActivationThreshold, mustBeSatifiable)

	// All policies must have a de-activation threshold, to allow bugs to
	// eventually close.
	mustBeSatifiable = true
	validateMetricThreshold(ctx, "deactivation_threshold", m.DeactivationThreshold, mustBeSatifiable)

	// Verify that if the activation_threshold is met, the deactivation_threshold is also met.
	opts := validateImplicationOptions{
		lhsDescription:         "the activation threshold",
		implicationDescription: "this ensures policies which activate do not immediately de-activate",
	}
	validateMetricThresholdImpliedBy(ctx, "deactivation_threshold", m.DeactivationThreshold, m.ActivationThreshold, opts)
}

func validateBugManagementPolicyExplanation(ctx *validation.Context, e *configpb.BugManagementPolicy_Explanation) {
	ctx.Enter("explanation")
	defer ctx.Exit()

	if e == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	if err := ValidateRunesGraphicOrNewline(e.ProblemHtml, longMaxLengthBytes); err != nil {
		ctx.Enter("problem_html")
		ctx.Error(err)
		ctx.Exit()
	}
	if err := ValidateRunesGraphicOrNewline(e.ActionHtml, longMaxLengthBytes); err != nil {
		ctx.Enter("action_html")
		ctx.Error(err)
		ctx.Exit()
	}
}

func validateBugManagementPolicyBugTemplate(ctx *validation.Context, t *configpb.BugManagementPolicy_BugTemplate) {
	ctx.Enter("bug_template")
	defer ctx.Exit()

	if t == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	validateCommentTemplate(ctx, t.CommentTemplate)
	validateBugManagementPolicyBugTemplateBuganizer(ctx, t.Buganizer)
	validateBugManagementPolicyBugTemplateMonorail(ctx, t.Monorail)
}

func validateCommentTemplate(ctx *validation.Context, t string) {
	ctx.Enter("comment_template")
	defer ctx.Exit()

	if t == "" {
		// It is acceptable for a policy not to comment on a bug.
		return
	}
	if err := ValidateRunesGraphicOrNewline(t, longMaxLengthBytes); err != nil {
		ctx.Error(err)
		return
	}

	tmpl, err := bugs.ParseTemplate(t)
	if err != nil {
		ctx.Errorf("parsing template: %s", err)
		return
	}
	if err := tmpl.Validate(); err != nil {
		ctx.Errorf("validate template: %s", err)
	}
}

// ValidateRunesGraphicOrNewline validates a value:
// - is non-empty
// - is a valid UTF-8 string
// - contains only runes matching unicode.IsGraphic or '\n', and
// - has a specified maximum length.
func ValidateRunesGraphicOrNewline(value string, maxLengthInBytes int) error {
	if value == "" {
		return errors.New(unspecifiedMessage)
	}
	if len(value) > maxLengthInBytes {
		return errors.Fmt("exceeds maximum allowed length of %v bytes", maxLengthInBytes)
	}
	if !utf8.ValidString(value) {
		return errors.New("not a valid UTF-8 string")
	}
	for i, r := range value {
		if !unicode.IsGraphic(r) && r != rune('\n') {
			return errors.Fmt("unicode rune %q at index %v is not graphic or newline character", r, i)
		}
	}
	return nil
}

func validateBugManagementPolicyBugTemplateBuganizer(ctx *validation.Context, b *configpb.BugManagementPolicy_BugTemplate_Buganizer) {
	ctx.Enter("buganizer")
	defer ctx.Exit()

	if b == nil {
		// It is valid not specify buganizer-specific template options.
		return
	}

	validateBuganizerHotlists(ctx, b.Hotlists)
}

func validateBuganizerHotlists(ctx *validation.Context, hotlists []int64) {
	ctx.Enter("hotlists")
	defer ctx.Exit()
	if len(hotlists) > 5 {
		ctx.Errorf("exceeds maximum of 5 hotlists")
	}
	seenIDs := map[int64]struct{}{}
	for i, hotlist := range hotlists {
		ctx.Enter("[%v]", i)
		if hotlist <= 0 {
			ctx.Errorf("ID must be positive")
		} else {
			if _, ok := seenIDs[hotlist]; ok {
				ctx.Errorf("ID %v appears in collection more than once", hotlist)
			}
			seenIDs[hotlist] = struct{}{}
		}
		ctx.Exit()
	}
}

func validateBugManagementPolicyBugTemplateMonorail(ctx *validation.Context, m *configpb.BugManagementPolicy_BugTemplate_Monorail) {
	ctx.Enter("monorail")
	defer ctx.Exit()

	if m == nil {
		// It is valid not specify monorail-specific template options.
		return
	}

	validateMonorailLabels(ctx, m.Labels)
}

func validateMonorailLabels(ctx *validation.Context, labels []string) {
	ctx.Enter("labels")
	defer ctx.Exit()
	if len(labels) > 5 {
		ctx.Errorf("exceeds maximum of 5 labels")
	}
	seenLabels := map[string]struct{}{}
	for i, label := range labels {
		validateStringConfig(ctx, fmt.Sprintf("[%v]", i), label, monorailLabelRE, monorailLabelMaxLengthBytes)

		// Check for duplicates, case-insensitively (as monorail handles
		// labels in a case-insensitive way).
		if _, ok := seenLabels[strings.ToLower(label)]; ok {
			ctx.Enter("[%v]", i)
			ctx.Errorf("label %q appears in collection more than once", strings.ToLower(label))
			ctx.Exit()
		}
		seenLabels[label] = struct{}{}
	}
}

func validateTestStabilityCriteria(ctx *validation.Context, t *configpb.TestStabilityCriteria) {
	ctx.Enter("test_stability_criteria")
	defer ctx.Exit()

	if t == nil {
		// It is valid not to specify test stability criteria.
		return
	}

	validateFailureRateCriteria(ctx, t.FailureRate)
	validateFlakeRateCriteria(ctx, t.FlakeRate)
}

func validateFailureRateCriteria(ctx *validation.Context, f *configpb.TestStabilityCriteria_FailureRateCriteria) {
	ctx.Enter("failure_rate")
	defer ctx.Exit()

	if f == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}
	validateIntegerConfig(ctx, "failure_threshold", int64(f.FailureThreshold), 1, 10)
	validateIntegerConfig(ctx, "consecutive_failure_threshold", int64(f.ConsecutiveFailureThreshold), 1, 10)
}

func validateFlakeRateCriteria(ctx *validation.Context, f *configpb.TestStabilityCriteria_FlakeRateCriteria) {
	ctx.Enter("flake_rate")
	defer ctx.Exit()

	if f == nil {
		ctx.Errorf(unspecifiedMessage)
		return
	}

	// Window sizes on the order of 100-10,000 source verdicts are expected in normal operation.
	// 1,000,000 should be far larger than anything we ever need to use.
	const largeValue = 1_000_000
	validateIntegerConfig(ctx, "min_window", int64(f.MinWindow), 0, largeValue)
	validateIntegerConfig(ctx, "flake_threshold", int64(f.FlakeThreshold), 1, largeValue)
	validateFloat64Config(ctx, "flake_rate_threshold", f.FlakeRateThreshold, 0.0, 1.0)
}
