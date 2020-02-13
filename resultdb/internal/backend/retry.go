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

package backend

import (
	"context"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
)

var (
	streamClosed  = errors.BoolTag{Key: errors.NewTagKey("stream closed")}
	quotaExceeded = errors.BoolTag{Key: errors.NewTagKey("quota exceeded")}
)

type internalIter struct {
	errTags   []errors.BoolTag
	delay     time.Duration
	maxDelay  time.Duration
	addJitter bool
}

// next implements exponential backoff retry.
// Not use retry.ExponentialBackOff because we don't want to limited by
// number of retries, and also can add jitter.
func (it *internalIter) next(ctx context.Context) time.Duration {
	delay := it.delay
	switch {
	case delay > it.maxDelay:
		delay = it.maxDelay
	default:
		nextDelay := delay * 2
		if it.addJitter {
			// +-10% of next delay.
			nextDelay = nextDelay - nextDelay/10 + time.Duration(mathrand.Intn(ctx, int(nextDelay/5)))
		}
		it.delay = nextDelay
	}

	return delay
}

func (it *internalIter) supportError(err error) bool {
	for _, errTag := range it.errTags {
		if errTag.In(err) {
			return true
		}
	}
	return false
}

// retryIter is an implementation of retry.Iterator.
// The error must be tagged with supported tags (e.g. streamClosed or
// quotaExceeded) for retrying to continue.
type retryIter struct {
	iters []*internalIter
}

func newRetryIter() retry.Iterator {
	return &retryIter{
		iters: []*internalIter{
			{
				errTags: []errors.BoolTag{
					streamClosed,
				},
				delay:     200 * time.Millisecond,
				maxDelay:  10 * time.Second,
				addJitter: false,
			},
			{
				errTags: []errors.BoolTag{
					quotaExceeded,
				},
				delay:     time.Second,
				maxDelay:  10 * time.Second,
				addJitter: true,
			},
		}}
}

// Next implements retry.Iterator.
// Next returns delay based on the type of error.
// If we get different errors  when retrying a request, for example
// [quotaExceeded, streamClosed, streamClosed, streamClosed, quotaExceeded],
// Next will return delays like (if no jitter):
// [1s, 0.2s, 0.4s, 0.6s, 2s].
func (rt *retryIter) Next(ctx context.Context, err error) time.Duration {
	for _, it := range rt.iters {
		if it.supportError(err) {
			return it.next(ctx)
		}
	}
	return retry.Stop
}
