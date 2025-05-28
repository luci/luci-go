// Copyright 2021 The LUCI Authors.
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

package lease

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/common"
)

// MakeCLResourceID returns ResourceID of a CL in CV.
func MakeCLResourceID(clid common.CLID) ResourceID {
	return ResourceID(fmt.Sprintf("CL/%d", clid))
}

// ApplyOnCL applies for an exclusive lease on the CL.
//
// Typically used before a Gerrit CL is mutated.
//
// Returns:
//   - new time-limited context,
//   - best-effort lease cancellation func,
//   - and an error, if any.
func ApplyOnCL(ctx context.Context, clid common.CLID, duration time.Duration, requester string) (context.Context, func(), error) {
	leaseExpireTime := clock.Now(ctx).Add(duration)
	if d, ok := ctx.Deadline(); ok && d.Before(leaseExpireTime) {
		leaseExpireTime = d // Honor the deadline imposed by context
	}
	leaseExpireTime = leaseExpireTime.UTC().Truncate(time.Millisecond)
	l, err := Apply(ctx, Application{
		ResourceID: MakeCLResourceID(clid),
		Holder:     requester,
		ExpireTime: leaseExpireTime,
	})
	if err != nil {
		return nil, nil, err
	}

	dctx, cancel := clock.WithDeadline(ctx, leaseExpireTime)
	terminate := func() {
		cancel()
		if err := l.Terminate(ctx); err != nil {
			// Best-effort termination since lease will expire naturally.
			common.LogError(ctx, errors.Fmt("failed to terminate a lease on the CL %d: %w", clid, err))
		}
	}
	return dctx, terminate, nil
}
