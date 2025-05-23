// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// tableName is the name of the exported BigQuery table.
const tableName = "ants_invocations"

// Only keep the data for 30 days, since all the data in the table will be exported to placer soon after they arrive.
const partitionExpirationTime = 30 * 24 * time.Hour

const rowMessage = "luci.analysis.bq.legacy.AntsInvocationRow"

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
			Field:      "completion_time",
		},
		Description: "Contains invocations produced by Android.",
		// Relax ensures no fields are marked "required".
		Schema: bq.RelaxSchema(schema),
		Labels: map[string]string{bq.MetadataVersionKey: "1"},
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.AntsInvocationRow{})
	// We also need to get FileDescriptorProto for other referenced protos
	// because they are defined in different files.
	fdats, _ := descriptor.MessageDescriptorProto(&bqpb.AntsTestResultRow{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdats}}
	return bq.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.AntsInvocationRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
