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

package cli

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

var notReady = errors.BoolTag{Key: errors.NewTagKey("task incomplete")}

// pollingIter combines collectIter and retry.Default for transient errors.
// Uses the former for errors marked with notReady tag.
type pollingIter struct {
	collect   collectIter
	transient retry.Iterator
}

func newPollingIter() retry.Iterator {
	return &pollingIter{}
}

// Next implements Iterator.
func (p *pollingIter) Next(ctx context.Context, err error) time.Duration {
	switch {
	case notReady.In(err):
		p.transient = nil // reset out transient iterator.
		return p.collect.Next(ctx, err)

	case transient.Tag.In(err):
		if p.transient == nil {
			p.transient = retry.Default()
		}
		return p.transient.Next(ctx, err)

	default:
		return retry.Stop
	}
}

// collectIter implements the polling policy matching the one used by
// `swarming collect`:
// https://source.chromium.org/chromium/_/chromium/infra/luci/client-py.git/+/885b3febcc170a60f25795304e60927b77d1e92d:swarming.py;l=546;drc=7a76117d3fc4543d7e76c6fb3d39cc2977da0ab9
// which is
//   Start with 1 sec delay and for each 30 sec of waiting add another second
//   of delay, until hitting 15 sec ceiling.
type collectIter struct {
	start time.Time
}

// Next implements Iterator.
func (c *collectIter) Next(ctx context.Context, err error) time.Duration {
	now := clock.Now(ctx)
	if c.start.IsZero() {
		c.start = now
	}
	delay := now.Sub(c.start) / 30 / time.Second
	switch {
	case delay < time.Second:
		delay = time.Second
	case delay > 15*time.Second:
		delay = 15 * time.Second
	}
	return delay
}
