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
)

const (
	fakeProject     = "proj"
	fakeDataset     = "ds"
	fakeTable       = "table"
	fullTableID     = "projects/proj/datasets/ds/tables/table"
	fakeWriteStream = fullTableID + "/streams/write-stream"
)

var testTime = testclock.TestRecentTimeUTC.Truncate(time.Microsecond)

func TestExportOpHappyPath(t *testing.T) {
	t.Parallel()

	ctx, bq, op := setUp(t)
	defer bq.close()
	defer op.Close(ctx)

	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if !bq.bqw.streamCommitted {
		t.Fatal("Did not commit the stream")
	}
	if got := len(bq.bqw.rows); got != 10 {
		t.Fatalf("Expected 10 committed rows, but got %d", got)
	}

	expectedVal := int64(3)
	for idx, row := range bq.bqw.rows {
		val := &wrapperspb.Int64Value{}
		if err := proto.Unmarshal(row, val); err != nil {
			t.Fatalf("Unmarshal error %s", err)
		}
		if val.Value != expectedVal {
			t.Fatalf("Row %d: want %d, got %d", idx, expectedVal, val.Value)
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

	ctx, bq, op := setUp(t)
	defer bq.close()
	defer op.Close(ctx)

	// Export a range with no events in it.
	if err := op.Execute(ctx, testTime.Add(30*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if got := len(bq.bqw.rows); got != 0 {
		t.Fatalf("Unexpected %d rows", got)
	}
	if state := exportState(ctx); state != nil {
		t.Fatalf("Unexpectedly present ExportState: %v", state)
	}
}

func TestExportOpMissingTable(t *testing.T) {
	t.Parallel()

	ctx, bq, op := setUp(t)
	defer bq.close()
	defer op.Close(ctx)

	bq.bqw.tableMissing = true

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

	ctx, bq, op := setUp(t)
	defer bq.close()
	defer op.Close(ctx)

	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	op.Close(ctx)

	// Uploaded some rows.
	rows := len(bq.bqw.rows)
	if rows == 0 {
		t.Fatal("No rows uploaded")
	}

	// Running the second time silently succeeds without any new exports.
	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if len(bq.bqw.rows) != rows {
		t.Fatalf("Unexpectedly exported more rows: %d", len(bq.bqw.rows))
	}
}

func TestExportOpRetryFinalized(t *testing.T) {
	t.Parallel()

	ctx, bq, op := setUp(t)
	defer bq.close()
	defer op.Close(ctx)

	// Make commit fail with a transient error.
	bq.bqw.commitErr = status.Errorf(codes.Internal, "BOOM")

	err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second)
	if !transient.Tag.In(err) {
		t.Fatalf("Expected a transient error, got %s", err)
	}
	op.Close(ctx)

	// Uploaded some rows
	rows := len(bq.bqw.rows)
	if rows == 0 {
		t.Fatal("No rows uploaded")
	}
	// But didn't commit them.
	if bq.bqw.streamCommitted {
		t.Fatal("Stream unexpectedly committed already")
	}

	// Running the second time just commits the existing stream.
	bq.bqw.commitErr = nil
	if err := op.Execute(ctx, testTime.Add(3*time.Second), 10*time.Second); err != nil {
		t.Fatalf("%s", err)
	}
	if len(bq.bqw.rows) != rows {
		t.Fatalf("Unexpectedly exported more rows: %d", len(bq.bqw.rows))
	}
}

// Helpers below.

func setUp(t *testing.T) (context.Context, *mockedBQ, *ExportOp) {
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

	bq := mockBQ(ctx, t)

	op := &ExportOp{
		Client:      bq.client,
		OperationID: "operation-id",
		TableID:     fullTableID,
		Fetcher: &Fetcher[entity, *wrapperspb.Int64Value]{
			entityKind:     "entity",
			timestampField: "TS",
			queryBatchSize: 300,
			convert: func(_ context.Context, ents []*entity) ([]*wrapperspb.Int64Value, error) {
				out := make([]*wrapperspb.Int64Value, len(ents))
				for i, ent := range ents {
					out[i] = &wrapperspb.Int64Value{Value: ent.ID}
				}
				return out, nil
			},
		},
		bqFlushThreshold: 6,
	}

	return ctx, bq, op
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

// mockedBQ is a mocked BQ gRPC server and a client connected to it.
type mockedBQ struct {
	listener *bufconn.Listener
	server   *grpc.Server
	client   *managedwriter.Client
	bqw      *bigQueryWrite
}

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

func mockBQ(ctx context.Context, t *testing.T) *mockedBQ {
	listener := bufconn.Listen(100 * 1024)
	server := grpc.NewServer()
	bqw := &bigQueryWrite{t: t}

	storagepb.RegisterBigQueryWriteServer(server, bqw)

	go func() { _ = server.Serve(listener) }()

	conn, err := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to dial gRPC: %s", err))
	}

	client, err := managedwriter.NewClient(ctx, fakeProject, option.WithGRPCConn(conn))
	if err != nil {
		panic(fmt.Sprintf("failed to create BQ client: %s", err))
	}

	return &mockedBQ{
		listener: listener,
		server:   server,
		client:   client,
		bqw:      bqw,
	}
}

func (m *mockedBQ) close() {
	_ = m.client.Close()
	_ = m.listener.Close()
	m.server.Stop()
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
