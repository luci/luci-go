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

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/scheduler/appengine/messages"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	run := func(p messages.TriggeringPolicy) error {
		ctx := validation.Context{}
		ValidateDefinition(&ctx, &p)
		return ctx.Finalize()
	}

	Convey("Works", t, func() {
		So(run(messages.TriggeringPolicy{}), ShouldBeNil)
		So(run(messages.TriggeringPolicy{Kind: 123}),
			ShouldErrLike, "unrecognized policy kind 123")
		So(run(messages.TriggeringPolicy{MaxConcurrentInvocations: -1}),
			ShouldErrLike, "max_concurrent_invocations should be positive, got -1")
		So(run(messages.TriggeringPolicy{MaxBatchSize: -1}),
			ShouldErrLike, "max_batch_size should be positive, got -1")
	})
}
