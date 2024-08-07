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

package testverdicts

import (
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// tableName is the name of the exported BigQuery table.
const tableName = "test_verdicts"

const partitionExpirationTime = 510 * 24 * time.Hour // 510 days, or 540 days minus 30 days deletion time.

const rowMessage = "luci.analysis.bq.TestVerdictRow"

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
		TimePartitioning: &bigquery.TimePartitioning{
			Type:       bigquery.DayPartitioningType,
			Expiration: partitionExpirationTime,
			Field:      "partition_time",
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"project", "test_id"},
		},
		Description: "Contains test verdicts produced by all LUCI Projects.",
		// Relax ensures no fields are marked "required".
		Schema: schema.Relax(),
		Labels: map[string]string{bq.MetadataVersionKey: "1"},
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestVerdictRow{})
	// We also need to get FileDescriptorProto for StringPair, FailureReason,
	// Sources, GitilesCommit, Changelist, TestMetadata, TestLocation and
	// BugComponent.
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdfr, _ := descriptor.MessageDescriptorProto(&pb.FailureReason{})
	fdsrc, _ := descriptor.MessageDescriptorProto(&pb.Sources{})
	fdgc, _ := descriptor.MessageDescriptorProto(&pb.GitilesCommit{})
	fdcl, _ := descriptor.MessageDescriptorProto(&pb.Changelist{})
	fdtmd, _ := descriptor.MessageDescriptorProto(&pb.TestMetadata{})
	fdtl, _ := descriptor.MessageDescriptorProto(&pb.TestLocation{})
	fdbtc, _ := descriptor.MessageDescriptorProto(&pb.BugComponent{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{
		fd, fdsp, fdfr, fdsrc, fdgc, fdcl, fdtmd, fdtl, fdbtc,
	}}
	return bq.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.TestVerdictRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
