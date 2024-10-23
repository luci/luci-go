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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/scheduler/appengine/messages"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	run := func(p messages.TriggeringPolicy) error {
		ctx := validation.Context{}
		ValidateDefinition(&ctx, &p)
		return ctx.Finalize()
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, run(messages.TriggeringPolicy{}), should.BeNil)
		assert.Loosely(t, run(messages.TriggeringPolicy{Kind: 123}),
			should.ErrLike("unrecognized policy kind 123"))
		assert.Loosely(t, run(messages.TriggeringPolicy{MaxConcurrentInvocations: -1}),
			should.ErrLike("max_concurrent_invocations should be positive, got -1"))
		assert.Loosely(t, run(messages.TriggeringPolicy{MaxBatchSize: -1}),
			should.ErrLike("max_batch_size should be positive, got -1"))
		assert.Loosely(t, run(messages.TriggeringPolicy{
			Kind: messages.TriggeringPolicy_GREEDY_BATCHING}), should.BeNil)
		assert.Loosely(t, run(messages.TriggeringPolicy{
			Kind: messages.TriggeringPolicy_NEWEST_FIRST}), should.BeNil)
		assert.Loosely(t, run(messages.TriggeringPolicy{
			Kind: messages.TriggeringPolicy_LOGARITHMIC_BATCHING, LogBase: 0.5}),
			should.ErrLike("log_base should be larger or equal 1.0001, got 0.5"))
		assert.Loosely(t, run(messages.TriggeringPolicy{
			Kind:           messages.TriggeringPolicy_NEWEST_FIRST,
			PendingTimeout: durationpb.New(-time.Hour)}),
			should.ErrLike("pending_timeout should be positive, got -1h"))
		assert.Loosely(t, run(messages.TriggeringPolicy{
			PendingTimeout: durationpb.New(time.Hour)}),
			should.ErrLike("pending_timeout is non-zero with non-NEWEST_FIRST policy"))
	})
}
