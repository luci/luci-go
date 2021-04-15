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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// MaxBatchableArtifactSize is the maximum size of an artifact that can be added to
// batchChannel.
const MaxBatchableArtifactSize = 2 * 1024 * 1024

type artifactChannel struct {
	// batchChannel uploads artifacts via pb.BatchCreateArtifacts().
	//
	// This batches input artifacts and uploads them all at once.
	// This is suitable for uploading a large number of small artifacts.
	//
	// The downside of this channel is that there is a limit on the maximum size of
	// an artifact that can be included in a batch. Use streamChannel for artifacts
	// bigger than MaxBatchableArtifactSize.
	batchChannel dispatcher.Channel

	// streamChannel uploads artifacts in a streaming manner via HTTP.
	//
	// This is suitable for uploading large files, but with limited parallelism.
	// Use batchChannel, if possible.
	streamChannel dispatcher.Channel

	// wgActive indicates if there are active goroutines invoking reportTestResults.
	//
	// reportTestResults can be invoked by multiple goroutines in parallel. wgActive is used
	// to ensure that all active goroutines finish enqueuing messages to the channel before
	// closeAndDrain closes and drains the channel.
	wgActive sync.WaitGroup

	// 1 indicates that artifactChannel started the process of closing and draining
	// the channel. 0, otherwise.
	closed int32
}

type uploadTask struct {
	artName string
	art     *sinkpb.Artifact
}

func newArtifactChannel(ctx context.Context, cfg *ServerConfig) *artifactChannel {
	var err error
	c := &artifactChannel{}
	au := artifactUploader{
		Recorder:     cfg.Recorder,
		StreamClient: cfg.ArtifactStreamClient,
		StreamHost:   cfg.ArtifactStreamHost,
	}

	// batchChannel
	bcOpts := &dispatcher.Options{
		Buffer: buffer.Options{
			// BatchRequest can include up to 500 requests. KEEP BatchSize <= 500
			// For more details, visit
			// https://godoc.org/go.chromium.org/luci/resultdb/proto/v1#BatchCreateArtifactsRequest
			BatchSize:    500,
			MaxLeases:    int(cfg.ArtChannelMaxLeases),
			FullBehavior: &buffer.BlockNewItems{MaxItems: 8000},
		},
	}
	c.batchChannel, err = dispatcher.NewChannel(ctx, bcOpts, func(b *buffer.Batch) error {
		// TODO(ddoman): implement me
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create batch channel for artifacts: %s", err))
	}

	// streamChannel
	stOpts := &dispatcher.Options{
		Buffer: buffer.Options{
			// BatchSize MUST be 1.
			BatchSize:    1,
			MaxLeases:    int(cfg.ArtChannelMaxLeases),
			FullBehavior: &buffer.BlockNewItems{MaxItems: 4000},
		},
	}
	c.streamChannel, err = dispatcher.NewChannel(ctx, stOpts, func(b *buffer.Batch) error {
		return errors.Annotate(
			au.StreamUpload(ctx, b.Data[0].(*uploadTask), cfg.UpdateToken),
			"StreamUpload").Err()
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create stream channel for artifacts: %s", err))
	}
	return c
}

func (c *artifactChannel) closeAndDrain(ctx context.Context) {
	// mark the channel as closed, so that schedule() won't accept new tasks.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enquing tests results to the channel
	c.wgActive.Wait()

	var draining sync.WaitGroup
	draining.Add(2)
	go func() {
		defer draining.Done()
		c.batchChannel.CloseAndDrain(ctx)
	}()
	go func() {
		defer draining.Done()
		c.streamChannel.CloseAndDrain(ctx)
	}()
	draining.Wait()
}

func (c *artifactChannel) schedule(tasks ...*uploadTask) {
	c.wgActive.Add(1)
	defer c.wgActive.Done()
	// if the channel already has been closed, drop the test results.
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}

	for _, task := range tasks {
		// TODO(ddoman): send small artifacts to batchChannel
		c.streamChannel.C <- task
	}
}
