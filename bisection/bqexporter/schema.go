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
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/types/descriptorpb"

	bqpb "go.chromium.org/luci/bisection/proto/bq"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/bqutil"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/bq"
)

// The table containing test analyses.
const testFailureAnalysesTableName = "test_failure_analyses"

const partitionExpirationTime = 540 * 24 * time.Hour // 540 days (~1.5 years).

// schemaApplyer ensures BQ schema matches the row proto definitions.
var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(5))

const rowMessage = "luci.bisection.proto.bq.TestAnalysisRow"

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
			Field:      "created_time",
		},
		// Relax ensures no fields are marked "required".
		Schema: schema.Relax(),
	}
}

func generateRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestAnalysisRow{})
	// We also need to get FileDescriptorProto for:
	// - TestCulprit
	// - BuilderID
	// - TestFailure
	// - GitilesCommit
	// - TestNthSectionAnalysisResult
	// - CulpritAction
	// - Variant
	// - RegressionRange
	// because they are defined in different files.
	fdtc, _ := descriptor.MessageDescriptorProto(&pb.TestCulprit{})
	fdbid, _ := descriptor.MessageDescriptorProto(&bbpb.BuilderID{})
	fdtf, _ := descriptor.MessageDescriptorProto(&pb.TestFailure{})
	fdgc, _ := descriptor.MessageDescriptorProto(&bbpb.GitilesCommit{})
	fdtns, _ := descriptor.MessageDescriptorProto(&pb.TestNthSectionAnalysisResult{})
	fdca, _ := descriptor.MessageDescriptorProto(&pb.CulpritAction{})
	fdvr, _ := descriptor.MessageDescriptorProto(&pb.Variant{})
	fdrr, _ := descriptor.MessageDescriptorProto(&pb.RegressionRange{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdtc, fdbid, fdtf, fdgc, fdtns, fdca, fdvr, fdrr}}
	return bqutil.GenerateSchema(fdset, rowMessage)
}

func generateRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.TestAnalysisRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
