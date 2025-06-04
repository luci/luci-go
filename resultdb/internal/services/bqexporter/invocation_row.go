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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/bqutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// InvExportClient is the interface for exporting invocations.
type InvExportClient interface {
	InsertInvocationRow(ctx context.Context, row *bqpb.InvocationRow) error
}

func prepareInvocationRow(inv *pb.Invocation) (*bqpb.InvocationRow, error) {
	project, realm := realms.Split(inv.Realm)
	propertiesJSON, err := bqutil.MarshalStructPB(inv.Properties)
	if err != nil {
		return nil, errors.Fmt("marshal properties: %w", err)
	}
	extendedPropertiesJSON, err := bqutil.MarshalStringStructPBMap(inv.ExtendedProperties)
	if err != nil {
		return nil, errors.Fmt("marshal extended_properties: %w", err)
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
	}, nil
}

// exportInvocationToBigQuery read the given invocation in Spanner then export
// it to BigQuery.
func (b *bqExporter) exportInvocationToBigQuery(ctx context.Context, invID invocations.ID, invClient InvExportClient) error {
	// Get the invocation to be exported
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	inv, err := invocations.Read(ctx, invID, invocations.AllFields)
	if err != nil {
		return errors.Fmt("error reading invocation: %w", err)
	}
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Fmt("%s not finalized", invID.Name())
	}
	row, err := prepareInvocationRow(inv)
	if err != nil {
		return errors.Fmt("prepare invocation row: %w", err)
	}
	return invClient.InsertInvocationRow(ctx, row)
}
