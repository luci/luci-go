// Copyright 2026 The LUCI Authors.
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
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

const partitionExpirationTime = 510 * 24 * time.Hour // 510 days, or 540 days minus 30 days deletion time.

const rowMessage = "luci.analysis.bq.WorkUnitRow"

type ExportDestination struct {
	// A unique key for the export destination, using only characters [a-z\-].
	Key string
	// The name of the table in the internal dataset.
	tableName string
	// The desired schema of the table.
	tableMetadata *bigquery.TableMetadata
}

// WorkUnitTable is a BigQuery table containing work units.
var WorkUnitTable ExportDestination

// tableSchemaDescriptor is a self-contained DescriptorProto for describing
// row protocol buffers sent to the BigQuery Write API.
var tableSchemaDescriptor *descriptorpb.DescriptorProto

func init() {
	var err error
	var schema bigquery.Schema
	if schema, err = generateRowSchema(); err != nil {
		panic(err)
	}
	if tableSchemaDescriptor, err = generateRowSchemaDescriptor(); err != nil {
		panic(err)
	}

	WorkUnitTable = ExportDestination{
		Key:       "work-units",
		tableName: "work_units",
		tableMetadata: &bigquery.TableMetadata{
			TimePartitioning: &bigquery.TimePartitioning{
				Type:       bigquery.DayPartitioningType,
				Expiration: partitionExpirationTime,
				Field:      "partition_time",
			},
			Clustering: &bigquery.Clustering{
				Fields: []string{"project", "root_invocation_id"},
			},
			Description: "Contains work units exported from ResultDB for all LUCI Projects. Optimised for access over a narrow range of partition dates.",
			// Relax ensures no fields are marked "required".
			Schema: schema.Relax(),
			Labels: map[string]string{bq.MetadataVersionKey: "1"},
		},
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.WorkUnitRow{})
	// We also need to get FileDescriptorProto for other referenced protos
	// because they are defined in different files.
	fdCommon, _ := descriptor.MessageDescriptorProto(&bqpb.ModuleIdentifier{})
	fdAnalysisCommon, _ := descriptor.MessageDescriptorProto(&analysispb.StringPair{})
	fdRDBCommon, _ := descriptor.MessageDescriptorProto(&rdbpb.ProducerResource{})
	fdWorkUnit, _ := descriptor.MessageDescriptorProto(&rdbpb.WorkUnit{})

	fdset := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fd, fdCommon, fdAnalysisCommon, fdRDBCommon, fdWorkUnit}}
	return bq.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*descriptorpb.DescriptorProto, error) {
	m := &bqpb.WorkUnitRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
