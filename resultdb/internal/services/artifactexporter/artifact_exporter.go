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
	"strings"
	"unicode/utf8"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/bqutil"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxShardContentSize is the maximum content size in BQ row.
// Artifacts content bigger than this size needs to be sharded.
// Leave 10 KB for other fields, the rest is content.
const MaxShardContentSize = bqutil.RowMaxBytes - 10*1024

type Artifact struct {
	InvocationID string
	TestID       string
	ResultID     string
	ArtifactID   string
	ContentType  string
	Size         int64
	RBECASHash   string
	TestStatus   pb.TestStatus
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

// Options is artifact exporter configuration.
type Options struct {
	// ArtifactRBEInstance is the name of the RBE instance to use for artifact
	// storage. Example: "projects/luci-resultdb/instances/artifacts".
	ArtifactRBEInstance string
}

type artifactExporter struct {
	rbecasInstance string
	// Client to read from RBE-CAS.
	rbecasClient bytestream.ByteStreamClient
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

	srv.RegisterCleanup(func(ctx context.Context) {
		err := conn.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up artifact RBE connection: %s", err)
		}
	})

	ae := &artifactExporter{
		rbecasInstance: opts.ArtifactRBEInstance,
		rbecasClient:   bytestream.NewByteStreamClient(conn),
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
func (ae *artifactExporter) exportArtifacts(ctx context.Context, invID invocations.ID) error {
	logging.Infof(ctx, "Exporting artifacts for invocation ID %s", invID)
	shouldUpload, err := shouldUploadToBQ(ctx)
	if err != nil {
		return errors.Annotate(err, "getting config").Err()
	}
	if !shouldUpload {
		logging.Infof(ctx, "Uploading to BigQuery is disabled")
		return nil
	}

	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Get the invocation.
	inv, err := invocations.Read(ctx, invID)
	if err != nil {
		return errors.Annotate(err, "error reading exported invocation").Err()
	}
	if inv.State != pb.Invocation_FINALIZED {
		return errors.Reason("invocation not finalized").Err()
	}

	// Query for artifacts.
	artifacts, err := ae.queryTextArtifacts(ctx, invID)
	if err != nil {
		return errors.Annotate(err, "query text artifacts").Err()
	}

	logging.Infof(ctx, "Found %d text artifacts for invocation %v", len(artifacts), invID)

	percent, err := percentOfArtifactsToBQ(ctx)
	if err != nil {
		return errors.Annotate(err, "read percent from config").Err()
	}
	artifacts, err = throttleArtifactsForBQ(artifacts, percent)
	if err != nil {
		return errors.Annotate(err, "throttle artifacts for bq").Err()
	}

	// Get artifact content.
	// Channel to push the artifact rows.
	// In theory, some artifact content may be too big to
	// fit into memory (we allow artifacts upto 640MB [1]).
	// We need to "break" them in to sizeable chunks.
	// [1] https://source.corp.google.com/h/chromium/infra/infra_superproject/+/main:data/k8s/projects/luci-resultdb/resultdb.star;l=307?q=max-artifact-content-stream-length
	rowC := make(chan *bqpb.TextArtifactRow, 10)

	err = parallel.FanOutIn(func(work chan<- func() error) {
		// Download artifacts content and put the rows in a channel.
		work <- func() error {
			defer close(rowC)
			err := ae.downloadMultipleArtifactContent(ctx, artifacts, inv, rowC, MaxShardContentSize)
			if err != nil {
				return errors.Annotate(err, "download multiple artifact content").Err()
			}
			return nil
		}
		// Upload the rows to BQ.
		work <- func() error {
			for range rowC {
				// TODO (nqmtuan): Group rows to reasonable size and send to BQ default stream.
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
			}
			return nil
		}
	})

	return err
}

// queryTextArtifacts queries all text artifacts contained in an invocation.
// The query also join with TestResult table for test status.
// The content of the artifact is not populated in this function,
// this will keep the size of the slice small enough to fit in memory.
func (ae *artifactExporter) queryTextArtifacts(ctx context.Context, invID invocations.ID) ([]*Artifact, error) {
	st := spanner.NewStatement(`
		SELECT
			tr.TestId,
			tr.ResultId,
			a.ArtifactId,
			a.ContentType,
			a.Size,
			a.RBECASHash,
			IFNULL(tr.Status, 0)
		FROM Artifacts a
		LEFT JOIN TestResults tr
		ON a.InvocationId = tr.InvocationId
		AND a.ParentId = CONCAT("tr/", tr.TestId , "/" , tr.ResultId)
		WHERE a.InvocationId=@invID
		AND REGEXP_CONTAINS(IFNULL(a.ContentType, ""), @contentTypeRegexp)
		ORDER BY tr.TestId, tr.ResultId, a.ArtifactId
		`)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invID":             invID,
		"contentTypeRegexp": "text/.*",
	})

	results := []*Artifact{}
	it := span.Query(ctx, st)
	var b spanutil.Buffer
	err := it.Do(func(r *spanner.Row) error {
		a := &Artifact{InvocationID: string(invID)}
		err := b.FromSpanner(r, &a.TestID, &a.ResultID, &a.ArtifactID, &a.ContentType, &a.Size, &a.RBECASHash, &a.TestStatus)
		if err != nil {
			return errors.Annotate(err, "read row").Err()
		}
		results = append(results, a)
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "query artifact").Err()
	}
	return results, nil
}

// downloadMutipleArtifactContent downloads multiple artifact content
// in parallel and creates TextArtifactRows for each rows.
// The rows will be pushed in rowC.
func (ae *artifactExporter) downloadMultipleArtifactContent(ctx context.Context, artifacts []*Artifact, inv *pb.Invocation, rowC chan *bqpb.TextArtifactRow, maxShardContentSize int) error {
	return parallel.FanOutIn(func(work chan<- func() error) {
		for _, artifact := range artifacts {
			artifact := artifact
			work <- func() error {
				err := ae.downloadArtifactContent(ctx, artifact, inv, rowC, maxShardContentSize)
				if err != nil {
					// We don't want to retry on this error. Just log a warning.
					if errors.Is(err, ErrInvalidUTF8) {
						logging.Fields{
							"InvocationID": artifact.InvocationID,
							"TestID":       artifact.TestID,
							"ResultID":     artifact.ResultID,
							"ArtifactID":   artifact.ArtifactID,
						}.Warningf(ctx, "Test result artifact has invalid UTF-8")
					} else {
						return errors.Annotate(err, "download artifact content inv_id = %q test id =%q result_id=%q artifact_id = %q", artifact.InvocationID, artifact.TestID, artifact.ResultID, artifact.ArtifactID).Err()
					}
				}
				return nil
			}
		}
	})
}

// downloadArtifactContent downloads artifact content from RBE-CAS.
// It generates one or more TextArtifactRows for the content and push in rowC.
func (ae *artifactExporter) downloadArtifactContent(ctx context.Context, a *Artifact, inv *pb.Invocation, rowC chan *bqpb.TextArtifactRow, maxShardContentSize int) error {
	ac := artifactcontent.Reader{
		RBEInstance: ae.rbecasInstance,
		Hash:        a.RBECASHash,
		Size:        a.Size,
	}

	var str strings.Builder
	shardID := 0
	project, realm := realms.Split(inv.Realm)
	input := func() *bqpb.TextArtifactRow {
		return &bqpb.TextArtifactRow{
			Project:             project,
			Realm:               realm,
			InvocationId:        string(a.InvocationID),
			TestId:              a.TestID,
			ResultId:            a.ResultID,
			ArtifactId:          a.ArtifactID,
			ShardId:             int32(shardID),
			ContentType:         a.ContentType,
			Content:             str.String(),
			ArtifactContentSize: int32(a.Size),
			ShardContentSize:    int32(str.Len()),
			TestStatus:          testStatusToString(a.TestStatus),
			PartitionTime:       timestamppb.New(inv.CreateTime.AsTime()),
			ArtifactShard:       fmt.Sprintf("%s:%d", a.ArtifactID, shardID),
			// We don't populated numshards here because we don't know
			// exactly how many shards we need until we finish scanning.
			// Still we are not sure if this field is useful (e.g. from bigquery, we can
			// just query for all artifacts and get the max shardID).
			NumShards: 0,
		}
	}

	err := ac.DownloadRBECASContent(ctx, ae.rbecasClient, func(ctx context.Context, pr io.Reader) error {
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
				select {
				case <-ctx.Done():
					return ctx.Err()
				case rowC <- input():
				}
				shardID++
				str.Reset()
			}
			str.Write(sc.Bytes())
		}
		if err := sc.Err(); err != nil {
			return errors.Annotate(err, "scanner error").Err()
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
	if err != nil {
		return errors.Annotate(err, "read artifact content").Err()
	}
	return err
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
