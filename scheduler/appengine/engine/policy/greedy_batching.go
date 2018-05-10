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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/scheduler/appengine/task"
)

// GreedyBatchingPolicy instantiates new GREEDY_BATCHING policy function.
//
// It takes all pending triggers and collapses them into one new invocation,
// deriving its properties from the most recent trigger alone.
func GreedyBatchingPolicy(maxConcurrentInvs int) (Func, error) {
	if maxConcurrentInvs <= 0 {
		return nil, errors.Reason("'max concurrent invocations' should be positive").Err()
	}
	return func(env Environment, in In) (out Out) {
		switch {
		case len(in.Triggers) == 0:
			return // nothing new to launch
		case len(in.ActiveInvocations) >= maxConcurrentInvs:
			env.DebugLog(
				"Max concurrent invocations is %d and there's %d running => refusing to launch more",
				maxConcurrentInvs, len(in.ActiveInvocations))
			return // maxed all available slots
		}

		// Construct a single request from all incoming triggers, deriving the
		// properties from the most recent one (which is last in the list, since
		// in.Triggers are sorted by time already).
		req := RequestBuilder{env: env}
		req.FromTrigger(in.Triggers[len(in.Triggers)-1])
		req.IncomingTriggers = in.Triggers

		out.Requests = []task.Request{req.Request}
		return
	}, nil
}
