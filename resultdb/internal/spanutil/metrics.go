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

package spanutil

import (
	"context"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

// RowStatus is a status of a row.
// Used in metrics.
type RowStatus string

// Values of RowStatus type.
const (
	Inserted RowStatus = "INSERTED"
	Deleted  RowStatus = "DELETED"
)

// RowCounter is a metric, tracking the number of row operations in Spanner tables.
var RowCounter = metric.NewCounter(
	"resultdb/spanner/rows",
	"Number of Spanner row operations",
	nil,
	field.String("table"),  // See Table type.
	field.String("status"), // See RowStatus type.
	field.String("realm"),  // Invocation realm
)

// Table identifies a Spanner table.
// Used in metrics.
type Table string

// Values of Table type.
const (
	TestResults     Table = "TestResults"
	Invocations     Table = "Invocations"
	Artifacts       Table = "Artifacts"
	RootInvocations Table = "RootInvocations"
	WorkUnits       Table = "WorkUnits"
)

// IncRowCount increments the row counter.
func IncRowCount(ctx context.Context, count int, table Table, rowStatus RowStatus, realm string) {
	RowCounter.Add(ctx, int64(count), string(table), string(rowStatus), realm)
}
