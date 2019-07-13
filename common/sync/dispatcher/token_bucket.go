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

package dispatcher

import (
	"context"
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

type qpsBucket struct {
	ch     chan struct{}
	cancel func()
}

func (q *qpsBucket) GetC() <-chan struct{} {
	return q.ch
}

func (q *qpsBucket) Close() {
	q.cancel()
}

func newQPSBucket(ctx context.Context, depth int, maxQPS float64) *qpsBucket {
	ctx, cancel := context.WithCancel(ctx)

	rawChan := make(chan struct{}, depth)

	go func() {
		defer close(rawChan)

		interval := time.Duration(float64(time.Second) / maxQPS)

		ticker := newTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.GetC():
				// try to send it, but if we would block, we need to reset our ticker.
				select {
				case rawChan <- struct{}{}:
					continue
				default:
				}

				ticker.Stop()
				// We're goin' in for a (maybe) long sleep
				select {
				case rawChan <- struct{}{}:
				case <-ctx.Done():
					return
				}

				// Ok, we're back, start the clock again.
				ticker = newTicker(interval)
			}
		}
	}()

	return &qpsBucket{rawChan, cancel}
}

func newInfiniteQPSBucket() *qpsBucket {
	ret := make(chan struct{})
	close(ret)
	return &qpsBucket{ret, func() {}}
}
