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

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google/descutil"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var testResultRowSchema bigquery.Schema

var testResultRowMessage = "luci.resultdb.bq.TestResultRow"

func init() {
	var err error
	if testResultRowSchema, err = generateSchema(); err != nil {
		panic(err)
	}
}

func generateSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TestResultRow{})
	// We also need to get FileDescriptorProto for StringPair and TestMetadata
	// because they are defined in different files.
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdtmd, _ := descriptor.MessageDescriptorProto(&pb.TestMetadata{})
	fdinv, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRecord{})
	fdset := &desc.FileDescriptorSet{
		File: []*desc.FileDescriptorProto{fd, fdsp, fdtmd, fdinv}}
	conv := bq.SchemaConverter{
		Desc:           fdset,
		SourceCodeInfo: make(map[*desc.FileDescriptorProto]bq.SourceCodeInfoMap, len(fdset.File)),
	}
	for _, f := range fdset.File {
		conv.SourceCodeInfo[f], err = descutil.IndexSourceCodeInfo(f)
		if err != nil {
			return nil, errors.Annotate(err, "failed to index source code info in file %q", fd.GetName()).Err()
		}
	}
	schema, _, err = conv.Schema(testResultRowMessage)
	return schema, err
}

// Row size limit is 5MB according to
// https://cloud.google.com/bigquery/quotas#streaming_inserts
// Cap the summaryHTML's length to 4MB to ensure the row size is under
// limit.
const maxSummaryLength = 4e6

func invocationProtoToRecord(inv *pb.Invocation) *bqpb.InvocationRecord {
	return &bqpb.InvocationRecord{
		Id:    string(invocations.MustParseName(inv.Name)),
		Tags:  inv.Tags,
		Realm: inv.Realm,
	}
}

// rowInput is information required to generate a TestResult BigQuery row.
type rowInput struct {
	exported   *pb.Invocation
	parent     *pb.Invocation
	tr         *pb.TestResult
	exonerated bool
}

func (i *rowInput) row() *bqpb.TestResultRow {
	tr := i.tr

	ret := &bqpb.TestResultRow{
		Exported:      invocationProtoToRecord(i.exported),
		Parent:        invocationProtoToRecord(i.parent),
		TestId:        tr.TestId,
		ResultId:      tr.ResultId,
		Variant:       pbutil.VariantToStringPairs(tr.Variant),
		VariantHash:   tr.VariantHash,
		Expected:      tr.Expected,
		Status:        tr.Status.String(),
		SummaryHtml:   tr.SummaryHtml,
		StartTime:     tr.StartTime,
		Duration:      tr.Duration,
		Tags:          tr.Tags,
		Exonerated:    i.exonerated,
		PartitionTime: i.exported.CreateTime,
		TestLocation:  tr.TestLocation,
		TestMetadata:  tr.TestMetadata,
	}

	if len(ret.SummaryHtml) > maxSummaryLength {
		ret.SummaryHtml = "[Trimmed] " + ret.SummaryHtml[:maxSummaryLength]
	}

	return ret
}

// generateBQRow returns a *bq.Row to be inserted into BQ.
func (b *bqExporter) generateBQRow(input *rowInput) *bq.Row {
	ret := &bq.Row{Message: input.row()}

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
