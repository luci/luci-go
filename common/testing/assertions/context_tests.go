// Copyright 2015 The LUCI Authors.
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

package assertions

import (
	"context"
	"fmt"
	"time"

	"github.com/smartystreets/assertions"
)

// ShouldBeDone tests if the context's .Done() channel is unblocked.
func ShouldBeDone(actual interface{}, expected ...interface{}) string {
	if len(expected) > 0 {
		return fmt.Sprintf("ShouldBeDone requires 0 values, got %d", len(expected))
	}

	if actual == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	ctx, ok := actual.(context.Context)
	if !ok {
		return assertions.ShouldHaveSameTypeAs(actual, context.Context(nil))
	}

	if ctx == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	select {
	case <-ctx.Done():
		return ""
	case <-time.After(time.Millisecond):
		return "Expected context to be Done(), but it wasn't."
	}
}

// ShouldNotBeDone tests if the context's .Done() channel is still blocked.
func ShouldNotBeDone(actual interface{}, expected ...interface{}) string {
	if len(expected) > 0 {
		return fmt.Sprintf("ShouldNotBeDone requires 0 values, got %d", len(expected))
	}

	if actual == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	ctx, ok := actual.(context.Context)
	if !ok {
		return assertions.ShouldHaveSameTypeAs(actual, context.Context(nil))
	}

	if ctx == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	select {
	case <-ctx.Done():
		return "Expected context NOT to be Done(), but it was."
	case <-time.After(time.Millisecond):
		return ""
	}
}
