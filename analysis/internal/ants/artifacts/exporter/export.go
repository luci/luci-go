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

package exporter

import (
	"context"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/ants/utils"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.AntsArtifactRow) error
}

// Exporter provides methods to stream rows into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// ExportOptions captures context which will be exported
// alongside the test artifacts.
type ExportOptions struct {
	// Exactly one of Invocation or RootInvocation should be set.
	Invocation     *rdbpb.Invocation
	RootInvocation *rdbpb.RootInvocation
}

// Export inserts the provided artifacts into BigQuery.
func (e *Exporter) Export(ctx context.Context, artifacts []*rdbpb.Artifact, opts ExportOptions) error {
	exportRow, err := prepareExportRow(artifacts, opts)
	if err != nil {
		return errors.Fmt("prepare row: %w", err)
	}

	if err := e.client.Insert(ctx, exportRow); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

// prepareExportRow converts a ResultDB Artifact proto to an AntsArtifactRow BigQuery proto.
func prepareExportRow(artifacts []*rdbpb.Artifact, opts ExportOptions) ([]*bqpb.AntsArtifactRow, error) {
	var invocationIDInDestTable string
	var completionTime *timestamppb.Timestamp
	if opts.RootInvocation != nil {
		invocationIDInDestTable = opts.RootInvocation.RootInvocationId
		completionTime = opts.RootInvocation.FinalizeTime
	} else if opts.Invocation != nil {
		var err error
		invocationIDInDestTable, err = rdbpbutil.ParseInvocationName(opts.Invocation.Name)
		if err != nil {
			return nil, errors.Fmt("invalid invocation name %q: %w", opts.Invocation.Name, err)
		}
		completionTime = opts.Invocation.FinalizeTime
	} else {
		// Should not happen.
		return nil, errors.New("neither Invocation nor RootInvocation is provided")
	}

	results := make([]*bqpb.AntsArtifactRow, 0, len(artifacts))
	for _, artifact := range artifacts {

		artifactRow := &bqpb.AntsArtifactRow{
			InvocationId:   invocationIDInDestTable,
			TestResultId:   artifact.ResultId,
			Name:           artifact.ArtifactId,
			Size:           artifact.SizeBytes,
			ContentType:    artifact.ContentType,
			ArtifactType:   artifact.ArtifactType,
			CompletionTime: completionTime,
		}

		// Populate work unit id.
		if opts.RootInvocation != nil {
			parts, err := rdbpbutil.ParseArtifactName(artifact.Name)
			if err != nil {
				return nil, err
			}
			artifactRow.WorkUnitId = parts.WorkUnitID
		}

		// Populate root invocation info.
		if opts.RootInvocation != nil {
			buildDesc := opts.RootInvocation.PrimaryBuild.GetAndroidBuild()
			if buildDesc != nil {
				artifactRow.BuildType = utils.BuildTypeFromBuildID(buildDesc.BuildId)
				artifactRow.BuildId = buildDesc.BuildId
				artifactRow.BuildProvider = "androidbuild"
				artifactRow.Branch = buildDesc.Branch
				artifactRow.BuildTarget = buildDesc.BuildTarget
			}
		}
		results = append(results, artifactRow)
	}
	return results, nil
}
