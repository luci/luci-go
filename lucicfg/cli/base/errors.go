// Copyright 2019 The LUCI Authors.
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

package base

import (
	"fmt"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg"
)

// CollectErrorMessages traverses err (which can be a MultiError, recursively),
// and appends all error messages there to 'in', returning the resulting slice.
//
// They are eventually used in JSON output and printed to stderr.
func CollectErrorMessages(err error, in []string) (out []string) {
	out = in
	if err == nil {
		return
	}

	if err == auth.ErrLoginRequired {
		out = append(out, "Need to login first by running:\nlucicfg auth-login")
		return
	}

	switch e := err.(type) {
	case lucicfg.BacktracableError:
		out = append(out, e.Backtrace())
	case errors.MultiError:
		for _, inner := range e {
			out = CollectErrorMessages(inner, out)
		}
	default:
		out = append(out, fmt.Sprintf("Error: %s", err))
	}
	return
}
