// Copyright 2020 The LUCI Authors.
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
	"crypto/sha512"
	"encoding/hex"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var testResultRowSchema bigquery.Schema

func init() {
	var err error
	if testResultRowSchema, err = bigquery.InferSchema(&TestResultRow{}); err != nil {
		panic(err)
	}
}

// Row size limit is 1Mib according to
// https://cloud.google.com/bigquery/quotas#streaming_inserts
// Cap the summaryHTML's length to 0.6M to ensure the row size is under
// limit.
const maxSummaryLength = 6e5

// StringPair is a copy of pb.StringPair, suitable for representing a
// key:value pair in a BQ table.
// Inferred to be a field of type RECORD with Key and Value string fields.
type StringPair struct {
	Key   string `bigquery:"key"`
	Value string `bigquery:"value"`
}

// Invocation is a subset of pb.Invocation for the invocation fields that need
// to be saved in a BQ table.
type Invocation struct {
	// ID is the ID of the invocation.
	ID string `bigquery:"id"`

	// Tags represents Invocation-level string key-value pairs.
	// A key can be repeated.
	Tags []StringPair `bigquery:"tags"`

	// TODO(crbug.com/1090503): Add Realm field.
}

// TestResultRow represents a row in a BigQuery table for result of a functional
// test case.
type TestResultRow struct {
	// ExportedInvocation contains info of the exported invocation.
	// Note that it's possible that this invocation is not the result's
	// immediate parent invocation, but the including invocation.
	ExportedInvocation Invocation `bigquery:"exported"`

	// ParentInvocation contains info of the result's immediate parent
	// invocation.
	ParentInvocation Invocation `bigquery:"parent"`

	// TestID is a unique identifier of the test in a LUCI project.
	// Refer to pb.TestResult.TestId for details.
	TestID string `bigquery:"test_id"`

	// ResultID identifies a test result in a given invocation and test id.
	ResultID string `bigquery:"result_id"`

	// Variant describes one specific way of running the test,
	//  e.g. a specific bucket, builder and a test suite.
	Variant []StringPair `bigquery:"variant"`

	// A hex-encoded sha256 of concatenated "<key>:<value>\n" variant pairs.
	VariantHash string `bigquery:"variant_hash"`

	// Expected is a flag indicating whether the result of test case execution is expected.
	// Refer to pb.TestResult.Expected for details.
	Expected bool `bigquery:"expected"`

	// Status of the test result.
	Status string `bigquery:"status"`

	// SummaryHTML is a human-readable explanation of the result, in HTML.
	SummaryHTML string `bigquery:"summary_html"`

	// StartTime is the point in time when the test case started to execute.
	StartTime bigquery.NullTimestamp `bigquery:"start_time"`

	// Duration of the test case execution in seconds.
	Duration bigquery.NullFloat64 `bigquery:"duration"`

	// Tags contains metadata for this test result.
	// It might describe this particular execution or the test case.
	Tags []StringPair `bigquery:"tags"`

	// If the failures of the test variant are exonerated.
	// Note: the exoneration is at the test variant level, not result level.
	Exonerated bool `bigquery:"exonerated"`

	// PartitionTime is used to partition the table.
	// It is the time when exported invocation was created.
	// https://cloud.google.com/bigquery/docs/creating-column-partitions#limitations
	// mentions "The partitioning column must be a top-level field."
	// So we keep this column here instead of adding the CreateTime to Invocation.
	PartitionTime time.Time `bigquery:"partition_time"`

	// TestLocation is the location of the test definition.
	TestLocation *TestLocation `bigquery:"test_location"`
}

// TestLocation is a location of a test definition, e.g. the file name.
// For field description, see the comments in the TestLocation protobuf message.
type TestLocation struct {
	FileName string `bigquery:"file_name"`
	Line     int    `bigquery:"line"`
}

// Name returns test result name.
func (tr *TestResultRow) Name() string {
	return pbutil.TestResultName(tr.ParentInvocation.ID, tr.TestID, tr.ResultID)
}

// stringPairProtosToStringPairs returns a slice of StringPair derived from *pb.StringPair.
func stringPairProtosToStringPairs(pairs []*pb.StringPair) []StringPair {
	if len(pairs) == 0 {
		return nil
	}

	sp := make([]StringPair, len(pairs))
	for i, p := range pairs {
		sp[i] = StringPair{
			Key:   p.Key,
			Value: p.Value,
		}
	}
	return sp
}

// variantToStringPairs returns a slice of StringPair derived from *pb.Variant.
func variantToStringPairs(vr *pb.Variant) []StringPair {
	defMap := vr.GetDef()
	if len(defMap) == 0 {
		return nil
	}

	keys := pbutil.SortedVariantKeys(vr)
	sp := make([]StringPair, len(keys))
	for i, k := range keys {
		sp[i] = StringPair{
			Key:   k,
			Value: defMap[k],
		}
	}
	return sp
}

func invocationProtoToInvocation(inv *pb.Invocation) Invocation {
	return Invocation{
		ID:   string(invocations.MustParseName(inv.Name)),
		Tags: stringPairProtosToStringPairs(inv.Tags),
		// TODO(crbug.com/1090503): Add Realm field.
	}
}

// rowInput is information required to generate a TestResult BigQuery row.
type rowInput struct {
	exported   *pb.Invocation
	parent     *pb.Invocation
	tr         *pb.TestResult
	exonerated bool
}

func (i *rowInput) row() *TestResultRow {
	tr := i.tr

	ret := &TestResultRow{
		ExportedInvocation: invocationProtoToInvocation(i.exported),
		ParentInvocation:   invocationProtoToInvocation(i.parent),
		TestID:             tr.TestId,
		ResultID:           tr.ResultId,
		Variant:            variantToStringPairs(tr.Variant),
		VariantHash:        tr.VariantHash,
		Expected:           tr.Expected,
		Status:             tr.Status.String(),
		SummaryHTML:        tr.SummaryHtml,
		Tags:               stringPairProtosToStringPairs(tr.Tags),
		Exonerated:         i.exonerated,
		PartitionTime:      pbutil.MustTimestamp(i.exported.CreateTime),
	}

	if tr.StartTime != nil {
		ret.StartTime = bigquery.NullTimestamp{
			Timestamp: pbutil.MustTimestamp(tr.StartTime),
			Valid:     true,
		}
	}

	if tr.Duration != nil {
		ret.Duration = bigquery.NullFloat64{
			Float64: pbutil.MustDuration(tr.Duration).Seconds(),
			Valid:   true,
		}
	}

	if len(ret.SummaryHTML) > maxSummaryLength {
		ret.SummaryHTML = "[Trimmed] " + ret.SummaryHTML[:maxSummaryLength]
	}

	if tr.TestLocation != nil {
		ret.TestLocation = &TestLocation{
			FileName: tr.TestLocation.FileName,
			Line:     int(tr.TestLocation.Line),
		}
	}

	return ret
}

// generateBQRow returns a *bigquery.StructSaver to be inserted into BQ.
func (b *bqExporter) generateBQRow(input *rowInput) *bigquery.StructSaver {
	ret := &bigquery.StructSaver{Struct: input.row()}

	if b.UseInsertIDs {
		// InsertID cannot exceed 128 bytes.
		// https://cloud.google.com/bigquery/quotas#streaming_inserts
		// Use SHA512 which is exactly 128 bytes in hex.
		hash := sha512.Sum512([]byte(input.tr.Name))
		ret.InsertID = hex.EncodeToString(hash[:])
	} else {
		ret.InsertID = bigquery.NoDedupeID
	}

	return ret
}
