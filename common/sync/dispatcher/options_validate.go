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

	"golang.org/x/time/rate"

	"go.chromium.org/luci/common/errors"
)

// normalize validates that Options is well formed and populates defaults which
// are missing.
func (o *Options[T]) normalize(ctx context.Context) error {
	if o.ItemSizeFunc == nil {
		if o.Buffer.BatchSizeMax > 0 {
			return errors.New("Buffer.BatchSizeMax > 0 but ItemSizerFunc == nil")
		}
	}

	if o.ErrorFn == nil {
		o.ErrorFn = defaultErrorFnFactory[T](ctx)
	}
	if o.DropFn == nil {
		o.DropFn = defaultDropFnFactory[T](ctx, o.Buffer.FullBehavior)
	}

	if o.QPSLimit == nil {
		o.QPSLimit = rate.NewLimiter(rate.Inf, 0)
	}
	if o.QPSLimit.Limit() != rate.Inf && o.QPSLimit.Burst() < 1 {
		return errors.Fmt("QPSLimit has burst size < 1, but a non-infinite rate: %d",
			o.QPSLimit.Burst())
	}

	if o.MinQPS == rate.Inf {
		return errors.New("MinQPS cannot be infinite")
	}

	if o.MinQPS > 0 && o.MinQPS > o.QPSLimit.Limit() {
		return errors.Fmt("MinQPS: %f is greater than QPSLimit: %f",
			o.MinQPS, o.QPSLimit.Limit())
	}

	return nil
}

// durationFromLimit converts a rate.Limit to a time.Duration.
func durationFromLimit(limit rate.Limit) time.Duration {
	switch {
	case limit == rate.Inf:
		return 0
	case limit <= 0:
		// Reset the duration to 0, instead of returning rate.InfDuration.
		return 0
	default:
		seconds := float64(1) / float64(limit)
		return time.Duration(float64(time.Second) * seconds)
	}
}
