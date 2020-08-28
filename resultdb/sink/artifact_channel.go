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

	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

type artifactChannel struct {
	ch  dispatcher.Channel
	cfg *ServerConfig

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
	c := &artifactChannel{cfg: cfg}
	opts := &dispatcher.Options{
		Buffer: buffer.Options{
			// BatchSize MUST be 1, or the processing logic needs to be updated.
			//
			// The dispatcher uploads only the first item in each Batch.
			BatchSize:    1,
			MaxLeases:    16,
			FullBehavior: &buffer.BlockNewItems{MaxItems: 4000},
		},
	}
	c.ch, err = dispatcher.NewChannel(ctx, opts, func(b *buffer.Batch) error {
		task := b.Data[0].(*uploadTask)
		if task.art.GetFilePath() != "" {
			return c.cfg.ArtifactUploader.UploadFromFile(
				ctx, task.artName, task.art.ContentType, task.art.GetFilePath(), c.cfg.UpdateToken)
		}
		return c.cfg.ArtifactUploader.Upload(
			ctx, task.artName, task.art.ContentType, task.art.GetContents(), c.cfg.UpdateToken)
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create a channel for artifact uploads: %s", err))
	}
	return c
}

func (c *artifactChannel) closeAndDrain(ctx context.Context) {
	// annonuce that it is in the process of closeAndDrain.
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return
	}
	// wait for all the active sessions to finish enquing tests results to the channel
	c.wgActive.Wait()
	c.ch.CloseAndDrain(ctx)
}

func (c *artifactChannel) schedule(trs ...*sinkpb.TestResult) {
	c.wgActive.Add(1)
	defer c.wgActive.Done()
	// if the channel already has been closed, drop the test results.
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}

	for _, tr := range trs {
		for id, art := range tr.GetArtifacts() {
			c.ch.C <- &uploadTask{
				artName: pbutil.TestResultArtifactName(c.cfg.invocationID, tr.TestId, tr.ResultId, id),
				art:     art,
			}
		}
	}
}
