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
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// We stream test variant branch updates to this table.
const updatesTableName = "test_variant_segment_updates"

// This is the main table for the test variant branches.
// Periodically we merge from the test_variant_segment_updates to this table.
const stableTableName = "test_variant_segments"

// schemaApplyer ensures BQ schema matches the row proto definitions.
var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(50))

const rowMessage = "luci.analysis.bq.TestVariantBranchRow"

var tableMetadata *bigquery.TableMetadata

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

	tableMetadata = &bigquery.TableMetadata{
		RangePartitioning: &bigquery.RangePartitioning{
			// has_recent_unexpected_results has only 2 possible values: 0 and 1.
			Field: "has_recent_unexpected_results",
			Range: &bigquery.RangePartitioningRange{
				Start: 0,
				// End is exclusive.
				End: 2,
				// The width of each interval range.
				Interval: 1,
			},
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"project", "test_id"},
		},
		// Relax ensures no fields are marked "required".
		Schema: schema.Relax(),
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestVariantBranchRow{})
	// We also need to get FileDescriptorProto for other referenced protos
	// because they are defined in different files.
	fdtid, _ := descriptor.MessageDescriptorProto(&bqpb.TestIdentifier{})
	fdsr, _ := descriptor.MessageDescriptorProto(&pb.SourceRef{})
	fdgr, _ := descriptor.MessageDescriptorProto(&pb.GitilesRef{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdtid, fdsr, fdgr}}
	return bq.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.TestVariantBranchRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
