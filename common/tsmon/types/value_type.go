// Copyright 2015 The LUCI Authors.
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

package types

import (
	"fmt"
)

// ValueType is an enum for the type of a metric.
type ValueType int

// Types of metric values.
const (
	NonCumulativeIntType ValueType = iota
	CumulativeIntType
	NonCumulativeFloatType
	CumulativeFloatType
	StringType
	BoolType
	NonCumulativeDistributionType
	CumulativeDistributionType
)

func (v ValueType) String() string {
	switch v {
	case NonCumulativeIntType:
		return "NonCumulativeIntType"
	case CumulativeIntType:
		return "CumulativeIntType"
	case NonCumulativeFloatType:
		return "NonCumulativeFloatType"
	case CumulativeFloatType:
		return "CumulativeFloatType"
	case StringType:
		return "StringType"
	case BoolType:
		return "BoolType"
	case NonCumulativeDistributionType:
		return "NonCumulativeDistributionType"
	case CumulativeDistributionType:
		return "CumulativeDistributionType"
	}
	panic(fmt.Sprintf("unknown ValueType %d", v))
}

// IsCumulative returns true if this is a cumulative metric value type.
func (v ValueType) IsCumulative() bool {
	return v == CumulativeIntType ||
		v == CumulativeFloatType ||
		v == CumulativeDistributionType
}
