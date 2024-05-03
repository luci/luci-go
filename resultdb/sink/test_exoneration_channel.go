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

package sink

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// unexpectedPassChannel is the channel for unexpected passes which will be exonerated.
type unexpectedPassChannel struct {
	ch  dispatcher.Channel[any]
	cfg *ServerConfig

	// wgActive indicates if there are active goroutines invoking reportTestExonerations.
	//
	// reportTestExonerations can be invoked by multiple goroutines in parallel. wgActive is used
	// to ensure that all active goroutines finish enqueuing messages to the channel before
	// closeAndDrain closes and drains the channel.
	wgActive sync.WaitGroup

	// 1 indicates that unexpectedPassChannel started the process of closing and draining
	// the channel. 0, otherwise.
	closed int32
}

func newTestExonerationChannel(ctx context.Context, cfg *ServerConfig) *unexpectedPassChannel {
	var err error
	c := &unexpectedPassChannel{cfg: cfg}
	opts := &dispatcher.Options{
		Buffer: buffer.Options{
			// BatchRequest can include up to 500 requests. KEEP BatchItemsMax <= 500
			// to keep report() simple. For more details, visit
			// https://godoc.org/go.chromium.org/luci/resultdb/proto/v1#BatchCreateTestExonerations
			BatchItemsMax: 500,
			BatchAgeMax:   time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 8000},
		},
	}
	c.ch, err = dispatcher.NewChannel[any](ctx, opts, func(b *buffer.Batch) error {
		return c.report(ctx, b)
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create a channel for TestExoneration: %s", err))
	}
	return c
}

func (c *unexpectedPassChannel) closeAndDrain(ctx context.Context) {
	// announce that it is in the process of closeAndDrain.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enqueueing tests exonerations to the channel
	c.wgActive.Wait()
	c.ch.CloseAndDrain(ctx)
}

func (c *unexpectedPassChannel) schedule(trs ...*sinkpb.TestResult) {
	c.wgActive.Add(1)
	defer c.wgActive.Done()
	// if the channel already has been closed, drop the test exonerations.
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}
	for _, tr := range trs {
		c.ch.C <- tr
	}
}

func (c *unexpectedPassChannel) report(ctx context.Context, b *buffer.Batch) error {
	if b.Meta == nil {
		reqs := make([]*pb.CreateTestExonerationRequest, len(b.Data))
		for i, d := range b.Data {
			tr := d.Item.(*sinkpb.TestResult)
			reqs[i] = &pb.CreateTestExonerationRequest{
				TestExoneration: &pb.TestExoneration{
					TestId:          tr.GetTestId(),
					Variant:         c.cfg.BaseVariant,
					ExplanationHtml: "Unexpected passes are exonerated",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}
		}
		b.Meta = &pb.BatchCreateTestExonerationsRequest{
			Invocation: c.cfg.Invocation,
			// a random UUID
			RequestId: uuid.New().String(),
			Requests:  reqs,
		}
	}
	_, err := c.cfg.Recorder.BatchCreateTestExonerations(ctx, b.Meta.(*pb.BatchCreateTestExonerationsRequest))
	return err
}
