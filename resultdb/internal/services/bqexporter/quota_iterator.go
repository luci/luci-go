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

package bqexporter

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/retry"
)

// quotaErrorIterator is an retry.Iterator implementation that only retries
// Google API quota errors.
type quotaErrorIterator struct {
	delay    time.Duration
	maxDelay time.Duration
}

// Next implements exponential backoff retry.
// Not use retry.ExponentialBackOff because we don't want to limited by
// number of retries, and also add jitter.
func (it *quotaErrorIterator) Next(ctx context.Context, err error) time.Duration {
	if apiErr, ok := err.(*googleapi.Error); !ok || apiErr.Code != http.StatusForbidden || !hasReason(apiErr, "quotaExceeded") {
		return retry.Stop
	}

	delay := it.delay
	if delay > it.maxDelay {
		delay = it.maxDelay
	} else {
		nextDelay := delay * 2
		// +-10% of next delay.
		nextDelay = nextDelay - nextDelay/10 + time.Duration(mathrand.Intn(ctx, int(nextDelay/5)))
		it.delay = nextDelay
	}

	return delay
}

func quotaErrorIteratorFactory() retry.Factory {
	return func() retry.Iterator {
		return &quotaErrorIterator{
			delay:    time.Second,
			maxDelay: 10 * time.Second,
		}
	}
}
