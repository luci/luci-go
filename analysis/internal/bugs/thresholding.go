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

package bugs

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// InflateThreshold inflates or deflates impact thresholds by the given factor.
// This method is provided to help implement hysteresis. inflationPercent can
// be positive or negative (or zero), and is interpreted as follows:
// - If inflationPercent is positive, the new threshold is (threshold * (1 + (inflationPercent/100)))
// - If inflationPercent is negative, the new threshold used is (threshold / (1 + (-inflationPercent/100))
// i.e. inflationPercent of +100 would result in a threshold that is 200% the
// original threshold being used, inflationPercent of -100 would result in a
// threshold that is 50% of the original.
// To avoid unintended effects such as 1 being inflated down to 0, inflation
// can never make a zero number non-zero or a non-zero number zero.
func InflateThreshold(t []*configpb.ImpactMetricThreshold, inflationPercent int64) []*configpb.ImpactMetricThreshold {
	inflated := make([]*configpb.ImpactMetricThreshold, 0, len(t))
	for _, m := range t {
		inflated = append(inflated, &configpb.ImpactMetricThreshold{
			MetricId:  m.MetricId,
			Threshold: inflateMetricThreshold(m.Threshold, inflationPercent),
		})
	}
	return inflated
}

func inflateMetricThreshold(t *configpb.MetricThreshold, inflationPercent int64) *configpb.MetricThreshold {
	if t == nil {
		// No thresholds specified for metric.
		return nil
	}
	return &configpb.MetricThreshold{
		OneDay:   inflateSingleThreshold(t.OneDay, inflationPercent),
		ThreeDay: inflateSingleThreshold(t.ThreeDay, inflationPercent),
		SevenDay: inflateSingleThreshold(t.SevenDay, inflationPercent),
	}
}

func inflateSingleThreshold(threshold *int64, inflationPercent int64) *int64 {
	if threshold == nil {
		// No threshold was specified.
		return nil
	}
	thresholdValue := *threshold
	if thresholdValue == 0 {
		// Explicitly never change a zero value.
		return &thresholdValue
	}
	if inflationPercent >= 0 {
		// I.E. +100% doubles the threshold.
		thresholdValue = (thresholdValue * (100 + inflationPercent)) / 100
	} else {
		// I.E. -100% halves the threshold.
		thresholdValue = (thresholdValue * 100) / (100 + -inflationPercent)
	}
	// If the result is zero, set it to 1 instead to avoid things like
	// bug closing thresholds being rounded down to zero failures, and thus
	// bugs never being closed.
	if thresholdValue == 0 {
		thresholdValue = 1
	}
	return &thresholdValue
}

// MeetsAnyOfThresholds returns whether the cluster metrics meet or exceed
// any of the specified thresholds.
func (c *ClusterMetrics) MeetsAnyOfThresholds(ts []*configpb.ImpactMetricThreshold) bool {
	for _, t := range ts {
		if c.MeetsThreshold(metrics.ID(t.MetricId), t.Threshold) {
			return true
		}
	}
	return false
}

// MeetsThreshold returns whether the cluster metrics meet or exceed
// the specified threshold.
func (c *ClusterMetrics) MeetsThreshold(metricID metrics.ID, t *configpb.MetricThreshold) bool {
	impact, ok := (*c)[metricID]
	if ok && impact.meetsThreshold(t) {
		return true
	}
	return false
}

func (m MetricValues) meetsThreshold(t *configpb.MetricThreshold) bool {
	if t == nil {
		t = &configpb.MetricThreshold{}
	}
	if meetsThreshold(m.OneDay, t.OneDay) {
		return true
	}
	if meetsThreshold(m.ThreeDay, t.ThreeDay) {
		return true
	}
	if meetsThreshold(m.SevenDay, t.SevenDay) {
		return true
	}
	return false
}

// meetsThreshold tests whether value exceeds the given threshold.
// If threshold is nil, the threshold is considered "not set"
// and the method always returns false.
func meetsThreshold(value int64, threshold *int64) bool {
	if threshold == nil {
		return false
	}
	thresholdValue := *threshold
	return value >= thresholdValue
}

// ThresholdExplanation describes a threshold which was evaluated on
// a cluster's impact.
type ThresholdExplanation struct {
	// A human-readable explanation of the metric.
	Metric string
	// The number of days the metric value was measured over.
	TimescaleDays int
	// The threshold value of the metric.
	Threshold int64
}

// ExplainThresholdMet provides an explanation of why cluster impact would
// not have met the given priority threshold. As the overall threshold is an
// 'OR' combination of its underlying thresholds, this returns a list of all
// thresholds which would not have been met by the cluster's impact.
func ExplainThresholdNotMet(threshold []*configpb.ImpactMetricThreshold) []ThresholdExplanation {
	var results []ThresholdExplanation
	for _, t := range threshold {
		def := metrics.MustByID(metrics.ID(t.MetricId))
		results = append(results, explainMetricCriteriaNotMet(def.HumanReadableName, t.Threshold)...)
	}
	return results
}

func explainMetricCriteriaNotMet(metric string, threshold *configpb.MetricThreshold) []ThresholdExplanation {
	if threshold == nil {
		return nil
	}
	var results []ThresholdExplanation
	if threshold.OneDay != nil {
		results = append(results, ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 1,
			Threshold:     *threshold.OneDay,
		})
	}
	if threshold.ThreeDay != nil {
		results = append(results, ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 3,
			Threshold:     *threshold.ThreeDay,
		})
	}
	if threshold.SevenDay != nil {
		results = append(results, ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 7,
			Threshold:     *threshold.SevenDay,
		})
	}
	return results
}

// ExplainThresholdMet provides an explanation of why the given cluster impact
// met the given priority threshold. As the overall threshold is an 'OR' combination of
// its underlying thresholds, this returns an example of a threshold which a metric
// value exceeded.
func (c *ClusterMetrics) ExplainThresholdMet(thresholds []*configpb.ImpactMetricThreshold) ThresholdExplanation {
	for _, t := range thresholds {
		def := metrics.MustByID(metrics.ID(t.MetricId))
		impact, ok := (*c)[metrics.ID(t.MetricId)]
		if !ok {
			continue
		}
		explanation := explainMetricThresholdMet(def.HumanReadableName, impact, t.Threshold)
		if explanation != nil {
			return *explanation
		}
	}
	// This should not occur, unless the threshold was not met.
	return ThresholdExplanation{}
}

func explainMetricThresholdMet(metric string, impact MetricValues, threshold *configpb.MetricThreshold) *ThresholdExplanation {
	if threshold == nil {
		return nil
	}
	if threshold.OneDay != nil && impact.OneDay >= *threshold.OneDay {
		return &ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 1,
			Threshold:     *threshold.OneDay,
		}
	}
	if threshold.ThreeDay != nil && impact.ThreeDay >= *threshold.ThreeDay {
		return &ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 3,
			Threshold:     *threshold.ThreeDay,
		}
	}
	if threshold.SevenDay != nil && impact.SevenDay >= *threshold.SevenDay {
		return &ThresholdExplanation{
			Metric:        metric,
			TimescaleDays: 7,
			Threshold:     *threshold.SevenDay,
		}
	}
	return nil
}

// MergeThresholdMetExplanations merges multiple explanations for why thresholds
// were met into a minimal list, that removes redundant explanations.
func MergeThresholdMetExplanations(explanations []ThresholdExplanation) []ThresholdExplanation {
	var results []ThresholdExplanation
	for _, exp := range explanations {
		var merged bool
		for i, otherExp := range results {
			if otherExp.Metric == exp.Metric && otherExp.TimescaleDays == exp.TimescaleDays {
				threshold := otherExp.Threshold
				if exp.Threshold > threshold {
					threshold = exp.Threshold
				}
				results[i].Threshold = threshold
				merged = true
				break
			}
		}
		if !merged {
			results = append(results, exp)
		}
	}
	return results
}

// ExplainThresholdNotMetMessage generates a message that explains why the threshold
// was not met.
//
// It combines the reasons of multiple threshold and why we didn't meet them.
func ExplainThresholdNotMetMessage(thresoldNotMet []*configpb.ImpactMetricThreshold) string {
	explanation := ExplainThresholdNotMet(thresoldNotMet)

	var message strings.Builder
	// As there may be multiple ways in which we could have met the
	// threshold for the next-higher priority (due to the OR-
	// disjunction of different metric thresholds), we must explain why
	// we did not meet any of them.
	for i, exp := range explanation {
		message.WriteString(fmt.Sprintf("- %s (%v-day) < %v", exp.Metric, exp.TimescaleDays, exp.Threshold))
		if i < (len(explanation) - 1) {
			message.WriteString(", and")
		}
		message.WriteString("\n")
	}
	return message.String()
}

// ExplainThresholdsMet creates a threshold explanation of how the cluster's impact met
// the threshold of a priority.
func ExplainThresholdsMet(impact ClusterMetrics, thresholds ...[]*configpb.ImpactMetricThreshold) string {
	var explanations []ThresholdExplanation
	for _, t := range thresholds {
		// There may be multiple ways in which we could have met the
		// threshold for the next-higher priority (due to the OR-
		// disjunction of different metric thresholds). This obtains
		// just one of the ways in which we met it.
		explanations = append(explanations, impact.ExplainThresholdMet(t))
	}

	// Remove redundant explanations.
	// E.g. "Presubmit Runs Failed (1-day) >= 15"
	// and "Presubmit Runs Failed (1-day) >= 30" can be merged to just
	// "Presubmit Runs Failed (1-day) >= 30", because the latter
	// trivially implies the former.
	explanations = MergeThresholdMetExplanations(explanations)

	var message strings.Builder
	for i, exp := range explanations {
		message.WriteString(fmt.Sprintf("- %s (%v-day) >= %v", exp.Metric, exp.TimescaleDays, exp.Threshold))
		if i < (len(explanations) - 1) {
			message.WriteString(", and")
		}
		message.WriteString("\n")
	}
	return message.String()
}
