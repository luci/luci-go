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

package metrics

import "fmt"

var OneDayNominalBasis = CalculationBasis{Residual: false, IntervalDays: 1}
var OneDayResidualBasis = CalculationBasis{Residual: true, IntervalDays: 1}
var ThreeDayNominalBasis = CalculationBasis{Residual: false, IntervalDays: 3}
var ThreeDayResidualBasis = CalculationBasis{Residual: true, IntervalDays: 3}
var SevenDayNominalBasis = CalculationBasis{Residual: false, IntervalDays: 7}
var SevenDayResidualBasis = CalculationBasis{Residual: true, IntervalDays: 7}

var CalculationBases = []CalculationBasis{
	OneDayNominalBasis, OneDayResidualBasis,
	ThreeDayNominalBasis, ThreeDayResidualBasis,
	SevenDayNominalBasis, SevenDayResidualBasis,
}

// CalculationBasis defines a way of calculating a metric.
type CalculationBasis struct {
	// Residual is whether failures that are included in bug clusters should
	// be counted in suggested clusters as well.
	Residual bool
	// IntervalDays is the number of days of data to include in the metric.
	IntervalDays int
}

// ColumnSuffix returns a column suffix that can be used to distinguish
// this calculation basis from all other calculation bases.
func (c CalculationBasis) ColumnSuffix() string {
	suffix := fmt.Sprintf("%vd", c.IntervalDays)
	if c.Residual {
		suffix += "_residual"
	}
	return suffix
}

// PutValue stores the metric value obtained for the given calculation basis
// in TimewiseCounts.
func (tc *TimewiseCounts) PutValue(value int64, basis CalculationBasis) {
	var counts *Counts
	switch basis.IntervalDays {
	case 1:
		counts = &tc.OneDay
	case 3:
		counts = &tc.ThreeDay
	case 7:
		counts = &tc.SevenDay
	default:
		panic(fmt.Sprintf("invalid calculation basis, interval days = %v", basis.IntervalDays))
	}
	if basis.Residual {
		counts.Residual = value
	} else {
		counts.Nominal = value
	}
}

// TimewiseCounts captures the value of a metric over multiple time periods.
type TimewiseCounts struct {
	// OneDay is the value of the metric for the last day.
	OneDay Counts
	// ThreeDay is the value of the metric for the last three days.
	ThreeDay Counts
	// SevenDay is the value of the metric for the last seven days.
	SevenDay Counts
}

// Counts captures the values of an integer-valued metric in different
// calculation bases.
type Counts struct {
	// Nominal is the value of the metric, as calculated over all failures
	// in the cluster.
	Nominal int64 `json:"nominal"`
	// The value of the metric excluding failures already counted under
	// other higher-priority clusters (e.g. bug clusters.)
	// For bug clusters, this is the same as the nominal metric value.
	// For suggested clusters, this may be less than the nominal metric
	// value if some of the failures are also in a bug cluster.
	Residual int64 `json:"residual"`
}
