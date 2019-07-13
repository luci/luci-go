// Copyright 2019 The LUCI Authors.
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

package token

import (
	"context"
	"errors"
	"time"
)

type iTicker interface {
	GetC() <-chan time.Time
	Stop()
}

type realTicker time.Ticker

func (r *realTicker) GetC() <-chan time.Time { return r.C }
func (r *realTicker) Stop()                  { r.Stop() }

var newTicker = func(interval time.Duration) iTicker {
	return (*realTicker)(time.NewTicker(interval))
}

// Bucket represents a fixed-size bucket which refills at a fixed rate.
//
// Construct this with NewBucket.
//
// Clients of this should pull from `C`; once the pull has succeeded, that
// client "has" a token good for one operation (usually an RPC of some sort).
type Bucket struct {
	C      <-chan struct{}
	cancel func()
}

// Close stops the Bucket from filling, and closes the `C` channel, causing
// any other
func (q *Bucket) Close() {
	q.cancel()
}

// NewBucket returns a new token bucket.
//
// The bucket will refill at a rate provided by tokensPerSecond, and will
// conatin a maximum of `depth` tokens.
//
// If tokensPerSecond is <= 0, the bucket has infinite depth and refills at
// infinite speed.
//
// The caller is responsible for closing the Bucket when they're finished
// with it, to avoid leaking resources.
//
// Closing `ctx` will also cause the Bucket to close.
func NewBucket(ctx context.Context, depth int, tokensPerSecond float64) *Bucket {
	if tokensPerSecond <= 0 {
		ret := make(chan struct{})
		close(ret)
		return &Bucket{ret, func() {}}
	}

	if depth <= 0 {
		panic(errors.New("if tokensPerSecond is positive, depth must also be positive"))
	}

	ctx, cancel := context.WithCancel(ctx)

	rawChan := make(chan struct{}, depth)

	go func() {
		defer close(rawChan)

		interval := time.Duration(float64(time.Second) / tokensPerSecond)

		ticker := newTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.GetC():
				// try to send it, but if we would block, we need to pause our ticker
				// for the duration we're blocked.
				select {
				case rawChan <- struct{}{}:
					continue
				default:
				}

				// We're goin' in for a (maybe) long sleep
				ticker.Stop()
				select {
				case rawChan <- struct{}{}:
				case <-ctx.Done():
					return
				}

				// Ok, we're back, start the clock again. It should take `interval` time
				// for the next token to appear in the bucket now.
				ticker = newTicker(interval)
			}
		}
	}()

	return &Bucket{rawChan, cancel}
}
