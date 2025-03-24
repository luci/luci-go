// Copyright 2020 The LUCI Authors.
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
)

type testResultChannel struct {
	ch  dispatcher.Channel[*pb.TestResult]
	cfg *ServerConfig

	// wgActive indicates if there are active goroutines invoking reportTestResults.
	//
	// reportTestResults can be invoked by multiple goroutines in parallel. wgActive is used
	// to ensure that all active goroutines finish enqueuing messages to the channel before
	// closeAndDrain closes and drains the channel.
	wgActive sync.WaitGroup

	// 1 indicates that testResultChannel started the process of closing and draining
	// the channel. 0, otherwise.
	closed int32
}

func newTestResultChannel(ctx context.Context, cfg *ServerConfig) *testResultChannel {
	var err error
	c := &testResultChannel{cfg: cfg}
	opts := &dispatcher.Options[*pb.TestResult]{
		Buffer: buffer.Options{
			// BatchRequest can include up to 500 requests. KEEP BatchItemsMax <= 500
			// to keep report() simple. For more details, visit
			// https://godoc.org/go.chromium.org/luci/resultdb/proto/v1#BatchCreateTestResultsRequest
			BatchItemsMax: 500,
			MaxLeases:     int(cfg.TestResultChannelMaxLeases),
			BatchAgeMax:   time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 8000},
		},
	}
	c.ch, err = dispatcher.NewChannel[*pb.TestResult](ctx, opts, func(b *buffer.Batch[*pb.TestResult]) error {
		return c.report(ctx, b)
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create a channel for TestResult: %s", err))
	}
	return c
}

func (c *testResultChannel) closeAndDrain(ctx context.Context) {
	// announce that it is in the process of closeAndDrain.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enquing tests results to the channel
	c.wgActive.Wait()
	c.ch.CloseAndDrain(ctx)
}

func (c *testResultChannel) schedule(trs ...*pb.TestResult) {
	c.wgActive.Add(1)
	defer c.wgActive.Done()
	// if the channel already has been closed, drop the test results.
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}
	for _, tr := range trs {
		c.ch.C <- tr
	}
}

func (c *testResultChannel) report(ctx context.Context, b *buffer.Batch[*pb.TestResult]) error {
	// retried batch?
	if b.Meta == nil {
		reqs := make([]*pb.CreateTestResultRequest, len(b.Data))
		for i, d := range b.Data {
			tr := d.Item

			reqs[i] = &pb.CreateTestResultRequest{
				TestResult: tr,
			}
		}
		b.Meta = &pb.BatchCreateTestResultsRequest{
			Invocation: c.cfg.Invocation,
			// a random UUID
			RequestId: uuid.New().String(),
			Requests:  reqs,
		}
	}
	_, err := c.cfg.Recorder.BatchCreateTestResults(ctx, b.Meta.(*pb.BatchCreateTestResultsRequest))
	return err
}
