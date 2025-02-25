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

package units

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSizeToString(t *testing.T) {
	t.Parallel()
	ftt.Run(`A size unit should convert to a valid string representation.`, t, func(t *ftt.Test) {
		data := []struct {
			in       int64
			expected string
		}{
			{0, "0B"},
			{1, "1B"},
			{1000, "1000B"},
			{1023, "1023B"},
			{1024, "1.00KiB"},
			{1029, "1.00KiB"},
			{1030, "1.01KiB"},
			{10234, "9.99KiB"},
			{10239, "10.00KiB"},
			{10240, "10.0KiB"},
			{1048575, "1024.0KiB"},
			{1048576, "1.00MiB"},
		}
		for _, line := range data {
			assert.Loosely(t, SizeToString(line.in), should.Match(line.expected))
		}
	})
}
