// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/bq"

	"go.chromium.org/luci/auth_service/api/bqpb"
)

const (
	// How long to retain data in a BQ table - set to ~6 months.
	partitionExpirationTime = 6 * 31 * 24 * time.Hour
	// The column to partition tables with.
	partitionColumn = "exported_at"
	// The interval to use for time partitioning.
	partitionType = bigquery.MonthPartitioningType
)

var (
	// schemaApplyer ensures BQ schema matches the row proto definitions.
	schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(6))
	// Metadata for the groups table.
	groupsTableMetadata *bigquery.TableMetadata
	// groupsTableSchemaDescriptor is a self-contained DescriptorProto for
	// describing group row protocol buffers sent to the BigQuery Write API.
	groupsTableSchemaDescriptor *descriptorpb.DescriptorProto
	// Metadata for the realms table.
	realmsTableMetadata *bigquery.TableMetadata
	// realmsTableSchemaDescriptor is a self-contained DescriptorProto for
	// describing realm row protocol buffers sent to the BigQuery Write API.
	realmsTableSchemaDescriptor *descriptorpb.DescriptorProto
	// Metadata for the roles table.
	rolesTableMetadata *bigquery.TableMetadata
	// rolesTableSchemaDescriptor is a self-contained DescriptorProto for
	// describing role row protocol buffers sent to the BigQuery Write API.
	rolesTableSchemaDescriptor *descriptorpb.DescriptorProto
)

func init() {
	groupRow := &bqpb.GroupRow{}
	groupsSchema, err := generateRowSchema(groupRow, "auth.service.bq.GroupRow")
	if err != nil {
		panic(err)
	}
	groupsTableMetadata = &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Type:       partitionType,
			Expiration: partitionExpirationTime,
			Field:      partitionColumn,
		},
		// Relax ensures no fields are marked "required".
		Schema: groupsSchema.Relax(),
		Labels: map[string]string{bq.MetadataVersionKey: "3"},
	}
	groupDesc := groupRow.ProtoReflect().Descriptor()
	if groupsTableSchemaDescriptor, err = generateSchemaDescriptor(groupDesc); err != nil {
		panic(err)
	}

	realmRow := &bqpb.RealmRow{}
	realmsSchema, err := generateRowSchema(realmRow, "auth.service.bq.RealmRow")
	if err != nil {
		panic(err)
	}
	realmsTableMetadata = &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Type:       partitionType,
			Expiration: partitionExpirationTime,
			Field:      partitionColumn,
		},
		// Relax ensures no fields are marked "required".
		Schema: realmsSchema.Relax(),
		Labels: map[string]string{bq.MetadataVersionKey: "1"},
	}
	realmDesc := realmRow.ProtoReflect().Descriptor()
	if realmsTableSchemaDescriptor, err = generateSchemaDescriptor(realmDesc); err != nil {
		panic(err)
	}

	roleRow := &bqpb.RoleRow{}
	rolesSchema, err := generateRowSchema(roleRow, "auth.service.bq.RoleRow")
	if err != nil {
		panic(err)
	}
	rolesTableMetadata = &bigquery.TableMetadata{
		TimePartitioning: &bigquery.TimePartitioning{
			Type:       partitionType,
			Expiration: partitionExpirationTime,
			Field:      partitionColumn,
		},
		// Relax ensures no fields are marked "required".
		Schema: rolesSchema.Relax(),
		Labels: map[string]string{bq.MetadataVersionKey: "1"},
	}
	roleDesc := roleRow.ProtoReflect().Descriptor()
	if rolesTableSchemaDescriptor, err = generateSchemaDescriptor(roleDesc); err != nil {
		panic(err)
	}
}

func generateRowSchema(m proto.Message, name string) (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(m)
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd}}
	return bq.GenerateSchema(fdset, name)
}

func generateSchemaDescriptor(in protoreflect.MessageDescriptor) (*desc.DescriptorProto, error) {
	descriptorProto, err := adapt.NormalizeDescriptor(in)
	if err != nil {
		return nil, err
	}
	return descriptorProto, nil
}
