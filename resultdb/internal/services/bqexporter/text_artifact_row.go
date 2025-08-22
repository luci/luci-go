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

	// Row size limit is 5MB according to
	// https://cloud.google.com/bigquery/quotas#streaming_inserts
	// Split artifact content into 4MB shards if it's too large.
	contentShardSize = 4e6

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

func (i *textArtifactRowInput) row() proto.Message {
	_, testID, resultID, artifactID := artifacts.MustParseLegacyName(i.a.Name)
	expRec := invocationProtoToRecord(i.exported)
	parRec := invocationProtoToRecord(i.parent)

	return &bqpb.TextArtifactRowLegacy{
		Exported:      expRec,
		Parent:        parRec,
		TestId:        testID,
		ResultId:      resultID,
		ArtifactId:    artifactID,
		ShardId:       i.shardID,
		Content:       i.content,
		PartitionTime: i.exported.CreateTime,
	}
}

func (i *textArtifactRowInput) id() []byte {
	return []byte(fmt.Sprintf("%s/%d", i.a.Name, i.shardID))
}

func (b *bqExporter) downloadArtifactContent(ctx context.Context, a *artifact, rowC chan rowInput) error {
	ac := artifactcontent.Reader{
		RBEInstance: b.Options.ArtifactRBEInstance,
		Hash:        a.RBECASHash,
		Size:        a.SizeBytes,
	}

	var str strings.Builder
	shardId := 0
	input := func() *textArtifactRowInput {
		return &textArtifactRowInput{
			exported: a.exported,
			parent:   a.parent,
			a:        a.Artifact.Artifact,
			shardID:  int32(shardId),
			content:  str.String(),
		}
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

func (b *bqExporter) artifactRowInputToBatch(ctx context.Context, rowC chan rowInput, batchC chan []rowInput) error {
	rows := make([]rowInput, 0, b.MaxBatchRowCount)
	batchSize := 0 // Estimated size of rows in bytes.
	for row := range rowC {
		contentLength := len(row.(*textArtifactRowInput).content)
		if len(rows)+1 >= b.MaxBatchRowCount || batchSize+contentLength >= b.MaxBatchSizeApprox {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- rows:
			}
			rows = make([]rowInput, 0, b.MaxBatchRowCount)
			batchSize = 0
		}
		rows = append(rows, row)
		batchSize += contentLength
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
	batchC := make(chan []rowInput)
	rowC := make(chan rowInput)
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
