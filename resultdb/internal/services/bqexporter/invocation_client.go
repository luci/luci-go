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
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/bqutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// InvClient provides methods to export data to BigQuery
// via the BigQuery Write API.
type InvClient struct {
	// projectID is the name of the GCP project that contains ResultDB
	// BigQuery datasets.
	projectID string
	bqClient  *bigquery.Client
	mwClient  *managedwriter.Client
}

// invTableName is the name of the exported BigQuery table.
const invTableName = "invocations"

const invRowMessage = "luci.resultdb.bq.InvocationRow"

var invocationTableMetadata *bigquery.TableMetadata

// invTableSchemaDescriptor is a self-contained DescriptorProto for
// describing row protocol buffers sent to the BigQuery Write API.
var invTableSchemaDescriptor *descriptorpb.DescriptorProto

func init() {
	var err error
	var schema bigquery.Schema
	if schema, err = generateInvocationRowSchema(); err != nil {
		panic(err)
	}
	if invTableSchemaDescriptor, err = generateInvocationRowSchemaDescriptor(); err != nil {
		panic(err)
	}
	invocationTableMetadata = &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Field:      "partition_time",
			Expiration: partitionExpirationTime,
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"project", "id"},
		},
		Description: "Contains invocations produced by all LUCI Projects.",
		Schema:      schema.Relax(),
		Labels:      map[string]string{bq.MetadataVersionKey: "1"},
	}
}

func generateInvocationRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRow{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdsp}}
	return bq.GenerateSchema(fdset, invRowMessage)
}

func generateInvocationRowSchemaDescriptor() (*desc.DescriptorProto, error) {
	m := &bqpb.InvocationRow{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}

// NewInvClient creates a new client for exporting data
// via the BigQuery Write API.
func NewInvClient(ctx context.Context, projectID string) (s *InvClient, reterr error) {
	if projectID == "" {
		return nil, errors.New("GCP project must be specified")
	}

	bqClient, err := bq.NewClient(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err, "creating BQ client").Err()
	}
	defer func() {
		if reterr != nil {
			// This method failed for some reason, clean up the
			// BigQuery client. Swallow any error returned by the Close()
			// call.
			bqClient.Close()
		}
	}()

	mwClient, err := bq.NewWriterClient(ctx, projectID)
	if err != nil {
		return nil, errors.Annotate(err, "creating managed writer client").Err()
	}
	return &InvClient{
		projectID: projectID,
		bqClient:  bqClient,
		mwClient:  mwClient,
	}, nil
}

// Close releases resources held by the client.
func (c *InvClient) Close() (reterr error) {
	// Ensure both bqClient and mwClient Close() methods
	// are called, even if one panics or fails.
	defer func() {
		err := c.mwClient.Close()
		if reterr == nil {
			reterr = err
		}
	}()
	return c.bqClient.Close()
}

func (c *InvClient) EnsureSchema(ctx context.Context) error {
	table := c.bqClient.Dataset(bqutil.InternalDatasetID).Table(invTableName)
	if err := schemaApplyer.EnsureTable(ctx, table, invocationTableMetadata, bq.UpdateMetadata()); err != nil {
		return errors.Annotate(err, "ensure invocations bq table").Err()
	}
	return nil
}

// InsertInvocationRow inserts one InvocationRow in BigQuery.
func (c *InvClient) InsertInvocationRow(ctx context.Context, row *bqpb.InvocationRow) error {
	if err := c.EnsureSchema(ctx); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}
	tableName := fmt.Sprintf("projects/%s/datasets/%s/tables/%s", c.projectID, bqutil.InternalDatasetID, invTableName)
	writer := bq.NewWriter(c.mwClient, tableName, invTableSchemaDescriptor)
	payload := []proto.Message{row}
	return writer.AppendRowsWithDefaultStream(ctx, payload)
}
