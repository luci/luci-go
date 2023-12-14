// Copyright 2023 The LUCI Authors.
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

package policy

import (
	"sort"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/internal"
)

// NewestFirstPolicy instantiates new NEWEST_FIRST policy function.
//
// It takes the most newly added pending triggers and creates invocations
// for them. It also discards triggers that have been pending for a long time.
func NewestFirstPolicy(maxConcurrentInvs int, pendingTimeout time.Duration) (Func, error) {
	switch {
	case maxConcurrentInvs <= 0:
		return nil, errors.Reason("max_concurrent_invocations should be positive").Err()
	case pendingTimeout <= 0:
		return nil, errors.Reason("pending_timeout should be positive").Err()
	}

	return func(env Environment, in In) (out Out) {
		// Split the triggers list into what we're going to discard and what's going to be invoked.
		// Every trigger that's exceeded the pending timeout is going to be discarded.
		triggers := in.Triggers
		i := sort.Search(len(triggers), func(i int) bool {
			pendingTime := in.Now.Sub(triggers[i].Created.AsTime())
			return pendingTime < pendingTimeout
		})
		out.Discard = triggers[:i]
		invoke := triggers[i:]

		// Determine how many available concurrent invocations we have.
		slots := maxConcurrentInvs - len(in.ActiveInvocations)
		if slots <= 0 {
			// Exit early since we can't fill any slots anyway.
			env.DebugLog(
				"Max concurrent invocations is %d and there's %d running => refusing to launch more",
				maxConcurrentInvs, len(in.ActiveInvocations))
			return // maxed all available slots
		}

		// Prune the invoke list down to the most recent triggers that will fit in the
		// available invocation slots.
		if len(invoke) > slots {
			invoke = invoke[len(invoke)-slots:]
		}

		// Create requests for everything left on the invoke list.
		for _, t := range invoke {
			// One trigger maps to one request. There's no batching here.
			req := RequestBuilder{env: env}
			req.FromTrigger(t)
			req.IncomingTriggers = []*internal.Trigger{t}
			req.TriggeredBy = identity.Identity(t.EmittedByUser)

			// Add the request to the request list.
			out.Requests = append(out.Requests, req.Request)
		}

		return
	}, nil
}
