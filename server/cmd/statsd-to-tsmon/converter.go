// Copyright 2020 The LUCI Authors.
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

package main

import (
	"context"
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/metric"
)

var (
	// ErrSkipped is returned by ConvertMetric if the metric is not mentioned in
	// the config and thus should not be reported to tsmon.
	ErrSkipped = errors.New("the metric doesn't match the filter")

	// ErrUnexpectedType is returned by ConvertMetric if statsd metric type
	// doesn't match the metric type specified in the config.
	ErrUnexpectedType = errors.New("the metric has unexpected type")
)

// ConvertMetric uses the config as a template to construct a tsmon metric point
// from a statsd metric point.
func ConvertMetric(ctx context.Context, cfg *Config, m *StatsdMetric) error {
	rule := cfg.FindMatchingRule(m.Name)
	if rule == nil {
		return ErrSkipped
	}

	// Now that we really know the value is needed, parse it.
	val, err := strconv.ParseInt(string(m.Value), 10, 64)
	if err != nil {
		return ErrMalformedStatsdLine
	}

	// Assemble values of metric fields. Some of them come directly from the
	// config rule, and some are picked from the parsed statsd metric name.
	fields := make([]any, len(rule.Fields))
	for i, spec := range rule.Fields {
		switch val := spec.(type) {
		case string:
			fields[i] = val
		case NameComponentIndex:
			fields[i] = string(m.Name[int(val)])
		default:
			panic("impossible")
		}
	}

	// Send the value to tsmon.
	switch m.Type {
	case StatsdMetricGauge:
		// metric.Counter is a metric.Int too, but we want Int specifically.
		if _, ok := rule.Metric.(metric.Counter); ok {
			return ErrUnexpectedType
		}
		if tm, ok := rule.Metric.(metric.Int); ok {
			tm.Set(ctx, val, fields...)
		} else {
			return ErrUnexpectedType
		}

	case StatsdMetricCounter:
		if tm, ok := rule.Metric.(metric.Counter); ok {
			tm.Add(ctx, val, fields...)
		} else {
			return ErrUnexpectedType
		}

	case StatsdMetricTimer:
		if tm, ok := rule.Metric.(metric.CumulativeDistribution); ok {
			tm.Add(ctx, float64(val), fields...)
		} else {
			return ErrUnexpectedType
		}

	default:
		panic("impossible")
	}

	return nil
}
