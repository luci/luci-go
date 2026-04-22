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
	"time"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/descriptor"
	desc "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
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



	// softTimeoutLimit is the time limit after which the export task will
	// stop processing new artifacts and schedule a continuation task.
	softTimeoutLimit = 150 * time.Second
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

func (b *bqExporter) queryTextArtifacts(ctx context.Context, task *taskspb.ExportInvocationArtifactsToBQ, startTime time.Time, artifactC chan *artifact) (completed bool, nextBatchIndex int32, nextPageToken string, count int64, err error) {
	exportedID := invocations.ID(task.InvocationId)
	bqExport := task.BqExport

	exportedInv, err := invocations.Read(ctx, exportedID, invocations.ExcludeExtendedProperties)
	if err != nil {
		return false, 0, "", 0, errors.Fmt("error reading exported invocation: %w", err)
	}
	if exportedInv.State != pb.Invocation_FINALIZED {
		return false, 0, "", 0, errors.Fmt("%s is not finalized yet", exportedID.Name())
	}

	invs, err := graph.Reachable(ctx, invocations.NewIDSet(exportedID))
	if err != nil {
		return false, 0, "", 0, errors.Fmt("querying reachable invocations: %w", err)
	}

	batches := invs.Batches()
	var lastProcessed *artifacts.Artifact
	processedCount := task.ProcessedCount
	errTimeout := errors.New("soft timeout limit reached")

	for i := int(task.CurrentBatchIndex); i < len(batches); i++ {
		batch := batches[i]
		lastProcessed = nil
		contentTypeRegexp := bqExport.GetTextArtifacts().GetPredicate().GetContentTypeRegexp()
		if contentTypeRegexp == "" {
			contentTypeRegexp = "text/.*"
		}
		batchInvocations, err := batch.IDSet()
		if err != nil {
			return false, 0, "", 0, err
		}
		q := artifacts.Query{
			InvocationIDs:       batchInvocations,
			TestResultPredicate: bqExport.GetTextArtifacts().GetPredicate().GetTestResultPredicate(),
			ContentTypeRegexp:   contentTypeRegexp,
			ArtifactIDRegexp:    bqExport.GetTextArtifacts().GetPredicate().GetArtifactIdRegexp(),
			WithRBECASHash:      true,
		}

		// Resume from page token of previous task.
		if i == int(task.CurrentBatchIndex) && task.PageToken != "" {
			q.PageToken = task.PageToken
		}

		invs, err := invocations.ReadBatch(ctx, q.InvocationIDs, invocations.ExcludeExtendedProperties)
		if err != nil {
			return false, 0, "", 0, err
		}

		err = q.Run(ctx, func(a *artifacts.Artifact) error {
			if clock.Now(ctx).Sub(startTime) > softTimeoutLimit {
				return errTimeout
			}
			invID, _, _, _ := artifacts.MustParseLegacyName(a.Name)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case artifactC <- &artifact{Artifact: a, exported: exportedInv, parent: invs[invID]}:
			}
			lastProcessed = a
			processedCount++
			return nil
		})

		if errors.Is(err, errTimeout) {
			completed = false
			nextBatchIndex = int32(i)
			if lastProcessed != nil {
				nextPageToken = artifacts.PageToken(lastProcessed.Artifact)
			} else {
				if processedCount == task.ProcessedCount {
					return false, 0, "", 0, errors.New("task made no progress, likely a single item is too large or blocking")
				}
				nextPageToken = ""
			}
			return completed, nextBatchIndex, nextPageToken, processedCount, nil
		}

		if err != nil {
			return false, 0, "", 0, errors.Fmt("exporting batch %d: %w", i, err)
		}
	}

	return true, 0, "", processedCount, nil
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
func (b *bqExporter) exportTextArtifactsToBigQuery(ctx context.Context, ins inserter, task *taskspb.ExportInvocationArtifactsToBQ) error {
	startTime := clock.Now(ctx)
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Query artifacts and export to BigQuery.
	batchC := make(chan []bigqueryRow)
	rowC := make(chan bigqueryRow)
	artifactC := make(chan *artifact, artifactWorkers)

	// Batch exports rows to BigQuery.
	eg, gctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		err := b.batchExportRows(gctx, ins, batchC, func(ctx context.Context, err bigquery.PutMultiError, rows []*bq.Row) {
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
		return errors.WrapIf(b.artifactRowInputToBatch(gctx, rowC, batchC), "artifact row input to batch")
	})

	eg.Go(func() error {
		defer close(rowC)

		subEg, sctx := errgroup.WithContext(gctx)
		for range artifactWorkers {
			subEg.Go(func() error {
				for a := range artifactC {
					if err := b.downloadArtifactContent(sctx, a, rowC); err != nil {
						return err
					}
				}
				return nil
			})
		}
		return errors.WrapIf(subEg.Wait(), "download artifact contents")
	})

	var completed bool
	var nextBatchIndex int32
	var nextPageToken string
	var processedCount int64

	eg.Go(func() error {
		defer close(artifactC)
		var err error
		completed, nextBatchIndex, nextPageToken, processedCount, err = b.queryTextArtifacts(gctx, task, startTime, artifactC)
		return errors.WrapIf(err, "query text artifacts")
	})

	err := eg.Wait()
	if err != nil {
		return err
	}

	if !completed {
		logging.Infof(ctx, "Artifact export for %s exceeded soft timeout after processing %d items. Scheduling continuation task.", task.InvocationId, processedCount)
		// Schedule continuation task
		continuationTask := &taskspb.ExportInvocationArtifactsToBQ{
			InvocationId:      task.InvocationId,
			BqExport:          task.BqExport,
			CurrentBatchIndex: nextBatchIndex,
			PageToken:         nextPageToken,
			ProcessedCount:    processedCount,
		}
		tq.MustAddTask(ctx, &tq.Task{
			Payload: continuationTask,
			Title:   fmt.Sprintf("%s:cont:%d", task.InvocationId, processedCount),
		})
	} else {
		logging.Infof(ctx, "Artifact export for %s completed successfully. Processed %d items.", task.InvocationId, processedCount)
	}

	return nil
}
