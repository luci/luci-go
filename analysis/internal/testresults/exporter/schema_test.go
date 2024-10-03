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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSchema(t *testing.T) {
	t.Parallel()
	ftt.Run(`With Schema`, t, func(t *ftt.Test) {
		destinations := []ExportDestination{ByDayTable, ByMonthTable}
		for _, destination := range destinations {
			assert.Loosely(t, destination.Key, should.NotBeEmpty)
			assert.Loosely(t, destination.tableName, should.NotBeEmpty)

			var fieldNames []string
			for _, field := range destination.tableMetadata.Schema {
				fieldNames = append(fieldNames, field.Name)
			}

			// Time partitioning field is defined
			partitioningField := destination.tableMetadata.TimePartitioning.Field
			assert.Loosely(t, partitioningField, should.BeIn(fieldNames...))

			// Clustering fields are defined.
			for _, clusteringField := range destination.tableMetadata.Clustering.Fields {
				assert.Loosely(t, clusteringField, should.BeIn(fieldNames...))
			}
		}
	})
}
