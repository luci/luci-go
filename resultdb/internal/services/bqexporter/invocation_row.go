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

package bqexporter

import (
	"context"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/bqutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var invocationRowSchema bigquery.Schema

const invocationRowMessage = "luci.resultdb.bq.InvocationRow"

func init() {
	var err error
	if invocationRowSchema, err = generateInvocationRowSchema(); err != nil {
		panic(err)
	}
}

func generateInvocationRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRow{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdsp}}
	return bqutil.GenerateSchema(fdset, invocationRowMessage)
}

// invocationRowInput is information required to generate an invocation BigQuery row.
type invocationRowInput struct {
	inv *pb.Invocation
}

func (i *invocationRowInput) row() proto.Message {
	inv := i.inv
	project, realm := realms.Split(inv.Realm)
	propertiesJSON, err := bqutil.MarshalStructPB(inv.Properties)
	if err != nil {
		panic(errors.Annotate(err, "marshal properties").Err())
	}
	extendedPropertiesJSON, err := bqutil.MarshalStringStructPBMap(inv.ExtendedProperties)
	if err != nil {
		panic(errors.Annotate(err, "marshal extended_properties").Err())
	}

	return &bqpb.InvocationRow{
		Project:             project,
		Realm:               realm,
		Id:                  string(invocations.MustParseName(inv.Name)),
		CreateTime:          inv.CreateTime,
		Tags:                inv.Tags,
		FinalizeTime:        inv.FinalizeTime,
		IncludedInvocations: inv.IncludedInvocations,
		IsExportRoot:        inv.IsExportRoot,
		ProducerResource:    inv.ProducerResource,
		Properties:          propertiesJSON,
		ExtendedProperties:  extendedPropertiesJSON,
		PartitionTime:       inv.CreateTime,
	}
}

func (i *invocationRowInput) id() []byte {
	return []byte(i.inv.Name)
}

// exportInvocationToBigQuery queries all reachable invocations from an
// invocation ID in Spanner then exports them to BigQuery.
func (b *bqExporter) exportInvocationToBigQuery(ctx context.Context, invID invocations.ID) error {
	// TODO(crbug.com/341362001): Add the implementation
	return nil
}
