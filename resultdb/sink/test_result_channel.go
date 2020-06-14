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
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type testResultChannel struct {
	ch  *dispatcher.Channel
	cfg ServerConfig

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

func (c *testResultChannel) init(ctx context.Context) {
	// install a dispatcher channel for pb.TestResult
	opts := &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:     400,
			MaxLeases:     4,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 2000},
		},
	}
	ch, err := dispatcher.NewChannel(ctx, opts, func(b *buffer.Batch) error {
		req := c.prepareReportTestResultsRequest(ctx, b)
		_, err := c.cfg.Recorder.BatchCreateTestResults(ctx, req)
		return err
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create a channel for TestResult: %s", err))
	}
	c.ch = &ch
}

func (c *testResultChannel) closeAndDrain(ctx context.Context) {
	// annonuce that it is in the process of closeAndDrain.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enquing tests results to the channel
	c.wgActive.Wait()
	c.ch.CloseAndDrain(ctx)
}

func (c *testResultChannel) reportTestResults(trs []*sinkpb.TestResult) {
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

func (c *testResultChannel) prepareReportTestResultsRequest(ctx context.Context, b *buffer.Batch) *pb.BatchCreateTestResultsRequest {
	// retried batch?
	if b.Meta != nil {
		return b.Meta.(*pb.BatchCreateTestResultsRequest)
	}
	req := &pb.BatchCreateTestResultsRequest{
		Invocation: c.cfg.Invocation,
		// a random UUID
		RequestId: uuid.New().String(),
	}
	for _, d := range b.Data {
		tr := d.(*sinkpb.TestResult)
		req.Requests = append(req.Requests, &pb.CreateTestResultRequest{
			TestResult: &pb.TestResult{
				TestId:      tr.GetTestId(),
				ResultId:    tr.GetResultId(),
				Variant:     c.cfg.BaseVariant,
				Expected:    tr.GetExpected(),
				SummaryHtml: tr.GetSummaryHtml(),
				StartTime:   tr.GetStartTime(),
				Duration:    tr.GetDuration(),
				Tags:        tr.GetTags(),
			},
		})
	}
	b.Meta = req
	return req
}

func sinkArtsToRPCArts(ctx context.Context, sArts map[string]*sinkpb.Artifact) (rArts []*pb.Artifact) {
	for name, sart := range sArts {
		var size int64 = -1
		switch {
		case sart.GetFilePath() != "":
			if info, err := os.Stat(sart.GetFilePath()); err == nil {
				size = info.Size()
			} else {
				logging.Errorf(ctx, "artifact %q: %q - %s", name, sart.GetFilePath(), err)
			}
		case sart.GetContents() != nil:
			size = int64(len(sart.GetContents()))
		default:
			// This should never be reached. pbutil.ValidateSinkArtifact() should
			// filter out invalid artifacts.
			panic(fmt.Sprintf("%s: neither file_path nor contents were given", name))
		}

		rArts = append(rArts, &pb.Artifact{
			Name: name,
			// TODO(ddoman): set fetch_url and fetch_url_expiration
			ContentType: sart.GetContentType(),
			SizeBytes:   size,
		})
	}
	sort.Slice(rArts, func(i, j int) bool {
		return rArts[i].Name < rArts[j].Name
	})
	return
}
