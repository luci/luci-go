// Copyright 2024 The LUCI Authors.
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

package bq

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter"
	pubsub "cloud.google.com/go/pubsub/apiv1"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	bqpb "go.chromium.org/luci/swarming/proto/bq"
)

const (
	fakeProject     = "proj"
	fakeDataset     = "ds"
	fakeTable       = "table"
	fullTableID     = "projects/proj/datasets/ds/tables/table"
	fullTopicID     = "projects/proj/topics/table"
	fakeWriteStream = fullTableID + "/streams/write-stream"
)

var testTime = testclock.TestRecentTimeUTC.Truncate(time.Microsecond)

func TestExportOpHappyPath(t *testing.T) {
	t.Parallel()

	ctx, grpc, bq, ps, op := setUp(t)
	defer grpc.close()
	defer op.Close(ctx)

	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if !bq.streamCommitted {
		t.Fatal("Did not commit the stream")
	}

	// Uploaded to BQ.
	if got := len(bq.rows); got != 10 {
		t.Fatalf("Expected 10 committed rows, but got %d", got)
	}
	expectedVal := int32(3)
	for idx, row := range bq.rows {
		val := &bqpb.TaskRequest{}
		if err := proto.Unmarshal(row, val); err != nil {
			t.Fatalf("Unmarshal error %s", err)
		}
		if val.Priority != expectedVal {
			t.Fatalf("Row %d: want %d, got %d", idx, expectedVal, val.Priority)
		}
		expectedVal++ // exported rows have sequential values
	}

	// Published to PubSub.
	if got := len(ps.messages); got != 10 {
		t.Fatalf("Expected 10 PubSub messages, but got %d", got)
	}
	expectedVal = int32(3)
	for idx, msg := range ps.messages {
		want := fmt.Sprintf("{\n  \"priority\": %d\n}", expectedVal)
		if msg != want {
			t.Fatalf("PubSub message %d: want %q, got %q", idx, want, msg)
		}
		expectedVal++ // exported rows have sequential values
	}

	state := exportState(ctx)
	if state == nil {
		t.Fatalf("ExportState unexpectedly missing")
	}
	if state.WriteStreamName != fakeWriteStream {
		t.Fatalf("Wrong WriteStreamName %q", state.WriteStreamName)
	}
	if !state.ExpireAt.Equal(testTime.Add(exportStateExpiry)) {
		t.Fatalf("Wrong ExpireAt %v", state.ExpireAt)
	}
}

func TestExportOpNothingToExport(t *testing.T) {
	t.Parallel()

	ctx, grpc, bq, _, op := setUp(t)
	defer grpc.close()
	defer op.Close(ctx)

	// Export a range with no events in it.
	if err := op.Execute(ctx, testTime.Add(30*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if got := len(bq.rows); got != 0 {
		t.Fatalf("Unexpected %d rows", got)
	}
	if state := exportState(ctx); state != nil {
		t.Fatalf("Unexpectedly present ExportState: %v", state)
	}
}

func TestExportOpMissingTable(t *testing.T) {
	t.Parallel()

	ctx, grpc, bq, _, op := setUp(t)
	defer grpc.close()
	defer op.Close(ctx)

	bq.tableMissing = true

	err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second)
	if err == nil || !tq.Fatal.In(err) {
		t.Fatalf("Expecting a fatal error, got %s", err)
	}
	if state := exportState(ctx); state == nil {
		t.Fatalf("Expected ExportState to exist, but it didn't")
	}
}

func TestExportOpRetryCommitted(t *testing.T) {
	t.Parallel()

	ctx, grpc, bq, _, op := setUp(t)
	defer grpc.close()
	defer op.Close(ctx)

	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	op.Close(ctx)

	// Uploaded some rows.
	rows := len(bq.rows)
	if rows == 0 {
		t.Fatal("No rows uploaded")
	}

	// Running the second time silently succeeds without any new exports.
	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if len(bq.rows) != rows {
		t.Fatalf("Unexpectedly exported more rows: %d", len(bq.rows))
	}
}

func TestExportOpRetryFinalized(t *testing.T) {
	t.Parallel()

	ctx, grpc, bq, _, op := setUp(t)
	defer grpc.close()
	defer op.Close(ctx)

	// Make commit fail with a transient error.
	bq.commitErr = status.Errorf(codes.Internal, "BOOM")

	err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second)
	if !transient.Tag.In(err) {
		t.Fatalf("Expected a transient error, got %s", err)
	}
	op.Close(ctx)

	// Uploaded some rows
	rows := len(bq.rows)
	if rows == 0 {
		t.Fatal("No rows uploaded")
	}
	// But didn't commit them.
	if bq.streamCommitted {
		t.Fatal("Stream unexpectedly committed already")
	}

	// Running the second time just commits the existing stream.
	bq.commitErr = nil
	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if len(bq.rows) != rows {
		t.Fatalf("Unexpectedly exported more rows: %d", len(bq.rows))
	}
}

// Helpers below.

func setUp(t *testing.T) (context.Context, *localGRPC, *bigQueryWrite, *pubSubPublisher, *ExportOp) {
	ctx, _ := testclock.UseTime(context.Background(), testTime)
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	// Create a bunch of test entities to export.
	type entity struct {
		ID int64 `gae:"$id"`
		TS time.Time
	}
	for i := 1; i < 20; i++ {
		_ = datastore.Put(ctx, &entity{
			ID: int64(i),
			TS: testTime.Add(time.Duration(i) * time.Second),
		})
	}

	g := newLocalGRPC()
	bq := &bigQueryWrite{t: t}
	ps := &pubSubPublisher{t: t}
	bq.register(g)
	ps.register(g)
	g.start(ctx)

	op := &ExportOp{
		BQClient:    bq.client(ctx, g),
		PSClient:    ps.client(ctx, g),
		OperationID: "operation-id",
		TableID:     fullTableID,
		Topic:       fullTopicID,
		Fetcher: &Fetcher[entity, *bqpb.TaskRequest]{
			entityKind:     "entity",
			timestampField: "TS",
			queryBatchSize: 300,
			convert: func(_ context.Context, ents []*entity) ([]*bqpb.TaskRequest, error) {
				out := make([]*bqpb.TaskRequest, len(ents))
				for i, ent := range ents {
					out[i] = &bqpb.TaskRequest{Priority: int32(ent.ID)}
				}
				return out, nil
			},
		},
		bqFlushThreshold: 6,
		psFlushThreshold: 6,
	}

	return ctx, g, bq, ps, op
}

// exportState returns the single ExportState if it exists.
func exportState(ctx context.Context) *ExportState {
	var ents []*ExportState
	if err := datastore.GetAll(ctx, datastore.NewQuery("bq.ExportState"), &ents); err != nil {
		panic(err)
	}
	switch len(ents) {
	case 0:
		return nil
	case 1:
		return ents[0]
	default:
		panic(fmt.Sprintf("too many ExportState entities: %v", ents))
	}
}

// localGPRC is a gRPC server and a client connected to it.
type localGRPC struct {
	listener *bufconn.Listener
	server   *grpc.Server
	conn     *grpc.ClientConn
}

func newLocalGRPC() *localGRPC {
	return &localGRPC{
		listener: bufconn.Listen(100 * 1024),
		server:   grpc.NewServer(),
	}
}

func (l *localGRPC) start(ctx context.Context) {
	go func() { _ = l.server.Serve(l.listener) }()
	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return l.listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to dial gRPC: %s", err))
	}
	l.conn = conn
}

func (l *localGRPC) close() {
	_ = l.listener.Close()
	l.server.Stop()
}

// BigQuery mock.

type bigQueryWrite struct {
	storagepb.UnimplementedBigQueryWriteServer

	t *testing.T

	m               sync.Mutex
	streamExists    bool
	streamFinalized bool
	streamCommitted bool
	rows            [][]byte
	commitErr       error
	tableMissing    bool
}

func (w *bigQueryWrite) register(g *localGRPC) {
	storagepb.RegisterBigQueryWriteServer(g.server, w)
}

func (w *bigQueryWrite) client(ctx context.Context, g *localGRPC) *managedwriter.Client {
	client, err := managedwriter.NewClient(ctx, fakeProject, option.WithGRPCConn(g.conn))
	if err != nil {
		panic(fmt.Sprintf("failed to create BQ client: %s", err))
	}
	return client
}

func (w *bigQueryWrite) CreateWriteStream(_ context.Context, req *storagepb.CreateWriteStreamRequest) (*storagepb.WriteStream, error) {
	w.m.Lock()
	defer w.m.Unlock()

	expected := &storagepb.CreateWriteStreamRequest{
		Parent: fullTableID,
		WriteStream: &storagepb.WriteStream{
			Type: storagepb.WriteStream_PENDING,
		},
	}
	if !proto.Equal(req, expected) {
		w.t.Fatalf("Wrong CreateWriteStreamRequest: %s", req)
	}

	w.streamExists = true
	return &storagepb.WriteStream{
		Name: fakeWriteStream,
		Type: storagepb.WriteStream_PENDING,
	}, nil
}

func (w *bigQueryWrite) AppendRows(stream storagepb.BigQueryWrite_AppendRowsServer) error {
	for {
		in, err := stream.Recv()
		switch {
		case err == io.EOF:
			return nil
		case err != nil:
			return err
		}

		w.m.Lock()
		for _, row := range in.Rows.(*storagepb.AppendRowsRequest_ProtoRows).ProtoRows.Rows.SerializedRows {
			w.rows = append(w.rows, append([]byte(nil), row...))
		}
		rowCount := len(w.rows)
		w.m.Unlock()

		resp := &storagepb.AppendRowsResponse{
			Response: &storagepb.AppendRowsResponse_AppendResult_{
				AppendResult: &storagepb.AppendRowsResponse_AppendResult{
					Offset: &wrapperspb.Int64Value{
						Value: int64(rowCount),
					},
				},
			},
			WriteStream: fakeWriteStream,
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func (w *bigQueryWrite) GetWriteStream(_ context.Context, req *storagepb.GetWriteStreamRequest) (*storagepb.WriteStream, error) {
	w.m.Lock()
	defer w.m.Unlock()

	if req.Name != fakeWriteStream {
		return nil, status.Errorf(codes.NotFound, "unknown write stream")
	}
	if !w.streamExists {
		return nil, status.Errorf(codes.NotFound, "doesn't exist yet")
	}

	resp := &storagepb.WriteStream{
		Name: fakeWriteStream,
		Type: storagepb.WriteStream_PENDING,
	}
	if w.streamCommitted {
		resp.CommitTime = timestamppb.New(testTime)
	}
	return resp, nil
}

func (w *bigQueryWrite) FinalizeWriteStream(_ context.Context, req *storagepb.FinalizeWriteStreamRequest) (*storagepb.FinalizeWriteStreamResponse, error) {
	w.m.Lock()
	defer w.m.Unlock()

	expected := &storagepb.FinalizeWriteStreamRequest{Name: fakeWriteStream}
	if !proto.Equal(req, expected) {
		w.t.Fatalf("Wrong FinalizeWriteStreamRequest: %s", req)
	}

	w.streamFinalized = true
	return &storagepb.FinalizeWriteStreamResponse{
		RowCount: int64(len(w.rows)),
	}, nil
}

func (w *bigQueryWrite) BatchCommitWriteStreams(_ context.Context, req *storagepb.BatchCommitWriteStreamsRequest) (*storagepb.BatchCommitWriteStreamsResponse, error) {
	w.m.Lock()
	defer w.m.Unlock()

	if w.commitErr != nil {
		return nil, w.commitErr
	}

	expected := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       fullTableID,
		WriteStreams: []string{fakeWriteStream},
	}
	if !proto.Equal(req, expected) {
		w.t.Fatalf("Wrong BatchCommitWriteStreamsRequest: %s", req)
	}
	if !w.streamFinalized {
		w.t.Fatal("Committing unfinalized stream")
	}

	w.streamCommitted = true

	if w.tableMissing {
		return &storagepb.BatchCommitWriteStreamsResponse{
			StreamErrors: []*storagepb.StorageError{
				{Code: storagepb.StorageError_TABLE_NOT_FOUND},
			},
		}, nil
	}

	return &storagepb.BatchCommitWriteStreamsResponse{
		CommitTime: timestamppb.New(testTime),
	}, nil
}

// PubSub mock.

type pubSubPublisher struct {
	pubsubpb.UnimplementedPublisherServer

	t *testing.T

	m        sync.Mutex
	messages []string
}

func (p *pubSubPublisher) register(g *localGRPC) {
	pubsubpb.RegisterPublisherServer(g.server, p)
}

func (p *pubSubPublisher) client(ctx context.Context, g *localGRPC) *pubsub.PublisherClient {
	client, err := pubsub.NewPublisherClient(ctx, option.WithGRPCConn(g.conn))
	if err != nil {
		panic(fmt.Sprintf("failed to create PubSub client: %s", err))
	}
	return client
}

func (p *pubSubPublisher) Publish(_ context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(req.Messages) == 0 {
		p.t.Fatalf("Empty Publish request")
	}
	for _, msg := range req.Messages {
		p.messages = append(p.messages, string(msg.Data))
	}
	return &pubsubpb.PublishResponse{}, nil
}
