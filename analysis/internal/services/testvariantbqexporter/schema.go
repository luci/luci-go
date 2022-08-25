// Copyright 2022 The LUCI Authors.
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

package testvariantbqexporter

import (
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.chromium.org/luci/analysis/internal/bqutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const rowMessage = "weetbix.bq.TestVariantRow"

const partitionExpirationTime = 540 * 24 * time.Hour

var tableMetadata *bigquery.TableMetadata

func init() {
	var err error
	var schema bigquery.Schema
	if schema, err = generateRowSchema(); err != nil {
		panic(err)
	}
	tableMetadata = &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Type:       bigquery.DayPartitioningType,
			Expiration: partitionExpirationTime,
			Field:      "partition_time",
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"partition_time", "realm", "test_id", "variant_hash"},
		},
		// Relax ensures no fields are marked "required".
		Schema: schema.Relax(),
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestVariantRow{})
	fdfs, _ := descriptor.MessageDescriptorProto(&atvpb.FlakeStatistics{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdtmd, _ := descriptor.MessageDescriptorProto(&pb.TestMetadata{})
	fdtr, _ := descriptor.MessageDescriptorProto(&pb.TimeRange{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdfs, fdsp, fdtmd, fdtr}}
	return bqutil.GenerateSchema(fdset, rowMessage)
}
