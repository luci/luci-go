// Copyright 2026 The LUCI Authors.
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

// Package exporter contains methods to export test results to BigQuery.
package exporter

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.TestResultRow, dest ExportDestination) error
}

// Exporter provides methods to stream test results to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

type Options struct {
	// Metadata about the root invocation the test results exist inside.
	// The partition time for the export is the root invocation's creation time.
	RootInvocation *resultpb.RootInvocationMetadata
	// The sources of the test results.
	Sources *pb.Sources
}

func (e *Exporter) Export(ctx context.Context, results []*rdbpb.TestResultsNotification_TestResultsByWorkUnit, dest ExportDestination, opts Options) error {
	// Use the same timestamp for all rows exported in the same batch.
	insertTime := clock.Now(ctx)

	rows, err := prepareExportRows(results, opts, insertTime)
	if err != nil {
		return errors.Fmt("prepare rows: %w", err)
	}

	if err := e.client.Insert(ctx, rows, dest); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

func prepareExportRows(results []*rdbpb.TestResultsNotification_TestResultsByWorkUnit, opts Options, insertTime time.Time) ([]*bqpb.TestResultRow, error) {
	rootInvocation := opts.RootInvocation
	if rootInvocation == nil {
		return nil, errors.New("root invocation must be specified")
	}
	rootProject, _, err := perms.SplitRealm(rootInvocation.Realm)
	if err != nil {
		return nil, errors.Fmt("invalid root realm: %w", err)
	}

	sources := opts.Sources
	if sources == nil {
		// This is an ResultDB invariant.
		return nil, errors.New("sources must be specified")
	}
	var sourceRef *pb.SourceRef
	var sourceRefHash string
	sourceRef = pbutil.SourceRefFromSources(sources)
	if sourceRef != nil {
		sourceRefHash = hex.EncodeToString(pbutil.SourceRefHash(sourceRef))
	}

	var out []*bqpb.TestResultRow

	primaryBuild, err := buildDescriptor(opts.RootInvocation.PrimaryBuild)
	if err != nil {
		return nil, errors.Fmt("build descriptor: %w", err)
	}

	rootInvocationDefinition, err := rootInvocationDefinition(opts.RootInvocation.Definition)
	if err != nil {
		return nil, errors.Fmt("root invocation definition: %w", err)
	}

	for _, wuTestResults := range results {
		parent, err := parentFromWorkUnit(wuTestResults.WorkUnit)
		if err != nil {
			return nil, errors.Fmt("parent from work unit: %w", err)
		}

		for _, tr := range wuTestResults.TestResults {
			variant, err := bqutil.VariantJSON(tr.Variant)
			if err != nil {
				return nil, errors.Fmt("variant: %w", err)
			}

			testIDStructured, err := bqutil.StructuredTestIdentifierRDB(tr.TestId, tr.Variant)
			if err != nil {
				return nil, errors.Fmt("parse structured test id: %w", err)
			}

			propertiesJSON, err := bqutil.MarshalStructPB(tr.Properties)
			if err != nil {
				return nil, errors.Fmt("marshal properties: %w", err)
			}

			tmd, err := bqutil.TestMetadata(tr.TestMetadata)
			if err != nil {
				return nil, errors.Fmt("prepare test metadata: %w", err)
			}

			var skipReasonString string
			// Deprecated SkipReason field.
			if tr.Status == rdbpb.TestStatus_SKIP && tr.SkipReason != rdbpb.SkipReason_SKIP_REASON_UNSPECIFIED {
				skipReasonString = tr.SkipReason.String()
			}

			out = append(out, &bqpb.TestResultRow{
				Project:          rootProject,
				TestIdStructured: testIDStructured,
				TestId:           tr.TestId,
				Variant:          variant,
				VariantHash:      tr.VariantHash,
				Invocation: &bqpb.TestResultRow_InvocationRecord{
					Id:               rootInvocation.RootInvocationId,
					Realm:            rootInvocation.Realm,
					IsRootInvocation: true,
				},
				PartitionTime:            rootInvocation.CreateTime,
				Parent:                   parent,
				Name:                     tr.Name,
				ResultId:                 tr.ResultId,
				Expected:                 tr.Expected,
				Status:                   pbutil.LegacyTestStatusFromResultDB(tr.Status),
				StatusV2:                 pbutil.TestStatusV2FromResultDB(tr.StatusV2),
				SummaryHtml:              tr.SummaryHtml,
				StartTime:                tr.StartTime,
				DurationSecs:             tr.Duration.AsDuration().Seconds(),
				Tags:                     pbutil.StringPairFromResultDB(tr.Tags),
				FailureReason:            tr.FailureReason,
				SkipReason:               skipReasonString,
				Properties:               propertiesJSON,
				Sources:                  sources,
				SourceRef:                sourceRef,
				SourceRefHash:            sourceRefHash,
				PrimaryBuild:             primaryBuild,
				RootInvocationDefinition: rootInvocationDefinition,
				TestMetadata:             tmd,
				SkippedReason:            tr.SkippedReason,
				FrameworkExtensions:      tr.FrameworkExtensions,
				InsertTime:               timestamppb.New(insertTime),
			})
		}
	}
	return out, nil
}

func parentFromWorkUnit(wu *rdbpb.WorkUnit) (*bqpb.TestResultRow_ParentRecord, error) {
	propertiesJSON, err := bqutil.MarshalStructPB(wu.Properties)
	if err != nil {
		return nil, errors.Fmt("marshal properties: %w", err)
	}
	parent := &bqpb.TestResultRow_ParentRecord{
		Id:         wu.WorkUnitId,
		Tags:       pbutil.StringPairFromResultDB(wu.Tags),
		Realm:      wu.Realm,
		Properties: propertiesJSON,
	}
	return parent, nil
}

// rootInvocationDefinition converts a ResultDB RootInvocationDefinition
// into its BigQuery export format.
func rootInvocationDefinition(def *rdbpb.RootInvocationDefinition) (*bqpb.RootInvocationDefinition, error) {
	if def == nil {
		// The definition is an optional field in ResultDB.
		return nil, nil
	}

	b, err := json.Marshal(def.Properties.Def)
	if err != nil {
		return nil, err
	}

	result := &bqpb.RootInvocationDefinition{
		System:         def.System,
		Name:           def.Name,
		Properties:     string(b),
		PropertiesHash: def.PropertiesHash,
	}
	return result, nil
}

// buildDescriptor converts a ResultDB BuildDescriptor into its BigQuery export format.
func buildDescriptor(build *rdbpb.BuildDescriptor) (*bqpb.BuildDescriptor, error) {
	if build == nil {
		// The primary build is an optional field in ResultDB.
		return nil, nil
	}
	switch build := build.Definition.(type) {
	case *rdbpb.BuildDescriptor_AndroidBuild:
		return &bqpb.BuildDescriptor{
			Definition: &bqpb.BuildDescriptor_AndroidBuild{
				AndroidBuild: androidBuildDescriptor(build.AndroidBuild),
			},
		}, nil
	default:
		return nil, errors.New("unknown build descriptor type")
	}
}

// androidBuildDescriptor converts a ResultDB AndroidBuildDescriptor into its BigQuery export format.
func androidBuildDescriptor(build *rdbpb.AndroidBuildDescriptor) *bqpb.AndroidBuildDescriptor {
	return &bqpb.AndroidBuildDescriptor{
		DataRealm:   build.DataRealm,
		Branch:      build.Branch,
		BuildTarget: build.BuildTarget,
		BuildId:     build.BuildId,
	}
}
