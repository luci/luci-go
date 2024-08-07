// Copyright 2024 The LUCI Authors.
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

package exporter

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSchema(t *testing.T) {
	t.Parallel()
	Convey(`With Schema`, t, func() {
		destinations := []ExportDestination{ByDayTable, ByMonthTable}
		for _, destination := range destinations {
			So(destination.Key, ShouldNotBeEmpty)
			So(destination.tableName, ShouldNotBeEmpty)

			var fieldNames []string
			for _, field := range destination.tableMetadata.Schema {
				fieldNames = append(fieldNames, field.Name)
			}

			// Time partitioning field is defined
			partitioningField := destination.tableMetadata.TimePartitioning.Field
			So(partitioningField, ShouldBeIn, fieldNames)

			// Clustering fields are defined.
			for _, clusteringField := range destination.tableMetadata.Clustering.Fields {
				So(clusteringField, ShouldBeIn, fieldNames)
			}
		}
	})
}
