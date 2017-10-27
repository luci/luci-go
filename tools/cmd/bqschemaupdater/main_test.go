// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"testing"

	"cloud.google.com/go/bigquery"
	"golang.org/x/net/context"

	"io/ioutil"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBQSchemaUpdater(t *testing.T) {
	ctx := context.Background()
	Convey("Update", t, func() {
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
			err := updateFromTableDef(ctx, ts, td)
			So(err, ShouldBeNil)
			got, err := ts.getTableMetadata(ctx, datasetID, tableID)
			So(err, ShouldBeNil)
			want := &bigquery.TableMetadata{Schema: tc}
			So(got, ShouldResemble, want)
		}
	})
	Convey("Schema", t, func() {
		descBytes, err := ioutil.ReadFile("testdata/event.desc")
		So(err, ShouldBeNil)
		var desc descriptor.FileDescriptorSet
		So(proto.Unmarshal(descBytes, &desc), ShouldBeNil)
		schema, description, err := schemaFromMessage(&desc, "test.BuildEvent")
		So(err, ShouldBeNil)
		So(description, ShouldEqual, "Build events.\n\nLine after blank line.")
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
		So(schema, ShouldResemble, bigquery.Schema{
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
		})
	})
}
