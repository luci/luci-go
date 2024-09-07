// Copyright 2024 The LUCI Authors.
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

package truth

import (
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Option is an interface to allow test authors to modify the behavior of
// {assert,check}.{That,Loosely} invocations.
//
// A nil Option has no effect.
//
// Known Options:
//   - truth.LineContext
//   - truth.SummaryModifier
type Option interface {
	truthOption()
}

// ApplyAllOptions applies all Options to the given failure.Summary.
//
// This is a no-op if `summary` is nil.
//
// This is a fairly low-level function - typically options will be applied for
// you as part of {check,assert}.{That,Loosely}.
//
// For convenience, returns `summary`.
func ApplyAllOptions(summary *failure.Summary, opts []Option) *failure.Summary {
	if summary == nil || len(opts) == 0 {
		return summary
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		switch x := opt.(type) {
		case summaryModifier:
			x(summary)
		}
	}
	return summary
}
