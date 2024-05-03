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
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type testResultChannel struct {
	ch  dispatcher.Channel[any]
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
	opts := &dispatcher.Options{
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
	c.ch, err = dispatcher.NewChannel[any](ctx, opts, func(b *buffer.Batch) error {
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

func (c *testResultChannel) schedule(trs ...*sinkpb.TestResult) {
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

// setLocationSpecificFields sets the test tags and bug component in tr
// by looking for the directory of tr.TestMetadata.Location.FileName
// in the location tags file.
func (c *testResultChannel) setLocationSpecificFields(tr *sinkpb.TestResult) {
	if c.cfg.LocationTags == nil || tr.TestMetadata.GetLocation().GetFileName() == "" {
		return
	}
	repo, ok := c.cfg.LocationTags.Repos[tr.TestMetadata.Location.Repo]
	if !ok || (len(repo.GetDirs()) == 0 && len(repo.GetFiles()) == 0) {
		return
	}

	tagKeySet := stringset.New(0)

	var bugComponent *pb.BugComponent

	// if a test result has a matching file location by file name, use the metadata
	// associated with it first. Fill in the rest using directory metadata.
	// fileName must start with "//" and it has been validated.
	filePath := strings.TrimPrefix(tr.TestMetadata.Location.FileName, "//")
	if f, ok := repo.Files[filePath]; ok {
		for _, ft := range f.Tags {
			if !tagKeySet.Has(ft.Key) {
				tr.Tags = append(tr.Tags, ft)
			}
		}

		// Fill in keys from file definition so that they are not repeated.
		for _, ft := range f.Tags {
			tagKeySet.Add(ft.Key)
		}

		if bugComponent == nil {
			bugComponent = f.BugComponent
		}
	}

	dir := path.Dir(filePath)
	// Start from the directory of the file, then traverse to upper directories.
	for {
		if d, ok := repo.Dirs[dir]; ok {
			for _, t := range d.Tags {
				if !tagKeySet.Has(t.Key) {
					tr.Tags = append(tr.Tags, t)
				}
			}

			// Add new keys to tagKeySet.
			// We cannot do this above because tag keys for this dir could be repeated.
			for _, t := range d.Tags {
				tagKeySet.Add(t.Key)
			}

			if bugComponent == nil {
				bugComponent = d.BugComponent
			}
		}
		if dir == "." {
			// Have reached the root.
			break
		}
		dir = path.Dir(dir)
	}

	// Use LocationTags-derived bug component if one is not already set.
	if tr.TestMetadata.BugComponent == nil && bugComponent != nil {
		tr.TestMetadata.BugComponent = proto.Clone(bugComponent).(*pb.BugComponent)
	}
}

func (c *testResultChannel) report(ctx context.Context, b *buffer.Batch) error {
	// retried batch?
	if b.Meta == nil {
		reqs := make([]*pb.CreateTestResultRequest, len(b.Data))
		for i, d := range b.Data {
			tr := d.Item.(*sinkpb.TestResult)
			c.setLocationSpecificFields(tr)
			tags := append(tr.GetTags(), c.cfg.BaseTags...)

			// The test result variant will overwrite the value for the
			// duplicate key in the base variant.
			variant := pbutil.CombineVariant(c.cfg.BaseVariant, tr.GetVariant())

			pbutil.SortStringPairs(tags)
			reqs[i] = &pb.CreateTestResultRequest{
				TestResult: &pb.TestResult{
					TestId:        tr.GetTestId(),
					ResultId:      tr.GetResultId(),
					Variant:       variant,
					Expected:      tr.GetExpected(),
					Status:        tr.GetStatus(),
					SummaryHtml:   tr.GetSummaryHtml(),
					StartTime:     tr.GetStartTime(),
					Duration:      tr.GetDuration(),
					Tags:          tags,
					TestMetadata:  tr.GetTestMetadata(),
					FailureReason: tr.GetFailureReason(),
					Properties:    tr.GetProperties(),
				},
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
