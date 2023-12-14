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
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/scheduler/appengine/messages"
)

// ValidateDefinition validates the triggering policy message.
//
// Emits errors into the given context.
func ValidateDefinition(ctx *validation.Context, p *messages.TriggeringPolicy) {
	switch p.Kind {
	case
		messages.TriggeringPolicy_UNDEFINED, // same as GREEDY_BATCHING
		messages.TriggeringPolicy_GREEDY_BATCHING,
		messages.TriggeringPolicy_LOGARITHMIC_BATCHING,
		messages.TriggeringPolicy_NEWEST_FIRST:
		// ok
	default:
		ctx.Errorf("unrecognized policy kind %d", p.Kind)
	}

	// Note: 0 is fine. It means "use default".
	if p.MaxConcurrentInvocations < 0 {
		ctx.Errorf("max_concurrent_invocations should be positive, got %d", p.MaxConcurrentInvocations)
	}
	// Same here.
	if p.MaxBatchSize < 0 {
		ctx.Errorf("max_batch_size should be positive, got %d", p.MaxBatchSize)
	}
	// And here.
	if p.PendingTimeout.AsDuration() < 0 {
		ctx.Errorf("pending_timeout should be positive, got %s", p.PendingTimeout.AsDuration())
	}

	// Let's make sure PendingTimeout is unset if Kind != NEWEST_FIRST, since it doesn't do
	// anything in those cases. Otherwise it's easy to make a false assumption.
	if p.Kind != messages.TriggeringPolicy_NEWEST_FIRST && p.PendingTimeout.AsDuration() != 0 {
		ctx.Errorf("pending_timeout is non-zero with non-NEWEST_FIRST policy, but pending_timeout doesn't apply to those policies")
	}

	if p.Kind == messages.TriggeringPolicy_LOGARITHMIC_BATCHING && p.LogBase < 1.0001 {
		ctx.Errorf("log_base should be larger or equal 1.0001, got %f", p.LogBase)
	}
}
