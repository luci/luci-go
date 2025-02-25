// Copyright 2018 The LUCI Authors.
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
	"os"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBQSchemaUpdater(t *testing.T) {
	ctx := context.Background()
	ftt.Run("Update", t, func(t *ftt.Test) {
		ts := localTableStore{}
		datasetID := "test_dataset"
		tableID := "test_table"

		field := &bigquery.FieldSchema{
			Name:        "test_field",
			Description: "test description",
			Type:        bigquery.StringFieldType,
		}
		anotherField := &bigquery.FieldSchema{
			Name:        "field_2",
			Description: "another field",
			Type:        bigquery.StringFieldType,
		}
		tcs := []bigquery.Schema{
			{field},
			{field, anotherField},
		}
		for _, tc := range tcs {
			td := tableDef{
				DataSetID: "test_dataset",
				TableID:   tableID,
				Schema:    tc,
			}
			err := updateFromTableDef(ctx, true, ts, td)
			assert.Loosely(t, err, should.BeNil)
			got, err := ts.getTableMetadata(ctx, datasetID, tableID)
			assert.Loosely(t, err, should.BeNil)
			want := &bigquery.TableMetadata{
				Schema:           tc,
				TimePartitioning: &bigquery.TimePartitioning{},
			}
			assert.Loosely(t, got, should.Match(want))
		}
	})
	ftt.Run("Schema", t, func(t *ftt.Test) {
		descBytes, err := os.ReadFile("testdata/event.desc")
		assert.Loosely(t, err, should.BeNil)
		var desc descriptorpb.FileDescriptorSet
		assert.Loosely(t, proto.Unmarshal(descBytes, &desc), should.BeNil)
		schema, description, err := schemaFromMessage(&desc, "testdata.BuildEvent")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, description, should.Equal("Build events.\n\nLine after blank line."))
		ioSchema := bigquery.Schema{
			{
				Name:     "properties",
				Type:     bigquery.RecordFieldType,
				Repeated: true,
				Schema: bigquery.Schema{
					{
						Name: "name",
						Type: bigquery.StringFieldType,
					},
					{
						Name: "value_json",
						Type: bigquery.StringFieldType,
					},
				},
			},
		}
		assert.Loosely(t, schema, should.Match(bigquery.Schema{
			{
				Name:        "build_id",
				Description: "Universal build id.",
				Type:        bigquery.StringFieldType,
			},
			{
				Name:        "builder",
				Description: "Builder name.",
				Type:        bigquery.StringFieldType,
			},
			{
				Name:        "status",
				Description: "Valid values: SUCCESS, FAILURE, ERROR.",
				Type:        bigquery.StringFieldType,
			},
			{
				Name:   "input",
				Type:   bigquery.RecordFieldType,
				Schema: ioSchema,
			},
			{
				Name:   "output",
				Type:   bigquery.RecordFieldType,
				Schema: ioSchema,
			},
			{
				Name: "timestamp",
				Type: bigquery.TimestampFieldType,
			},
			{
				Name: "struct",
				Type: bigquery.StringFieldType,
			},
			{
				Name: "duration",
				Type: bigquery.FloatFieldType,
			},
			{
				Name: "bq_type_override",
				Type: bigquery.TimestampFieldType,
			},
		}))
	})
}
