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
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const partitionExpirationTime = 510 * 24 * time.Hour // 510 days, or 540 days minus 30 days deletion time.

const rowMessage = "luci.analysis.bq.TestResultRow"

type ExportDestination struct {
	// A unique key for the export destination, using only characters [a-z\-].
	Key string
	// The name of the table in the internal dataset.
	tableName string
	// The desired schema of the table.
	tableMetadata *bigquery.TableMetadata
}

// ByDayTable is a BigQuery table optimised for access by single partition day(s) at a time.
var ByDayTable ExportDestination

// ByMonthTable is a BigQuery table optimised for accessing many months of data for each test_id.
var ByMonthTable ExportDestination

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

	ByDayTable = ExportDestination{
		Key:       "partitioned-by-day",
		tableName: "test_results",
		tableMetadata: &bigquery.TableMetadata{
			TimePartitioning: &bigquery.TimePartitioning{
				Type:       bigquery.DayPartitioningType,
				Expiration: partitionExpirationTime,
				Field:      "partition_time",
			},
			Clustering: &bigquery.Clustering{
				Fields: []string{"project", "test_id"},
			},
			Description: "Contains test results produced by all LUCI Projects. Optimised for access over a narrow range of partition dates.",
			// Relax ensures no fields are marked "required".
			Schema: schema.Relax(),
			Labels: map[string]string{bq.MetadataVersionKey: "1"},
		},
	}

	// Table optimised for access by test_id, (variant_hash) over long time ranges.
	ByMonthTable = ExportDestination{
		Key:       "partitioned-by-month",
		tableName: "test_results_by_month",
		tableMetadata: &bigquery.TableMetadata{
			TimePartitioning: &bigquery.TimePartitioning{
				Type: bigquery.MonthPartitioningType,
				// A month is up to 30 days longer than a day, adjust
				// expiration time accordingly.
				Expiration: partitionExpirationTime - 30*24*time.Hour,
				Field:      "partition_time",
			},
			Clustering: &bigquery.Clustering{
				Fields: []string{"project", "test_id", "variant_hash"},
			},
			Description: "Contains test results produced by all LUCI Projects, optimised for access by test ID over long time ranges.",
			// Relax ensures no fields are marked "required".
			Schema: schema.Relax(),
			Labels: map[string]string{bq.MetadataVersionKey: "1"},
		},
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestResultRow{})
	// We also need to get FileDescriptorProto for other referenced protos
	// because they are defined in different files.
	fdtid, _ := descriptor.MessageDescriptorProto(&bqpb.TestIdentifier{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdfr, _ := descriptor.MessageDescriptorProto(&rdbpb.FailureReason{})
	fdsrc, _ := descriptor.MessageDescriptorProto(&pb.Sources{})
	fdgc, _ := descriptor.MessageDescriptorProto(&pb.GitilesCommit{})
	fdcl, _ := descriptor.MessageDescriptorProto(&pb.Changelist{})
	fdtmd, _ := descriptor.MessageDescriptorProto(&bqpb.TestMetadata{})
	fdtl, _ := descriptor.MessageDescriptorProto(&rdbpb.TestLocation{})
	fdbtc, _ := descriptor.MessageDescriptorProto(&rdbpb.BugComponent{})
	fdsr, _ := descriptor.MessageDescriptorProto(&rdbpb.SkippedReason{})
	fdfx, _ := descriptor.MessageDescriptorProto(&rdbpb.FrameworkExtensions{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{
		fd, fdtid, fdsp, fdfr, fdsrc, fdgc, fdcl, fdtmd, fdtl, fdbtc, fdsr, fdfx,
	}}

	return bq.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.TestResultRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
