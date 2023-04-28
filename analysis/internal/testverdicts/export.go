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

// Package testverdicts handles export of test verdicts to BigQuery.
package testverdicts

import (
	"context"

	"go.chromium.org/luci/analysis/internal/analysis"
	controlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.TestVerdictRow) error
}

// Exporter provides methods to stream test verdicts into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// ExportOptions captures context which will be exported
// alongside the test verdicts.
type ExportOptions struct {
	Payload    *taskspb.IngestTestResults
	Invocation *rdbpb.Invocation
}

// Export exports the given test verdicts to BigQuery.
func (e *Exporter) Export(ctx context.Context, tvs *rdbpb.QueryTestVariantsResponse, opts ExportOptions) error {
	rows := make([]*bqpb.TestVerdictRow, 0, len(tvs.TestVariants))
	for _, tv := range tvs.TestVariants {
		rows = append(rows, prepareExportRow(tv, tvs.Sources, opts))
	}
	err := e.client.Insert(ctx, rows)
	if err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}

// prepareExportRow prepares a BigQuery export row for a
// ResultDB test verdict.
func prepareExportRow(tv *rdbpb.TestVariant, sourcesByID map[string]*rdbpb.Sources, opts ExportOptions) *bqpb.TestVerdictRow {
	project, _, err := perms.SplitRealm(opts.Invocation.Realm)
	if err != nil {
		panic(errors.Annotate(err, "invalid realm").Err())
	}

	results := make([]*bqpb.TestVerdictRow_TestResult, 0, len(tv.Results))
	for _, r := range tv.Results {
		results = append(results, result(r.Result))
	}

	exonerations := make([]*bqpb.TestVerdictRow_Exoneration, 0, len(tv.Exonerations))
	for _, e := range tv.Exonerations {
		exonerations = append(exonerations, exoneration(e))
	}

	var sources *pb.Sources
	if tv.SourcesId != "" {
		sources = pbutil.SourcesFromResultDB(sourcesByID[tv.SourcesId])
	}

	var metadata *pb.TestMetadata
	if tv.TestMetadata != nil {
		metadata = pbutil.TestMetadataFromResultDB(tv.TestMetadata)
	}

	var cvRun *bqpb.TestVerdictRow_ChangeVerifierRun
	if opts.Payload.PresubmitRun != nil && opts.Payload.PresubmitRun.PresubmitRunId.System == "luci-cv" {
		cvRun = changeVerifierRun(opts.Payload.PresubmitRun)
	}

	var build *bqpb.TestVerdictRow_BuildbucketBuild
	if opts.Payload.Build != nil {
		build = buildbucketBuild(opts.Payload.Build)
	}

	return &bqpb.TestVerdictRow{
		Project:           project,
		TestId:            tv.TestId,
		Variant:           variantJSON(tv.Variant),
		VariantHash:       tv.VariantHash,
		Invocation:        invocation(opts.Invocation),
		PartitionTime:     opts.Payload.PartitionTime,
		Status:            pbutil.TestVerdictStatusFromResultDB(tv.Status),
		Results:           results,
		Exonerations:      exonerations,
		Counts:            counts(results),
		BuildbucketBuild:  build,
		ChangeVerifierRun: cvRun,
		Sources:           sources,
		TestMetadata:      metadata,
	}
}

func invocation(invocation *rdbpb.Invocation) *bqpb.TestVerdictRow_InvocationRecord {
	invocationID, err := rdbpbutil.ParseInvocationName(invocation.Name)
	if err != nil {
		panic(errors.Annotate(err, "invalid invocation name %q", invocationID).Err())
	}

	return &bqpb.TestVerdictRow_InvocationRecord{
		Id:         invocationID,
		Tags:       pbutil.StringPairFromResultDB(invocation.Tags),
		Realm:      invocation.Realm,
		Properties: invocation.Properties,
	}
}

func exoneration(exoneration *rdbpb.TestExoneration) *bqpb.TestVerdictRow_Exoneration {
	return &bqpb.TestVerdictRow_Exoneration{
		ExplanationHtml: exoneration.ExplanationHtml,
		Reason:          pbutil.ExonerationReasonFromResultDB(exoneration.Reason),
	}
}

func counts(results []*bqpb.TestVerdictRow_TestResult) *bqpb.TestVerdictRow_Counts {
	counts := &bqpb.TestVerdictRow_Counts{}
	for _, result := range results {
		counts.Total += 1
		if result.Status != pb.TestResultStatus_SKIP {
			counts.TotalNonSkipped += 1
		}
		if !result.Expected {
			counts.Unexpected += 1
			if result.Status != pb.TestResultStatus_SKIP {
				counts.UnexpectedNonSkipped += 1
				if result.Status != pb.TestResultStatus_PASS {
					counts.UnexpectedNonSkippedNonPassed += 1
				}
			}
		}
	}
	return counts
}

func changeVerifierRun(cv *controlpb.PresubmitResult) *bqpb.TestVerdictRow_ChangeVerifierRun {
	return &bqpb.TestVerdictRow_ChangeVerifierRun{
		Id:              cv.PresubmitRunId.Id,
		Mode:            cv.Mode,
		Status:          analysis.ToBQPresubmitRunStatus(cv.Status),
		IsBuildCritical: cv.Critical,
	}
}

func buildbucketBuild(build *controlpb.BuildResult) *bqpb.TestVerdictRow_BuildbucketBuild {
	return &bqpb.TestVerdictRow_BuildbucketBuild{
		Id: build.Id,
		Builder: &bqpb.TestVerdictRow_BuildbucketBuild_Builder{
			Project: build.Project,
			Bucket:  build.Bucket,
			Builder: build.Builder,
		},
		Status:            analysis.ToBQBuildStatus(build.Status),
		GardenerRotations: build.GardenerRotations,
	}
}

func result(result *rdbpb.TestResult) *bqpb.TestVerdictRow_TestResult {
	return &bqpb.TestVerdictRow_TestResult{
		ResultId:      result.ResultId,
		Expected:      result.Expected,
		Status:        pbutil.TestResultStatusFromResultDB(result.Status),
		SummaryHtml:   result.SummaryHtml,
		StartTime:     result.StartTime,
		Duration:      result.Duration,
		Tags:          pbutil.StringPairFromResultDB(result.Tags),
		FailureReason: pbutil.FailureReasonFromResultDB(result.FailureReason),
		Properties:    result.Properties,
	}
}

func variantJSON(variant *rdbpb.Variant) *structpb.Struct {
	if variant == nil {
		return nil
	}
	fields := make(map[string]*structpb.Value)
	for key, value := range variant.Def {
		fields[key] = &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: value}}
	}
	return &structpb.Struct{Fields: fields}
}
