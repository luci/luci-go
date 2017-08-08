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
	"time"

	"go.chromium.org/luci/common/tsmon/field"
)

// Cell is the smallest unit of data recorded by tsmon.  Metrics can be
// thought of as multi-dimensional maps (with fields defining the dimensions) -
// a Cell is one value in that map, with information about its fields and its
// type.
type Cell struct {
	MetricInfo
	MetricMetadata
	CellData
}

// MetricInfo contains the definition of a metric.
type MetricInfo struct {
	Name        string
	Description string
	Fields      []field.Field
	ValueType   ValueType
}

// MetricMetadata contains user-provided metadata for a metric.
type MetricMetadata struct {
	Units MetricDataUnits // the unit of recorded data for a given metric.
}

// CellData contains the value of a single cell.
type CellData struct {
	FieldVals []interface{}
	Target    Target
	ResetTime time.Time
	Value     interface{}
}
