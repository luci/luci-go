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

// Package artifactexporter handles uploading artifacts to BigQuery.
// This is the replacement of the legacy artifact exporter in bqexp
package artifactexporter

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/checkpoints"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const CheckpointProcessID = "artifact-exporter"

// CheckpointTTL specifies the TTL for checkpoints in Spanner.
// 7 days for checkpoints should be enough to handle any production issues.
// Another option is to set it to 510 days (equals to Artifact TTL), but
// that may be wasteful.
const CheckpointTTL = 7 * 24 * time.Hour

// MaxShardContentSize is the maximum content size in BQ row.
// Artifacts content bigger than this size needs to be sharded.
// Leave 10 KB for other fields, the rest is content.
// If you need to change this value, please be aware that it may result
// in a different way to split an artifact. This may lead to the current
// checkpoint being incorrect.
const MaxShardContentSize = bq.RowMaxBytes - 10*1024

// MaxRBECasBatchSize is the batch size limit when we read artifact content.
// TODO(nqmtuan): Call the Capacity API to find out the exact size limit for
// batch operations.
// For now, hardcode to be 2MB. It should be under the limit,
// since BatchUpdateBlobs in BatchCreateArtifacts can handle up to 10MB.
// If you need to change this value, please make sure MaxRBECasBatchSize < MaxShardContentSize,
// as we only use 1 Bigquery shard to store these small artifacts.
const MaxRBECasBatchSize = 2 * 1024 * 1024 // 2 MB

// ArtifactRequestOverhead is the overhead (in bytes) applying to each artifact
// when calculating the size.
const ArtifactRequestOverhead = 100

// MaxArtifactSize is the maximum size of an artifact to be exported to BigQuery.
// Artifacts bigger than this size will not be exported.
const MaxArtifactSize = 100 * 1024 * 1024

// MaxTotalArtifactSizeForInvocation is the max total size of artifacts in an invocation.
// If an invocation has total artifact size exceeding this value, we will not export it.
// The reason is that the cloud task cannot finish the export within 10 minutes,
// leading to a timeout.
const MaxTotalArtifactSizeForInvocation = 5 * 1024 * 1024 * 1024 // 5GB

type Artifact struct {
	InvocationID    string
	TestID          string
	ResultID        string
	ArtifactID      string
	ContentType     string
	Size            int64
	RBECASHash      string
	TestStatus      pb.TestStatus
	TestVariant     *pb.Variant
	TestVariantHash string
}

// ExportArtifactsTask describes how to route export artifact task.
var ExportArtifactsTask = tq.RegisterTaskClass(tq.TaskClass{
	ID:            "export-artifacts",
	Prototype:     &taskspb.ExportArtifacts{},
	Kind:          tq.Transactional,
	Queue:         "artifactexporter",
	RoutingPrefix: "/internal/tasks/artifactexporter", // for routing to "artifactexporter" service
})

var (
	ErrInvalidUTF8 = fmt.Errorf("invalid UTF-8 character")
)

var (
	artifactExportCounter = metric.NewCounter(
		"resultdb/artifacts/bqexport",
		"The number of artifacts rows to export to BigQuery, grouped by project and status.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the export.
		// Possible values:
		// - "success": The export was successful.
		// - "failure_input": There was an error with the input artifact
		// (e.g. artifact contains invalid UTF-8 character).
		// - "failure_bq": There was an error with BigQuery (e.g. throttling, load shedding),
		// which made the artifact failed to export.
		// - "skipped_over_limit": The artifact was skipped because it is too big.
		field.String("status"),
	)

	artifactContentCounter = metric.NewCounter(
		"resultdb/artifacts/content",
		"The number of artifacts for a particular content type.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the export.
		// Possible values: "text", "nontext", "empty".
		// We record the group instead of the actual value to prevent
		// the explosion in cardinality.
		field.String("content_type"),
	)

	artifactInvocationCounter = metric.NewCounter(
		"resultdb/artifacts/invocation_exported",
		"The number of invocations got exported, grouped by project and status.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// The status of the export.
		// Possible values:
		// - "success": The export was successful.
		// - "skipped_over_limit": The invocation was skipped because it is too big.
		field.String("status"),
	)
)

// Options is artifact exporter configuration.
type Options struct {
	// ArtifactRBEInstance is the name of the RBE instance to use for artifact
	// storage. Example: "projects/luci-resultdb/instances/artifacts".
	ArtifactRBEInstance string
}

// BQExportClient is the interface for exporting artifacts.
type BQExportClient interface {
	InsertArtifactRows(ctx context.Context, rows []*bqpb.TextArtifactRow) error
}

type artifactExporter struct {
	rbecasInstance string
	// bytestreamClient is the client to stream big artifacts from RBE-CAS.
	bytestreamClient bytestream.ByteStreamClient
	// casClient is used to read artifacts in batch.
	casClient repb.ContentAddressableStorageClient
	// Client to export to BigQuery.
	bqExportClient BQExportClient
}

// Row represent the data to be inserted in the exporter channel.
type Row struct {
	content *bqpb.TextArtifactRow
	// isLastShard is true iff this is the last shard of an artifact.
	isLastShard bool
}

// InitServer initializes a artifactexporter server.
func InitServer(srv *server.Server, opts Options) error {
	if opts.ArtifactRBEInstance == "" {
		return errors.Reason("No rbe instance specified").Err()
	}

	conn, err := artifactcontent.RBEConn(srv.Context)
	if err != nil {
		return err
	}

	bqClient, err := NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "create bq export client").Err()
	}

	srv.RegisterCleanup(func(ctx context.Context) {
		err := conn.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up artifact RBE connection: %s", err)
		}
		err = bqClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up BigQuery export client: %s", err)
		}
	})

	ae := &artifactExporter{
		rbecasInstance:   opts.ArtifactRBEInstance,
		bytestreamClient: bytestream.NewByteStreamClient(conn),
		casClient:        repb.NewContentAddressableStorageClient(conn),
		bqExportClient:   bqClient,
	}

	ExportArtifactsTask.AttachHandler(func(ctx context.Context, msg proto.Message) error {
		task := msg.(*taskspb.ExportArtifacts)
		return ae.exportArtifacts(ctx, invocations.ID(task.InvocationId))
	})
	return nil
}

// Schedule schedules tasks for all the finalized invocations.
func Schedule(ctx context.Context, invID invocations.ID) error {
	return tq.AddTask(ctx, &tq.Task{
		Title:   string(invID),
		Payload: &taskspb.ExportArtifacts{InvocationId: string(invID)},
	})
}

// exportArtifact reads all text artifacts (including artifact content
// in RBE-CAS) for an invocation and exports to BigQuery.
func (ae *artifactExporter) exportArtifacts(ctx context.Context, invID invocations.ID) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.exportArtifacts")
	defer func() { tracing.End(ts, err) }()

	ctx = logging.SetField(ctx, "invocation_id", invID)
	shouldUpload, err := shouldUploadToBQ(ctx)
	if err != nil {
		return errors.Annotate(err, "getting config").Err()
	}
	if !shouldUpload {
		logging.Infof(ctx, "Uploading to BigQuery is disabled")
		return nil
	}

	// Get the invocation.
	inv, err := invocations.Read(span.Single(ctx), invID, invocations.ExcludeExtendedProperties)
	if err != nil {
		return errors.Annotate(err, "error reading exported invocation").Err()
	}
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Reason("invocation not finalized").Err()
	}
	project, _ := realms.Split(inv.Realm)

	// Query for checkpoints.
	cps, err := checkpoints.ReadAllUniquifiers(span.Single(ctx), project, string(invID), CheckpointProcessID)
	if err != nil {
		return errors.Annotate(err, "read all uniquifiers").Err()
	}
	if len(cps) > 0 {
		logging.Infof(ctx, "Found %d checkpoints for invocation %v", len(cps), invID)
	}

	// Query for artifacts.
	artifacts, err := ae.queryTextArtifacts(span.Single(ctx), invID, project, MaxTotalArtifactSizeForInvocation, cps)
	if err != nil {
		return errors.Annotate(err, "query text artifacts").Err()
	}

	logging.Infof(ctx, "Found %d text artifacts", len(artifacts))

	percent, err := percentOfArtifactsToBQ(ctx)
	if err != nil {
		return errors.Annotate(err, "read percent from config").Err()
	}
	artifacts, err = throttleArtifactsForBQ(artifacts, percent)
	if err != nil {
		return errors.Annotate(err, "throttle artifacts for bq").Err()
	}

	logging.Infof(ctx, "Found %d text artifact after throttling", len(artifacts))
	if len(artifacts) == 0 {
		return nil
	}

	// Get artifact content.
	// Channel to push the artifact rows.
	// In theory, some artifact content may be too big to
	// fit into memory (we allow artifacts upto 640MB [1]).
	// We need to "break" them in to sizeable chunks.
	// [1] https://source.corp.google.com/h/chromium/infra/infra_superproject/+/main:data/k8s/projects/luci-resultdb/resultdb.star;l=307?q=max-artifact-content-stream-length
	rowC := make(chan *Row, 10)
	err = parallel.FanOutIn(func(work chan<- func() error) {
		// Download artifacts content and put the rows in a channel.
		work <- func() error {
			defer close(rowC)
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, MaxShardContentSize, MaxRBECasBatchSize, cps)
			if err != nil {
				return errors.Annotate(err, "download multiple artifact content").Err()
			}
			return nil
		}
		// Upload the rows to BQ.
		work <- func() error {
			err := ae.exportToBigQuery(ctx, rowC)
			if err != nil {
				return errors.Annotate(err, "export to bigquery").Err()
			}
			return nil
		}
	})

	if err == nil {
		logging.Infof(ctx, "Finished exporting artifacts task")
		artifactInvocationCounter.Add(ctx, 1, project, "success")
	}

	return err
}

// exportToBigQuery reads rows from channel rowC and export to BigQuery.
// It groups the rows into chunks and appends to the default stream.
// There is a caveat in using default stream.
// If the exporter failed in the middle (with some data already exported to BQ),
// and then retried, duplicated rows may be inserted into BQ.
// Another alternative is to use pending type [1] instead of default stream,
// which allows atomic commit operation. The problem is that it has the limit
// of 10k CreateWriteStream limit per hour, which is not enough for the number
// of invocations we have (1.5 mil/day).
// Another alternative is add a field in the Artifact spanner table to mark which
// rows have been exported.
// [1] https://cloud.google.com/bigquery/docs/write-api#pending_type

func (ae *artifactExporter) exportToBigQuery(ctx context.Context, rowC chan *Row) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.exportToBigQuery")
	defer func() { tracing.End(ts, err) }()

	logging.Infof(ctx, "Start exporting to BigQuery")

	// bqrows contains the rows that will be sent to AppendRows.
	bqrows := []*bqpb.TextArtifactRow{}
	rows := []*Row{}
	currentSize := 0
	for row := range rowC {
		bqrow := row.content
		// Group the rows together such that the total size of the batch
		// does not exceed AppendRows max size.
		// The bqutil package also does the batching, but we don't want to send
		// a just slightly bigger group of row to it, to prevent a big batch and
		// a tiny batch, so we make sure that the rows we send just fit in one batch.
		rowSize := bq.RowSize(bqrow)

		// Exceed max batch size, send whatever we have.
		if currentSize+rowSize > bq.RowMaxBytes {
			logging.Infof(ctx, "Start inserting %d rows to BigQuery", len(bqrows))
			err := ae.bqExportClient.InsertArtifactRows(ctx, bqrows)
			if err != nil {
				artifactExportCounter.Add(ctx, int64(len(bqrows)), bqrow.Project, "failure_bq")
				return errors.Annotate(err, "insert artifact rows").Err()
			}
			// Create checkpoints for the rows.
			err = ae.createCheckPoints(ctx, rows)
			if err != nil {
				// Non-critical, just log and proceed.
				logging.Errorf(ctx, "Failed to create checkpoint for invocation %v: %v", bqrow.InvocationId, err.Error())
			}
			logging.Infof(ctx, "Finished inserting %d rows to BigQuery", len(bqrows))
			artifactExportCounter.Add(ctx, int64(len(bqrows)), bqrow.Project, "success")
			// Reset
			bqrows = []*bqpb.TextArtifactRow{}
			rows = []*Row{}
			currentSize = 0
		}

		bqrows = append(bqrows, bqrow)
		rows = append(rows, row)
		currentSize += rowSize
	}

	// Upload the remaining rows.
	if currentSize > 0 {
		logging.Infof(ctx, "Start inserting last batch of %d rows to BigQuery", len(bqrows))
		err := ae.bqExportClient.InsertArtifactRows(ctx, bqrows)
		if err != nil {
			artifactExportCounter.Add(ctx, int64(len(bqrows)), bqrows[0].Project, "failure_bq")
			return errors.Annotate(err, "insert artifact rows").Err()
		}
		logging.Infof(ctx, "Finished inserting last batch of %d rows to BigQuery", len(bqrows))
		artifactExportCounter.Add(ctx, int64(len(bqrows)), bqrows[0].Project, "success")

		// Create checkpoints for the rows.
		err = ae.createCheckPoints(ctx, rows)
		if err != nil {
			// Non-critical, just log and proceed.
			logging.Errorf(ctx, "Failed to create checkpoint for invocation %v: %v", bqrows[0].InvocationId, err.Error())
		}
	}

	logging.Infof(ctx, "Finished exporting to BigQuery")
	return nil
}

// createCheckPoints create CheckPoint in spanner for rows.
// If an artifact has been fully exported, a row with the artifactUniquifier is created.
// If only part of it has been exported, only those shardUniquifier will be exported.
// Note: we do not delete the shardUniquifier once an artifact is fully exported,
// they will be deleted after TTL.
// If an artifact has its artifactUniquifier in spanner, it means all of its shards have
// been exported.
func (ae *artifactExporter) createCheckPoints(ctx context.Context, rows []*Row) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.createCheckPoints")
	defer func() { tracing.End(ts, err) }()

	ms := []*spanner.Mutation{}

	for _, row := range rows {
		bqrow := row.content
		uniquifier := shardUniquifier(bqrow.TestId, bqrow.ResultId, bqrow.ArtifactId, int(bqrow.ShardId))
		if row.isLastShard {
			uniquifier = artifactUniquifier(bqrow.TestId, bqrow.ResultId, bqrow.ArtifactId)
		}

		checkpointKey := checkpoints.Key{
			Project:    bqrow.Project,
			ResourceID: bqrow.InvocationId,
			ProcessID:  CheckpointProcessID,
			Uniquifier: uniquifier,
		}
		ms = append(ms, checkpoints.Insert(ctx, checkpointKey, CheckpointTTL))
		// Spanner has a limit of 80k mutations per commit.
		// https://cloud.google.com/spanner/quotas#limits-for
		// To be safe, we will do batch of 5k.
		if len(ms) >= 5_000 {
			logging.Infof(ctx, "Writing %d mutations to spanner", len(ms))
			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				span.BufferWrite(ctx, ms...)
				return nil
			})
			if err != nil {
				return errors.Annotate(err, "apply mutations").Err()
			}
			logging.Infof(ctx, "Finish writing %d mutations to spanner", len(ms))
			ms = []*spanner.Mutation{}
		}
	}

	// Apply the remaining mutations.
	if len(ms) > 0 {
		logging.Infof(ctx, "Writing %d mutations", len(ms))
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			span.BufferWrite(ctx, ms...)
			return nil
		})
		if err != nil {
			return errors.Annotate(err, "apply mutations").Err()
		}
		logging.Infof(ctx, "Finish writing %d mutations to spanner", len(ms))
	}
	return nil
}

func artifactUniquifier(testID, resultID, artifactID string) string {
	return fmt.Sprintf("%s/%s/%s", url.PathEscape(testID), resultID, url.PathEscape(artifactID))
}

func shardUniquifier(testID, resultID, artifactID string, shardID int) string {
	return fmt.Sprintf("%s/%s/%s/%d", url.PathEscape(testID), resultID, url.PathEscape(artifactID), shardID)
}

// queryTextArtifacts queries all text artifacts contained in an invocation.
// The query also join with TestResult table for test status.
// The content of the artifact is not populated in this function,
// this will keep the size of the slice small enough to fit in memory.
func (ae *artifactExporter) queryTextArtifacts(ctx context.Context, invID invocations.ID, project string, maxTotalArtifactSizeForInvocation int, checkpoints map[string]bool) (artifacts []*Artifact, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.queryTextArtifacts")
	defer func() { tracing.End(ts, err) }()

	statementStr := `
		SELECT
			tr.TestId,
			tr.ResultId,
			a.ArtifactId,
			a.ContentType,
			a.Size,
			a.RBECASHash,
			IFNULL(tr.Status, 0),
			tr.Variant,
			tr.VariantHash,
		FROM Artifacts a
		LEFT JOIN TestResults tr
		ON a.InvocationId = tr.InvocationId
		AND a.ParentId = CONCAT("tr/", tr.TestId , "/" , tr.ResultId)
		WHERE a.InvocationId=@invID
		ORDER BY tr.TestId, tr.ResultId, a.ArtifactId
	`
	st := spanner.NewStatement(statementStr)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invID": invID,
	})

	results := []*Artifact{}
	it := span.Query(ctx, st)
	var b spanutil.Buffer
	totalSize := 0
	err = it.Do(func(r *spanner.Row) error {
		a := &Artifact{InvocationID: string(invID)}
		err := b.FromSpanner(r, &a.TestID, &a.ResultID, &a.ArtifactID, &a.ContentType, &a.Size, &a.RBECASHash, &a.TestStatus, &a.TestVariant, &a.TestVariantHash)
		if err != nil {
			return errors.Annotate(err, "read row").Err()
		}
		// We skip all artifacts in checkpoints, because we have fully exported them.
		uq := artifactUniquifier(a.TestID, a.ResultID, a.ArtifactID)
		if _, ok := checkpoints[uq]; ok {
			return nil
		}

		// Update tsmon metrics.
		// Note: In case a task is retried, the metric may got double-counted.
		// As we don't expect many tasks to get retried, this should still provide
		// a fairly good approximation of the data.
		if a.Size > MaxArtifactSize {
			artifactExportCounter.Add(ctx, 1, project, "skipped_over_limit")
		}
		if a.ContentType == "" {
			artifactContentCounter.Add(ctx, 1, project, "empty")
		} else {
			if pbutil.IsTextArtifact(a.ContentType) {
				artifactContentCounter.Add(ctx, 1, project, "text")
			} else {
				artifactContentCounter.Add(ctx, 1, project, "nontext")
			}
		}

		if a.Size <= MaxArtifactSize && pbutil.IsTextArtifact(a.ContentType) {
			results = append(results, a)
			totalSize += int(a.Size)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "query artifact").Err()
	}
	if totalSize > maxTotalArtifactSizeForInvocation {
		logging.Warningf(ctx, "Total artifact size %d exceeds limit: %d", totalSize, MaxTotalArtifactSizeForInvocation)
		artifactInvocationCounter.Add(ctx, 1, project, "skipped_over_limit")
		return []*Artifact{}, nil
	}
	return results, nil
}

// downloadMutipleArtifactContent downloads multiple artifact content
// in parallel and creates TextArtifactRows for each rows.
// The rows will be pushed in rowC.
// Artifacts with content size smaller or equal than MaxRBECasBatchSize will be downloaded in batch.
// Artifacts with content size bigger than MaxRBECasBatchSize will be downloaded in streaming manner.
func (ae *artifactExporter) downloadMultipleArtifactContent(ctx context.Context, artifacts []*Artifact, inv *pb.Invocation, rowC chan *Row, maxShardContentSize, maxRBECasBatchSize int, checkpoints map[string]bool) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.downloadMultipleArtifactContent")
	defer func() { tracing.End(ts, err) }()

	logging.Infof(ctx, "Start downloading artifact contents")
	project, _ := realms.Split(inv.Realm)

	// Sort the artifacts by size, make it easier to do batching.
	sort.Slice(artifacts, func(i, j int) bool {
		return (artifacts[i].Size < artifacts[j].Size)
	})

	// Limit to 10 workers to avoid possible OOM.
	err = parallel.WorkPool(10, func(work chan<- func() error) {
		// Smaller artifacts should be downloaded in batch.
		logging.Infof(ctx, "Start downloading artifact contents in batch manner")
		currentBatchSize := 0
		batch := []*Artifact{}
		bigArtifactStartIndex := -1
		for i, artifact := range artifacts {
			if artifactSizeWithOverhead(artifact.Size) > maxRBECasBatchSize {
				bigArtifactStartIndex = i
				break
			}
			// Exceed batch limit, download whatever in batch.
			if currentBatchSize+artifactSizeWithOverhead(artifact.Size) > maxRBECasBatchSize {
				b := batch
				work <- func() error {
					err := ae.batchDownloadArtifacts(ctx, b, inv, rowC)
					if err != nil {
						return errors.Annotate(err, "batch download artifacts").Err()
					}
					return nil
				}
				// Reset
				currentBatchSize = 0
				batch = []*Artifact{}
			}
			batch = append(batch, artifact)
			// Add ArtifactRequestOverhead bytes for the overhead of the request.
			currentBatchSize += artifactSizeWithOverhead(artifact.Size)
		}

		// Download whatever left in batch.
		if len(batch) > 0 {
			b := batch
			work <- func() error {
				err := ae.batchDownloadArtifacts(ctx, b, inv, rowC)
				if err != nil {
					return errors.Annotate(err, "batch download artifacts").Err()
				}
				return nil
			}
		}
		logging.Infof(ctx, "Finished scheduling work to download artifact contents in batch manner")

		// Download the big artifacts in streaming manner.
		if bigArtifactStartIndex > -1 {
			numBigArtifacts := len(artifacts) - bigArtifactStartIndex
			logging.Infof(ctx, "Start downloading %d artifact contents in streaming manner", numBigArtifacts)
			for i := bigArtifactStartIndex; i < len(artifacts); i++ {
				artifact := artifacts[i]
				work <- func() error {
					err := ae.streamArtifactContent(ctx, artifact, inv, rowC, maxShardContentSize, checkpoints)
					if err != nil {
						// We don't want to retry on this error. Just log a warning.
						if errors.Is(err, ErrInvalidUTF8) {
							logging.Fields{
								"InvocationID": artifact.InvocationID,
								"TestID":       artifact.TestID,
								"ResultID":     artifact.ResultID,
								"ArtifactID":   artifact.ArtifactID,
							}.Warningf(ctx, "Test result artifact has invalid UTF-8")
							artifactExportCounter.Add(ctx, 1, project, "failure_input")
						} else {
							return errors.Annotate(err, "download artifact content inv_id = %q test id =%q result_id=%q artifact_id = %q", artifact.InvocationID, artifact.TestID, artifact.ResultID, artifact.ArtifactID).Err()
						}
					}
					return nil
				}
			}
			logging.Infof(ctx, "Finished scheduling work to download %d artifact contents in streaming manner", numBigArtifacts)
		}
	})
	logging.Infof(ctx, "Finished downloading artifact contents")
	return err
}

func artifactSizeWithOverhead(artifactSize int64) int {
	return int(artifactSize) + ArtifactRequestOverhead
}

// batchDownloadArtifacts downloads artifact content from RBE-CAS in batch.
// It generates one or more TextArtifactRows for the content and push in rowC.
func (ae *artifactExporter) batchDownloadArtifacts(ctx context.Context, batch []*Artifact, inv *pb.Invocation, rowC chan *Row) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.batchDownloadArtifacts")
	defer func() { tracing.End(ts, err) }()

	req := &repb.BatchReadBlobsRequest{InstanceName: ae.rbecasInstance}
	totalSize := 0
	for _, artifact := range batch {
		req.Digests = append(req.Digests, &repb.Digest{
			Hash:      artifacts.TrimHashPrefix(artifact.RBECASHash),
			SizeBytes: artifact.Size,
		})
		totalSize += int(artifact.Size)
	}
	logging.Infof(ctx, "Start batch download of %d artifacts. Total size = %d.", len(batch), totalSize)
	resp, err := ae.casClient.BatchReadBlobs(ctx, req)
	if err != nil {
		// The Invalid argument or ResourceExhausted suggest something is wrong with the
		// input, so we should not retry in this case.
		// ResourceExhausted may happen when we try to download more than the capacity.
		if grpcutil.Code(err) == codes.InvalidArgument {
			logging.Errorf(ctx, "BatchReadBlobs: invalid argument for invocation %q. Error: %v", inv.Name, err.Error())
			return nil
		}
		if grpcutil.Code(err) == codes.ResourceExhausted {
			logging.Errorf(ctx, "BatchReadBlobs: resource exhausted for invocation %q. Error: %v", inv.Name, err.Error())
			return nil
		}
		return errors.Annotate(err, "batch read blobs").Err()
	}
	project, realm := realms.Split(inv.Realm)
	for i, r := range resp.GetResponses() {
		artifact := batch[i]
		c := codes.Code(r.GetStatus().GetCode())
		loggingFields := logging.Fields{
			"InvocationID": artifact.InvocationID,
			"TestID":       artifact.TestID,
			"ResultID":     artifact.ResultID,
			"ArtifactID":   artifact.ArtifactID,
		}
		// It's no use to retry if we get those code.
		// We'll just log the error.
		if c == codes.InvalidArgument {
			loggingFields.Warningf(ctx, "Invalid artifact")
			artifactExportCounter.Add(ctx, 1, project, "failure_input")
			continue
		}
		if c == codes.NotFound {
			loggingFields.Warningf(ctx, "Not found artifact")
			artifactExportCounter.Add(ctx, 1, project, "failure_input")
			continue
		}

		// Perhaps something is wrong with RBE, we should retry.
		if c != codes.OK {
			loggingFields.Errorf(ctx, "Error downloading artifact. Code = %v message = %q", c, r.Status.GetMessage())
			return errors.Reason("downloading artifact").Err()
		}
		// Check data, make sure it is of valid UTF-8 format.
		if !utf8.Valid(r.Data) {
			loggingFields.Warningf(ctx, "Invalid UTF-8 content")
			artifactExportCounter.Add(ctx, 1, project, "failure_input")
			continue
		}

		variantJSON, err := pbutil.VariantToJSON(artifact.TestVariant)
		if err != nil {
			return errors.Annotate(err, "variant to json").Err()
		}
		invocationVariantHash := ""
		invocationVariantJSON := pbutil.EmptyJSON
		// Invocation variant should only be set for invocation level artifacts.
		if artifact.TestID == "" {
			invocationVariantJSON, err = pbutil.VariantToJSON(inv.TestResultVariantUnion)
			if err != nil {
				return errors.Annotate(err, "invocation variant union to json").Err()
			}
			if inv.TestResultVariantUnion != nil {
				invocationVariantHash = pbutil.VariantHash(inv.TestResultVariantUnion)
			}
		}
		// Everything is ok, send to rowC.
		// This is guaranteed to be small artifact, so we need only 1 shard.
		row := &bqpb.TextArtifactRow{
			Project:                    project,
			Realm:                      realm,
			InvocationId:               string(artifact.InvocationID),
			TestId:                     artifact.TestID,
			ResultId:                   artifact.ResultID,
			ArtifactId:                 artifact.ArtifactID,
			ContentType:                artifact.ContentType,
			Content:                    string(r.Data),
			ArtifactContentSize:        int32(artifact.Size),
			ShardContentSize:           int32(artifact.Size),
			TestStatus:                 testStatusToString(artifact.TestStatus),
			PartitionTime:              timestamppb.New(inv.CreateTime.AsTime()),
			TestVariant:                variantJSON,
			TestVariantHash:            artifact.TestVariantHash,
			InvocationVariantUnion:     invocationVariantJSON,
			InvocationVariantUnionHash: invocationVariantHash,
		}

		rowC <- &Row{
			content:     row,
			isLastShard: true,
		}
	}
	logging.Infof(ctx, "Finished batch download of %d artifacts", len(batch))
	return nil
}

// streamArtifactContent downloads artifact content from RBE-CAS in streaming manner.
// It generates one or more TextArtifactRows for the content and push in rowC.
func (ae *artifactExporter) streamArtifactContent(ctx context.Context, a *Artifact, inv *pb.Invocation, rowC chan *Row, maxShardContentSize int, checkpoints map[string]bool) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/artifactexporter.artifactExporter.streamArtifactContent")
	defer func() { tracing.End(ts, err) }()

	ac := artifactcontent.Reader{
		RBEInstance: ae.rbecasInstance,
		Hash:        a.RBECASHash,
		Size:        a.Size,
	}

	var str strings.Builder
	shardID := 0
	// shardUniquifier calculation may be expensive, so we only update it
	// whenever the shardID changes.
	suq := shardUniquifier(a.TestID, a.ResultID, a.ArtifactID, shardID)
	project, realm := realms.Split(inv.Realm)
	variantJSON, err := pbutil.VariantToJSON(a.TestVariant)
	if err != nil {
		return errors.Annotate(err, "variant to json").Err()
	}
	isLastShard := false
	invocationVariantHash := ""
	invocationVariantJSON := pbutil.EmptyJSON
	// Invocation variant should only be set for invocation level artifacts.
	if a.TestID == "" {
		invocationVariantJSON, err = pbutil.VariantToJSON(inv.TestResultVariantUnion)
		if err != nil {
			return errors.Annotate(err, "invocation variant union to json").Err()
		}
		if inv.TestResultVariantUnion != nil {
			invocationVariantHash = pbutil.VariantHash(inv.TestResultVariantUnion)
		}
	}

	input := func() *Row {
		return &Row{
			content: &bqpb.TextArtifactRow{
				Project:                    project,
				Realm:                      realm,
				InvocationId:               string(a.InvocationID),
				TestId:                     a.TestID,
				ResultId:                   a.ResultID,
				ArtifactId:                 a.ArtifactID,
				ShardId:                    int32(shardID),
				ContentType:                a.ContentType,
				Content:                    str.String(),
				ArtifactContentSize:        int32(a.Size),
				ShardContentSize:           int32(str.Len()),
				TestStatus:                 testStatusToString(a.TestStatus),
				TestVariant:                variantJSON,
				TestVariantHash:            a.TestVariantHash,
				InvocationVariantUnion:     invocationVariantJSON,
				InvocationVariantUnionHash: invocationVariantHash,
				PartitionTime:              timestamppb.New(inv.CreateTime.AsTime()),
				// We don't populated numshards here because we don't know
				// exactly how many shards we need until we finish scanning.
				// Still we are not sure if this field is useful (e.g. from bigquery, we can
				// just query for all artifacts and get the max shardID).
				NumShards: 0,
			},
			isLastShard: isLastShard,
		}
	}

	err = ac.DownloadRBECASContent(ctx, ae.bytestreamClient, func(ctx context.Context, pr io.Reader) error {
		sc := bufio.NewScanner(pr)

		// Read by runes, so we will know if the input contains invalid Unicode
		// character, so we can exit early.
		// The invalid characters will cause proto.Marshal to fail for the row,
		// so we don't support it.
		sc.Split(bufio.ScanRunes)

		for sc.Scan() {
			// Detect invalid rune. Just stop.
			r, _ := utf8.DecodeRune(sc.Bytes())
			if r == utf8.RuneError {
				return ErrInvalidUTF8
			}

			if str.Len()+len(sc.Bytes()) > maxShardContentSize {
				// Only export shards not having a checkpoint.
				if _, ok := checkpoints[suq]; !ok {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case rowC <- input():
					}
				}
				shardID++
				suq = shardUniquifier(a.TestID, a.ResultID, a.ArtifactID, shardID)
				str.Reset()
			}
			str.Write(sc.Bytes())
		}
		if err := sc.Err(); err != nil {
			return errors.Annotate(err, "scanner error").Err()
		}

		// Last shard should always be added to the list of shards to be exported,
		// because it should not have been exported before.
		isLastShard = true
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rowC <- input():
		}

		return nil
	})
	if err != nil {
		return errors.Annotate(err, "read artifact content").Err()
	}
	return nil
}

// throttleArtifactsForBQ limits the artifacts being to BigQuery based on percentage.
// It will allow us to roll out the feature slowly.
func throttleArtifactsForBQ(artifacts []*Artifact, percent int) ([]*Artifact, error) {
	results := []*Artifact{}
	for _, artifact := range artifacts {
		hashStr := fmt.Sprintf("%s%s", artifact.TestID, artifact.ArtifactID)
		hashVal := hash64([]byte(hashStr)) % 100
		// This is complement with the throttle function in BatchCreateArtifact.
		// For example, if percent = 2, only choose hashVal = 98, 99
		// This allows smoother roll out of the service.
		if (hashVal + uint64(percent)) >= 100 {
			results = append(results, artifact)
		}
	}
	return results, nil
}

// hash64 returns a hash value (uint64) for a given string.
func hash64(bt []byte) uint64 {
	hasher := fnv.New64a()
	hasher.Write(bt)
	return hasher.Sum64()
}

func testStatusToString(status pb.TestStatus) string {
	if status == pb.TestStatus_STATUS_UNSPECIFIED {
		return ""
	}
	return pb.TestStatus_name[int32(status)]
}

// shouldUploadToBQ returns true if we should upload artifacts to BigQuery.
func shouldUploadToBQ(ctx context.Context) (bool, error) {
	cfg, err := config.GetServiceConfig(ctx)
	if err != nil {
		return false, errors.Annotate(err, "get service config").Err()
	}
	return cfg.GetBqArtifactExporterServiceConfig().GetEnabled(), nil
}

// percentOfArtifactsToBQ returns how many percents of artifact to be uploaded.
// Return value is an integer between [0, 100].
func percentOfArtifactsToBQ(ctx context.Context) (int, error) {
	cfg, err := config.GetServiceConfig(ctx)
	if err != nil {
		return 0, errors.Annotate(err, "get service config").Err()
	}
	return int(cfg.GetBqArtifactExporterServiceConfig().GetExportPercent()), nil
}
