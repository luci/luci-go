// Copyright 2022 The LUCI Authors.
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

package bugs

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// Shorten returns a derived context with its deadline shortened by d.
// The context can also be cancelled with the returned cancel function.
func Shorten(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return context.WithCancel(ctx)
	}
	return context.WithDeadline(ctx, deadline.Add(-d))
}

// EnsureTimeToDeadline ensures there is at least duration `d` available
// before the context deadline. If not, it returns an error. It also returns
// an error if the context is already cancelled.
func EnsureTimeToDeadline(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	minimumDeadline := clock.Now(ctx).Add(d)
	if ok && deadline.Before(minimumDeadline) {
		return errors.Fmt("context deadline has less than %v remaining", d)
	}
	return nil
}
