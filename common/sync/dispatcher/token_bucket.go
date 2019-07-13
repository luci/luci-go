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

	"go.chromium.org/luci/common/clock"
)

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

		interval := time.Duration(maxQPS / float64(time.Second))
		timer := clock.NewTimer(clock.Tag(ctx, "qpsBucket"))
		defer timer.Stop()

		for {
			timer.Reset(interval)

			select {
			case <-ctx.Done():
				return

			case <-timer.GetC():
				// don't need to check timer result; it's either good, or dead b/c the
				// context is canceled; we immediately re-select the context anyway so
				// we'll find out on the next line.

				select {
				case <-ctx.Done():
					return

				case rawChan <- struct{}{}:
				}
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
