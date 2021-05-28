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

package bqlog

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
	codepb "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/grpc/grpcutil"
)

const (
	// Roughly a limit on a size of a single AppendRows message.
	batchSizeMaxBytes = 5 * 1024 * 1024
	// How long to wait by default before sending an incomplete batch.
	defaultBatchAgeMax = 5 * time.Second
	// How many bytes to buffer by default before starting dropping excesses.
	defaultMaxLiveSizeBytes = 50 * 1024 * 1024
)

var (
	metricSentCounter = metric.NewCounter(
		"bqlog/sent",
		"Count of log entries successfully sent",
		nil,
		field.String("type"), // proto message type
	)
	metricDroppedCounter = metric.NewCounter(
		"bqlog/dropped",
		"Count of log entries dropped for various reasons",
		nil,
		field.String("type"),   // proto message type
		field.String("reason"), // reason of why it was dropped if known
	)
	metricErrorsCounter = metric.NewCounter(
		"bqlog/errors",
		"Count of encountered RPC errors",
		nil,
		field.String("type"), // proto message type
		field.String("code"), // canonical gRPC code (as string) if known
	)
)

// Bundler buffers logs in memory before sending them to BigQuery.
type Bundler struct {
	CloudProject string // the cloud project with the dataset, required
	Dataset      string // the BQ dataset with log tables, required

	m        sync.RWMutex
	bufs     map[protoreflect.FullName]*logBuffer
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	draining bool
}

// LoggedType describes where and how to log proto messages of the given type.
type LoggedType struct {
	Prototype        proto.Message // used only for its type descriptor, required
	Table            string        // the BQ table name within the bundler's dataset, required
	BatchAgeMax      time.Duration // for how long to buffer message (or 0 for some default)
	MaxLiveSizeBytes int           // approximate limit on memory buffer size (or 0 for some default)
}

// RegisterLoggedType tells the bundler where and how to log messages of some
// concrete proto type.
//
// Must be called before the bundler is running. Can be called during the init()
// time.
func (b *Bundler) RegisterLoggedType(t LoggedType) {
	if t.BatchAgeMax == 0 {
		t.BatchAgeMax = defaultBatchAgeMax
	}
	if t.MaxLiveSizeBytes == 0 {
		t.MaxLiveSizeBytes = defaultMaxLiveSizeBytes
	} else if t.MaxLiveSizeBytes < batchSizeMaxBytes {
		t.MaxLiveSizeBytes = batchSizeMaxBytes
	}

	b.m.Lock()
	defer b.m.Unlock()

	if b.running {
		panic("the bundler is already running")
	}

	typ := proto.MessageName(t.Prototype)
	if b.bufs[typ] != nil {
		panic(fmt.Sprintf("message type %q was already registered with RegisterLoggedType", typ))
	}
	if b.bufs == nil {
		b.bufs = make(map[protoreflect.FullName]*logBuffer, 1)
	}
	b.bufs[typ] = &logBuffer{
		t:    t,
		desc: protodesc.ToDescriptorProto(t.Prototype.ProtoReflect().Descriptor()),
	}
}

// Log asynchronously logs the given message to a BQ table associated with
// the message type via a prior RegisterLoggedType call.
//
// This is a best effort operation (and thus returns no error).
//
// Messages are dropped when:
//  * Writes to BigQuery are failing with a fatal error:
//    - The table doesn't exist.
//    - The table has an incompatible schema.
//    - The server account has no permission to write to the table.
//    - Etc.
//  * The server crashes before it manages to flush buffered logs.
//  * The internal flush buffer is full (per MaxLiveSizeBytes).
//
// Panics if `m` was not registered via RegisterLoggedType or if the bundler is
// not running yet.
func (b *Bundler) Log(ctx context.Context, m proto.Message) {
	typ := proto.MessageName(m)
	blob, err := proto.Marshal(m)
	if err != nil {
		recordDrop(ctx, typ, 1, err, "MARSHAL_ERROR")
		return
	}

	b.m.RLock()
	defer b.m.RUnlock()

	if !b.running {
		panic("the bundler is not running yet")
	}
	if b.draining {
		recordDrop(ctx, typ, 1, errors.New("draining already"), "DRAINING")
		return
	}

	buf := b.bufs[typ]
	if buf == nil {
		panic(fmt.Sprintf("message type %q was not registered with RegisterLoggedType", typ))
	}

	select {
	case buf.disp.C <- blob:
	case <-ctx.Done():
		recordDrop(ctx, typ, 1, ctx.Err(), "CONTEXT_DEADLINE")
	}
}

// Start launches the bundler internal goroutines.
//
// Canceling the context with cease all bundler activities. To gracefully
// shutdown the bundler (e.g. by flushing all pending logs) use Shutdown.
func (b *Bundler) Start(ctx context.Context, w BigQueryWriter) {
	b.m.Lock()
	defer b.m.Unlock()
	if b.running {
		panic("the bundler is already running")
	}
	b.running = true
	b.ctx, b.cancel = context.WithCancel(ctx)
	for typ, buf := range b.bufs {
		bufCtx := loggingFields(b.ctx, typ)
		buf.start(bufCtx, &logSender{
			ctx:  bufCtx,
			w:    w,
			typ:  typ,
			desc: buf.desc,
			dest: fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", b.CloudProject, b.Dataset, buf.t.Table),
		})
	}
}

// Shutdown flushes pending logs and closes streaming RPCs.
//
// Does nothing if the bundler wasn't running. Gives up waiting for all data to
// be flushed (and drops it) after 15s timeout or when `ctx` is canceled.
func (b *Bundler) Shutdown(ctx context.Context) {
	var drained []chan struct{}

	b.m.Lock()
	if b.running && !b.draining {
		b.draining = true
		for typ, buf := range b.bufs {
			drained = append(drained, buf.drain(loggingFields(ctx, typ)))
		}
		// Totally shutdown everything after some deadline by canceling the root
		// bundler context. It should cause all dispatcher.Channels to give up on
		// any retries they might be doing.
		cancel := b.cancel
		go func() {
			<-clock.After(ctx, 15*time.Second)
			cancel()
		}()
	}
	b.m.Unlock()

	for _, ch := range drained {
		<-ch
	}
}

////////////////////////////////////////////////////////////////////////////////

func loggingFields(ctx context.Context, typ protoreflect.FullName) context.Context {
	return logging.SetFields(ctx, logging.Fields{
		"activity": "luci.bqlog",
		"type":     string(typ),
	})
}

func recordSent(ctx context.Context, typ protoreflect.FullName, count int) {
	metricSentCounter.Add(ctx, int64(count), string(typ))
}

func recordDrop(ctx context.Context, typ protoreflect.FullName, count int, err error, reason string) {
	ctx = loggingFields(ctx, typ)
	if err != nil {
		logging.Errorf(ctx, "Dropped %d %q: %s: %s", count, typ, reason, err)
	} else {
		logging.Errorf(ctx, "Dropped %d %q: %s", count, typ, reason)
	}
	metricDroppedCounter.Add(ctx, int64(count), string(typ), reason)
}

func recordErr(ctx context.Context, typ protoreflect.FullName, count int, err error) {
	if transient.Tag.In(err) {
		logging.Warningf(ctx, "Transient error when sending %d items: %s", count, err)
	} else {
		logging.Errorf(ctx, "Fatal error when sending %d items: %s", count, err)
	}
	codeStr := "UNKNOWN"
	if code := grpcutil.Code(err); code != codes.Unknown {
		if codeStr, _ = codepb.Code_name[int32(code)]; codeStr == "" {
			codeStr = fmt.Sprintf("CODE_%d", code)
		}
	} else if errors.Contains(err, io.EOF) {
		codeStr = "EOF"
	}
	metricErrorsCounter.Add(ctx, 1, string(typ), codeStr)
}

////////////////////////////////////////////////////////////////////////////////

type logBuffer struct {
	t      LoggedType
	desc   *descriptorpb.DescriptorProto
	sender *logSender
	disp   dispatcher.Channel
}

func (b *logBuffer) start(ctx context.Context, sender *logSender) {
	b.sender = sender

	opts := dispatcher.Options{
		ItemSizeFunc: func(itm interface{}) int { return len(itm.([]byte)) },
		DropFn: func(data *buffer.Batch, flush bool) {
			if data != nil {
				recordDrop(ctx, proto.MessageName(b.t.Prototype), len(data.Data), nil, "DISPATCHER")
			}
		},
		ErrorFn: func(data *buffer.Batch, err error) (retry bool) {
			recordErr(ctx, proto.MessageName(b.t.Prototype), len(data.Data), err)
			return transient.Tag.In(err)
		},
		Buffer: buffer.Options{
			MaxLeases:     1,  // there can be only one outstanding write per an RPC stream
			BatchItemsMax: -1, // cut batches based on size
			BatchSizeMax:  batchSizeMaxBytes,
			BatchAgeMax:   b.t.BatchAgeMax,
			FullBehavior: &buffer.DropOldestBatch{
				MaxLiveSize: b.t.MaxLiveSizeBytes,
			},
		},
	}

	var err error
	b.disp, err = dispatcher.NewChannel(ctx, &opts, sender.send)
	if err != nil {
		panic(fmt.Sprintf("failed to start the dispatcher: %s", err)) // should not be happening
	}
}

func (b *logBuffer) drain(ctx context.Context) chan struct{} {
	logging.Debugf(ctx, "Draining...")

	drained := make(chan struct{})

	b.disp.Close()
	go func() {
		// Wait until the dispatcher channel is drained into the gRPC sender.
		select {
		case <-ctx.Done():
		case <-b.disp.DrainC:
		}

		// Wait until the pending gRPC data is flushed.
		b.sender.stop()

		logging.Debugf(ctx, "Drained")
		close(drained)
	}()

	return drained
}

////////////////////////////////////////////////////////////////////////////////

type logSender struct {
	ctx  context.Context
	w    BigQueryWriter
	typ  protoreflect.FullName
	desc *descriptorpb.DescriptorProto
	dest string

	m      sync.Mutex
	stream storagepb.BigQueryWrite_AppendRowsClient
}

func (s *logSender) send(data *buffer.Batch) (rerr error) {
	// There allowed only one concurrent Send or CloseSend per a gRPC stream.
	s.m.Lock()
	defer s.m.Unlock()

	// Open the gRPC stream if have none yet.
	opened := false
	if s.stream == nil {
		stream, err := s.w.AppendRows(s.ctx)
		if err != nil {
			return grpcutil.WrapIfTransient(err)
		}
		opened = true
		s.stream = stream
	}

	// Prepare the request.
	protoData := &storagepb.AppendRowsRequest_ProtoData{
		Rows: &storagepb.ProtoRows{
			SerializedRows: make([][]byte, len(data.Data)),
		},
	}
	for i, row := range data.Data {
		protoData.Rows.SerializedRows[i] = row.Item.([]byte)
	}

	// WriterSchema field is necessary only in the first request.
	if opened {
		protoData.WriterSchema = &storagepb.ProtoSchema{
			ProtoDescriptor: s.desc,
		}
	}

	err := s.stream.Send(&storagepb.AppendRowsRequest{
		WriteStream: s.dest,
		Rows: &storagepb.AppendRowsRequest_ProtoRows{
			ProtoRows: protoData,
		},
	})

	// If the stream was aborted, properly shut it all down so we can try again
	// later with a new stream.
	if err != nil {
		s.stream.CloseSend()
		s.stream = nil
		return errors.Annotate(err, "failed to send data to BQ").Tag(transient.Tag).Err()
	}

	// Otherwise try to read the acknowledgment from the server. We need to wait
	// for it to make sure it is OK to "forget" about the `data` batch.
	resp, err := s.stream.Recv()

	// If there's a gRPC-level error, it means the connection is broken and should
	// be abandoned.
	if err != nil {
		s.stream = nil
		if err == io.EOF {
			return errors.Annotate(err, "server unexpected closed the connection").Tag(transient.Tag).Err()
		}
		return grpcutil.WrapIfTransient(err)
	}

	// If the overall connection is fine, but the latest push specifically was
	// rejected, the error is in `resp`.
	if sts := resp.GetError(); sts != nil {
		return grpcutil.WrapIfTransient(status.ErrorProto(sts))
	}

	recordSent(s.ctx, s.typ, len(data.Data))
	return nil
}

func (s *logSender) stop() {
	s.m.Lock()
	defer s.m.Unlock()
	if s.stream != nil {
		s.stream.CloseSend()
		s.stream.Recv() // wait for ACK
		s.stream = nil
	}
}
