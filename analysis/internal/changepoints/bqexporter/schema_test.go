// Copyright 2023 The LUCI Authors.
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

package bqexporter

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSchema(t *testing.T) {
	t.Parallel()
	ftt.Run(`With Schema`, t, func(t *ftt.Test) {
		var fieldNames []string
		for _, field := range tableMetadata.Schema {
			fieldNames = append(fieldNames, field.Name)
		}
		t.Run(`Range partitioning field is defined`, func(t *ftt.Test) {
			partitioningField := tableMetadata.RangePartitioning.Field
			assert.Loosely(t, partitioningField, should.BeIn(fieldNames...))
		})
		t.Run(`Clustering fields are defined`, func(t *ftt.Test) {
			for _, clusteringField := range tableMetadata.Clustering.Fields {
				assert.Loosely(t, clusteringField, should.BeIn(fieldNames...))
			}
		})
	})
}
