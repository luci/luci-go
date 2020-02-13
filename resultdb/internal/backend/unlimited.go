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

// unLimited is an retry.Iterator implementation that only retries errors
// if they are tagged with errTag.
type unLimited struct {
	errTag   errors.BoolTag
	delay    time.Duration
	maxDelay time.Duration
}

// Next implements exponential backoff retry.
// Not use retry.ExponentialBackOff because we don't want to limited by
// number of retries, and also add jitter.
func (it *unLimited) Next(ctx context.Context, err error) time.Duration {
	if !it.errTag.In(err) {
		return retry.Stop
	}

	delay := it.delay
	switch {
	case delay > it.maxDelay:
		delay = it.maxDelay
	default:
		nextDelay := delay * 2
		// +-10% of next delay.
		nextDelay = nextDelay - nextDelay/10 + time.Duration(mathrand.Intn(ctx, int(nextDelay/5)))
		it.delay = nextDelay
	}

	return delay
}

func unLimitedFactory(errTag errors.BoolTag, delay, maxDelay time.Duration) retry.Factory {
	return func() retry.Iterator {
		return &unLimited{
			errTag:   errTag,
			delay:    delay,
			maxDelay: maxDelay,
		}
	}
}
