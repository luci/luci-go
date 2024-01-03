// Copyright 2023 The LUCI Authors.
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

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
)

type resultWaiter func(ctx context.Context) error

type streamClient interface {
	appendRows(ctx context.Context, rows [][]byte) (resultWaiter, error)
	streamName() string
	finalize(ctx context.Context) error
}

// bqWriter is a stateful client which handles writes to bigquery.
// This object is intended to be used for single thread exports.
type bqWriter struct {
	// maxSizePerWrite is a configurable number of bytes per appendRows call
	// the maxium number of bytes allowed is 10_000_000 but this is configurable
	// for unit test purposes.
	maxSizePerWrite int
	stream          streamClient
	results         *errgroup.Group
}

// writeProtos will write msgs using stream . It will break up input into
// multiple AppendRows calls if it detects that the byte size of msgs will
// exceed the maximum number of bytes per AppendRows call.
// Each call to appendRows will add the managedwriter.AppendRowsResult to
// results of the bqWriter so that they can be remembered for later.
// As soon as the AppendRows is called on all of msgs this will exit.
// writeProtos is guaranteed to call AppendRows at least once so long as msgs is
// non-empty.
func (w *bqWriter) writeProtos(ctx context.Context, msgs []proto.Message) error {
	l := len(msgs)
	encoded := make([][]byte, l)
	logging.Infof(ctx, "Starting write of %d protos to writestream %s", l, w.stream.streamName())
	for i, msg := range msgs {
		b, err := proto.Marshal(msg)
		if err != nil {
			return err
		}
		encoded[i] = b
	}
	write := func(batch [][]byte) error {
		logging.Infof(ctx, "AppendRows for %d rows to writestream %s", len(batch), w.stream.streamName())
		r, err := w.stream.appendRows(ctx, batch)
		if err != nil {
			return err
		}
		w.results.Go(func() error {
			return r(ctx)
		})
		return nil
	}
	for len(encoded) > 0 {
		batchSize := 0
		batchLen := 0
		for batchLen < len(encoded) && batchSize < w.maxSizePerWrite {
			batchSize += len(encoded[batchLen])
			batchLen += 1
		}
		err := write(encoded[:batchLen])
		if err != nil {
			return err
		}
		encoded = encoded[batchLen:]
	}
	return nil
}

// finalize waits for all appendRows operations to complete, then calls finalize on the writeStream.
func (w *bqWriter) finalize(ctx context.Context) error {
	logging.Infof(ctx, "Waiting on appendRow calls to writestream %s", w.stream.streamName())
	err := w.results.Wait()
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Finalizing writestream %s", w.stream.streamName())
	return w.stream.finalize(ctx)
}
