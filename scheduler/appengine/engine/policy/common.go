// Copyright 2018 The LUCI Authors.
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
	"fmt"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/scheduler/appengine/internal"
)

// basePolicy returns a generic policy customizable by the `reduce` func.
//
// Parameters `maxConcurrentInvs` and `maxBatchSize` control how many
// invocations can be running in parallel and maximum number of triggers that
// can be collapsed into a single invocation correspondingly. The `reducer` func
// allows to customize policy: it takes a list of outstanding triggers and must
// return a number of triggers to be collapsed. The returned number must be >=
// 0. The created invocation will collapse the specified number of triggers and
// take the properties of the latest one. If the returned number is 0, no
// invocation is created. If it is larger than `maxBatchSize`, the value is
// ignored and invocation collapsing `maxBatchSize` triggers is created.
func basePolicy(maxConcurrentInvs, maxBatchSize int, reducer func([]*internal.Trigger) int) (Func, error) {
	switch {
	case maxConcurrentInvs <= 0:
		return nil, errors.New("max_concurrent_invocations should be positive")
	case maxBatchSize <= 0:
		return nil, errors.New("max_batch_size should be positive")
	}

	return func(env Environment, in In) (out Out) {
		slots := maxConcurrentInvs - len(in.ActiveInvocations)
		switch {
		case len(in.Triggers) == 0:
			return // nothing new to launch
		case slots <= 0:
			env.DebugLog(
				"Max concurrent invocations is %d and there's %d running => refusing to launch more",
				maxConcurrentInvs, len(in.ActiveInvocations))
			return // maxed all available slots
		}

		triggers := in.Triggers
		for slots > 0 && len(triggers) != 0 {
			// Grab up to maxBatchSize triggers.
			size := maxBatchSize
			numTriggers := reducer(triggers)
			if numTriggers == 0 {
				break
			}
			if numTriggers > len(triggers) {
				panic(fmt.Errorf("invalid policy implementation: returned %d which is more than %d triggers given", numTriggers, len(triggers)))
			}
			if size > numTriggers {
				size = numTriggers
			}
			batch := triggers[:size]
			triggers = triggers[size:]

			// Put them into the new invocation, deriving its properties from the most
			// recent trigger (which is last in the list, since triggers are sorted by
			// time already)
			req := RequestBuilder{env: env}
			req.FromTrigger(batch[len(batch)-1])
			req.IncomingTriggers = batch

			// If all triggers were submitted by the same single identity (perhaps the
			// scheduler service itself as indicated by empty EmittedByUser),
			// attribute the invocation to this identity. This is mostly for UI,
			// nothing really depends on TriggeredBy field.
			idents := stringset.New(1)
			for _, t := range batch {
				idents.Add(t.EmittedByUser)
			}
			if idents.Len() == 1 {
				req.TriggeredBy = identity.Identity(idents.ToSlice()[0])
			}

			out.Requests = append(out.Requests, req.Request)
			slots--
		}

		return
	}, nil
}
