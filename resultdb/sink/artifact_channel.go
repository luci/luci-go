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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
)

type artifactChannel struct {
	ch  *dispatcher.Channel
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

func (c *artifactChannel) init(ctx context.Context) {
	opts := &dispatcher.Options{
		// TODO(1087955) - tune QPSLimit and MaxLeases
		QPSLimit: rate.NewLimiter(rate.Every(100*time.Millisecond), 4),
		Buffer: buffer.Options{
			BatchSize:    1,
			MaxLeases:    4,
			FullBehavior: &buffer.BlockNewItems{MaxItems: 2000},
		},
	}
	ch, err := dispatcher.NewChannel(ctx, opts, func(b *buffer.Batch) error {
		// TODO(crbug/1087955) - process
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("failed to create a channel for artifact uploads: %s", err))
	}
	c.ch = &ch
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
