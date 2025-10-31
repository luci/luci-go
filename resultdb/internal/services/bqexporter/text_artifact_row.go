// Copyright 2021 The LUCI Authors.
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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var textArtifactRowSchema bigquery.Schema

const (
	artifactRowMessage = "luci.resultdb.bq.TextArtifactRowLegacy"

	// Request size limit is 10MB according to
	// https://cloud.google.com/bigquery/quotas#streaming_inserts
	//
	// If files contain non-printable characters, the JSON encoding
	// can be up to 6x larger (as each invalid unicode byte is turned
	// into the unicode replacement character which is encoded as
	// "\ufffd").
	//
	// Split artifact content into 1MB shards if it's too large.
	contentShardSize = 1e6

	// Number of workers to download artifact content.
	artifactWorkers = 10
)

func init() {
	var err error
	if textArtifactRowSchema, err = generateArtifactRowSchema(); err != nil {
		panic(err)
	}
}

func generateArtifactRowSchema() (schema bigquery.Schema, err error) {
	fd, _ := descriptor.MessageDescriptorProto(&bqpb.TextArtifactRowLegacy{})
	fdinv, _ := descriptor.MessageDescriptorProto(&bqpb.InvocationRecord{})
	fdsp, _ := descriptor.MessageDescriptorProto(&pb.StringPair{})
	fdset := &desc.FileDescriptorSet{File: []*desc.FileDescriptorProto{fd, fdinv, fdsp}}
	return bq.GenerateSchema(fdset, artifactRowMessage)
}

// textArtifactRowInput is information required to generate a text artifact BigQuery row.
type textArtifactRowInput struct {
	exported *pb.Invocation
	parent   *pb.Invocation
	a        *pb.Artifact
	shardID  int32
	content  string
}

func (i textArtifactRowInput) toRow() bigqueryRow {
	_, testID, resultID, artifactID := artifacts.MustParseLegacyName(i.a.Name)
	expRec := invocationProtoToRecord(i.exported)
	parRec := invocationProtoToRecord(i.parent)

	rowContent := &bqpb.TextArtifactRowLegacy{
		Exported:      expRec,
		Parent:        parRec,
		TestId:        testID,
		ResultId:      resultID,
		ArtifactId:    artifactID,
		ShardId:       i.shardID,
		PartitionTime: i.exported.CreateTime,
	}

	// Include 500 bytes fixed cost per row for JSON overheads like string
	// field names. Use a more precise estimation method for the content
	// as it is a significant part of the row size and the JSON-enoded
	// size can be up to 6x the original size for some character sequences.
	size := 500 + proto.Size(rowContent) + estimateJSONSize(i.content)
	rowContent.Content = i.content

	return bigqueryRow{
		content: rowContent,
		id:      []byte(fmt.Sprintf("%s/%d", i.a.Name, i.shardID)),
		size:    size,
	}
}

// estimateJSONSize estimates the size of string in JSON encoding.
func estimateJSONSize(content string) int {
	// This estimation function relies upon golang/go/src/encoding/json/encode.go's
	// appendString method.
	// Assume leading and trailing double quotes (").
	estimate := 2
	for _, r := range content {
		if r == '\\' {
			estimate += 2 // Encoded as "\\".
		} else if r >= ' ' && r <= '~' { // Standard ASCII printables.
			estimate += 1 // Encoded as itself.
		} else if r < ' ' { // Range below 0x20
			// Some of these are \n, \r, etc. which need only two bytes.
			// The rest are encoded as \u00xx. Here we assume all take six
			// bytes to be conservative.
			estimate += 6
		} else if r == unicode.ReplacementChar || r == '\u2028' || r == '\u2029' {
			estimate += 6 // Encoded as "\uxxxx" by Go JSON serializer.
		} else {
			// Other unicode is included verbatim.
			estimate += utf8.RuneLen(r)
		}
	}
	return estimate
}

func (b *bqExporter) downloadArtifactContent(ctx context.Context, a *artifact, rowC chan bigqueryRow) error {
	ac := artifactcontent.Reader{
		RBEInstance: b.Options.ArtifactRBEInstance,
		Hash:        a.RBECASHash,
		Size:        a.SizeBytes,
	}

	var str strings.Builder
	shardId := 0
	input := func() bigqueryRow {
		return textArtifactRowInput{
			exported: a.exported,
			parent:   a.parent,
			a:        a.Artifact.Artifact,
			shardID:  int32(shardId),
			content:  str.String(),
		}.toRow()
	}

	err := ac.DownloadRBECASContent(ctx, b.rbecasClient, func(ctx context.Context, pr io.Reader) error {
		sc := bufio.NewScanner(pr)
		//var buf []byte
		sc.Buffer(nil, b.maxTokenSize)

		// Return one line at a time, unless the line exceeds the buffer, then return
		// data as it is.
		sc.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if len(data) == 0 {
				return 0, nil, nil
			}
			if i := bytes.IndexByte(data, '\n'); i >= 0 {
				// We have a full newline-terminated line.
				return i + 1, data[:i+1], nil
			}
			// A partial line occupies the entire buffer, return it as is.
			return len(data), data, nil
		})

		for sc.Scan() {
			if str.Len()+len(sc.Bytes()) > contentShardSize {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case rowC <- input():
				}
				shardId++
				str.Reset()
			}
			str.Write(sc.Bytes())
		}
		if err := sc.Err(); err != nil {
			return err
		}

		if str.Len() > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case rowC <- input():
			}
		}
		return nil
	})
	return errors.WrapIf(err, "read artifact content")
}

type artifact struct {
	*artifacts.Artifact
	exported *pb.Invocation
	parent   *pb.Invocation
}

func (b *bqExporter) queryTextArtifacts(ctx context.Context, exportedID invocations.ID, bqExport *pb.BigQueryExport, artifactC chan *artifact) error {
	exportedInv, err := invocations.Read(ctx, exportedID, invocations.ExcludeExtendedProperties)
	if err != nil {
		return errors.Fmt("error reading exported invocation: %w", err)
	}
	if exportedInv.State != pb.Invocation_FINALIZED {
		return errors.Fmt("%s is not finalized yet", exportedID.Name())
	}

	invs, err := graph.Reachable(ctx, invocations.NewIDSet(exportedID))
	if err != nil {
		return errors.Fmt("querying reachable invocations: %w", err)
	}
	for _, batch := range invs.Batches() {
		contentTypeRegexp := bqExport.GetTextArtifacts().GetPredicate().GetContentTypeRegexp()
		if contentTypeRegexp == "" {
			contentTypeRegexp = "text/.*"
		}
		batchInvocations, err := batch.IDSet()
		if err != nil {
			return err
		}
		q := artifacts.Query{
			InvocationIDs:       batchInvocations,
			TestResultPredicate: bqExport.GetTextArtifacts().GetPredicate().GetTestResultPredicate(),
			ContentTypeRegexp:   contentTypeRegexp,
			ArtifactIDRegexp:    bqExport.GetTextArtifacts().GetPredicate().GetArtifactIdRegexp(),
			WithRBECASHash:      true,
		}

		invs, err := invocations.ReadBatch(ctx, q.InvocationIDs, invocations.ExcludeExtendedProperties)
		if err != nil {
			return err
		}

		err = q.Run(ctx, func(a *artifacts.Artifact) error {
			invID, _, _, _ := artifacts.MustParseLegacyName(a.Name)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case artifactC <- &artifact{Artifact: a, exported: exportedInv, parent: invs[invID]}:
			}
			return nil
		})
		if err != nil {
			return errors.Fmt("exporting batch: %w", err)
		}
	}
	return nil
}

func (b *bqExporter) artifactRowInputToBatch(ctx context.Context, rowC chan bigqueryRow, batchC chan []bigqueryRow) error {
	rows := make([]bigqueryRow, 0, b.MaxBatchRowCount)
	batchSize := 0 // Estimated size of rows in bytes.
	for row := range rowC {
		if len(rows)+1 >= b.MaxBatchRowCount || batchSize+row.size >= b.MaxBatchSizeApprox {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- rows:
			}
			rows = make([]bigqueryRow, 0, b.MaxBatchRowCount)
			batchSize = 0
		}
		rows = append(rows, row)
		batchSize += row.size
	}
	if len(rows) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchC <- rows:
		}
	}
	return nil
}

// exportTextArtifactsToBigQuery queries text artifacts in Spanner then exports them to BigQuery.
func (b *bqExporter) exportTextArtifactsToBigQuery(ctx context.Context, ins inserter, invID invocations.ID, bqExport *pb.BigQueryExport) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Query artifacts and export to BigQuery.
	batchC := make(chan []bigqueryRow)
	rowC := make(chan bigqueryRow)
	artifactC := make(chan *artifact, artifactWorkers)

	// Batch exports rows to BigQuery.
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		err := b.batchExportRows(ctx, ins, batchC, func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row) {
			// Print up to 10 errors.
			for i := 0; i < 10 && i < len(err); i++ {
				a := rows[err[i].RowIndex].Message.(*bqpb.TextArtifactRowLegacy)
				var artifactName string
				if a.TestId != "" {
					artifactName = pbutil.LegacyTestResultArtifactName(a.Parent.Id, a.TestId, a.ResultId, a.ArtifactId)
				} else {
					artifactName = pbutil.LegacyInvocationArtifactName(a.Parent.Id, a.ArtifactId)
				}
				logging.Errorf(ctx, "failed to insert row for %s: %s", artifactName, err[i].Error())
			}
			if len(err) > 10 {
				logging.Errorf(ctx, "%d more row insertions failed", len(err)-10)
			}
		})
		return errors.WrapIf(err, "batch export rows")
	})

	eg.Go(func() error {
		defer close(batchC)
		return errors.WrapIf(b.artifactRowInputToBatch(ctx, rowC, batchC), "artifact row input to batch")
	})

	eg.Go(func() error {
		defer close(rowC)

		subEg, ctx := errgroup.WithContext(ctx)
		for range artifactWorkers {
			subEg.Go(func() error {
				for a := range artifactC {
					if err := b.downloadArtifactContent(ctx, a, rowC); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return errors.WrapIf(subEg.Wait(), "download artifact contents")
	})

	eg.Go(func() error {
		defer close(artifactC)
		return errors.WrapIf(b.queryTextArtifacts(ctx, invID, bqExport, artifactC), "query text artifacts")
	})

	return eg.Wait()
}
