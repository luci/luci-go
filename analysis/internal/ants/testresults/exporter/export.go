// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package exporter exports test results to BigQuery in AnTS format.
package exporter

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.AntsTestResultRow) error
}

// Exporter provides methods to stream test results into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// ExportOptions captures context which will be exported
// alongside the test results.
type ExportOptions struct {
	Invocation *rdbpb.Invocation
}

// Export exports the given test results to BigQuery.
func (e *Exporter) Export(ctx context.Context, testVariants []*rdbpb.TestVariant, opts ExportOptions) error {

	exportRow, err := prepareExportRow(testVariants, opts)
	if err != nil {
		return errors.Fmt("prepare row: %w", err)
	}

	if err := e.client.Insert(ctx, exportRow); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

// prepareExportRow prepares a BigQuery export rows.
func prepareExportRow(verdicts []*rdbpb.TestVariant, opts ExportOptions) ([]*bqpb.AntsTestResultRow, error) {

	invocationID, err := rdbpbutil.ParseInvocationName(opts.Invocation.Name)
	if err != nil {
		return nil, errors.Fmt("invalid invocation name %q: %w", invocationID, err)
	}
	// Initially allocate enough space for 2 result per test variant,
	// slice will be re-sized if necessary.
	results := make([]*bqpb.AntsTestResultRow, 0, len(verdicts)*2)

	for _, tv := range verdicts {

		testIDStructured, err := bqutil.StructuredTestIdentifierRDB(tv.TestId, tv.Variant)
		if err != nil {
			return nil, errors.Fmt("test_id_structured: %w", err)
		}
		moduleParameters := make([]*bqpb.StringPair, 0, len(tv.Variant.GetDef()))
		for key, val := range tv.Variant.GetDef() {
			// Module parameters should only contain module-abi and module-param.
			if key == "module_abi" || key == "module_param" {
				moduleParameters = append(moduleParameters, &bqpb.StringPair{
					// AnTS keys are module-abi and module-param (dash instead of underscore).
					Name:  strings.ReplaceAll(key, "_", "-"),
					Value: val,
				})
			}
		}
		moduleParametersHash := ""
		if len(moduleParameters) > 0 {
			moduleParametersHash = hashParameters(moduleParameters)
		}
		testIdentifier := &bqpb.AntsTestResultRow_TestIdentifier{
			Module:               testIDStructured.ModuleName,
			ModuleParameters:     moduleParameters,
			ModuleParametersHash: moduleParametersHash,
			TestClass:            fmt.Sprintf("%s.%s", testIDStructured.CoarseName, testIDStructured.FineName),
			ClassName:            testIDStructured.FineName,
			PackageName:          testIDStructured.CoarseName,
			Method:               testIDStructured.CaseName,
		}
		testIdentifierHash, err := persistentHashTestIdentifier(testIdentifier)
		if err != nil {
			return nil, errors.Fmt("test_identifier_hash: %w", err)
		}

		for _, trb := range tv.Results {
			tr := trb.Result

			timing := &bqpb.AntsTestResultRow_Timing{
				CreationTimestamp: tr.StartTime.AsTime().UnixMilli(),
				CompleteTimestamp: tr.StartTime.AsTime().Add(tr.Duration.AsDuration()).UnixMilli(),
				CreationMonth:     tr.StartTime.AsTime().Format("2006-01"),
			}

			var debugInfo *bqpb.AntsTestResultRow_DebugInfo
			if tr.FailureReason != nil {
				debugInfo = &bqpb.AntsTestResultRow_DebugInfo{
					ErrorMessage: tr.FailureReason.PrimaryErrorMessage,
				}
			}
			// TODO: populate more field when we have them in ResultDB.
			results = append(results, &bqpb.AntsTestResultRow{
				TestResultId:       tr.ResultId,
				InvocationId:       invocationID,
				TestIdentifier:     testIdentifier,
				TestStatus:         convertToAnTSStatus(tr.Status),
				TestIdentifierHash: testIdentifierHash,
				// TODO: populate these hashes when we have invocation data available.
				TestIdentifierId:       "",
				TestDefinitionId:       "",
				ParentTestIdentifierId: "",
				DebugInfo:              debugInfo,
				Timing:                 timing,
				Properties:             convertToAnTSStringPair(tr.Tags),
				TestId:                 tv.TestId,
				CompletionTime:         opts.Invocation.FinalizeTime,
			})
		}
	}
	return results, nil
}

func convertToAnTSStatus(status rdbpb.TestStatus) bqpb.AntsTestResultRow_TestStatus {
	// Roughly map to AntS test results.
	// This will be changed after we have the new ResultDB test status.
	switch status {
	case rdbpb.TestStatus_PASS:
		return bqpb.AntsTestResultRow_PASS
	case rdbpb.TestStatus_FAIL:
		return bqpb.AntsTestResultRow_FAIL
	case rdbpb.TestStatus_SKIP:
		return bqpb.AntsTestResultRow_TEST_SKIPPED
	case rdbpb.TestStatus_ABORT:
		return bqpb.AntsTestResultRow_FAIL
	case rdbpb.TestStatus_CRASH:
		return bqpb.AntsTestResultRow_FAIL
	default:
		return bqpb.AntsTestResultRow_TEST_STATUS_UNSPECIFIED
	}
}

func convertToAnTSStringPair(pairs []*rdbpb.StringPair) []*bqpb.StringPair {
	result := make([]*bqpb.StringPair, 0, len(pairs))
	for _, pair := range pairs {
		result = append(result, &bqpb.StringPair{
			Name:  pair.Key,
			Value: pair.Value,
		})
	}
	return result
}
