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

package artifactexporter

import (
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"

	"go.chromium.org/luci/resultdb/bqutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// The table containing test artifacts.
const tableName = "text_artifacts"

const partitionExpirationTime = 90 * 24 * time.Hour // 90 days.

const rowMessage = "luci.resultdb.bq.TextArtifactRow"

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
		// Clustering on these fields will allow good compression rate and query performance.
		Clustering: &bigquery.Clustering{
			Fields: []string{"project", "test_id", "artifact_shard", "invocation_id"},
		},
		// Relax ensures no fields are marked "required".
		Schema: schema.Relax(),
		Labels: map[string]string{bq.MetadataVersionKey: "1"},
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TextArtifactRow{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdsp}}
	return bqutil.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.TextArtifactRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
