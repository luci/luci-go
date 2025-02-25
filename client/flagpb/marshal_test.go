// Copyright 2016 The LUCI Authors.
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

package flagpb

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func msg(keysValues ...any) map[string]any {
	m := make(map[string]any, len(keysValues)/2)
	for i := 0; i < len(keysValues); i += 2 {
		m[keysValues[i].(string)] = keysValues[i+1]
	}
	return m
}

func repeated(a ...any) []any {
	return a
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	ftt.Run("Marshal", t, func(t *ftt.Test) {
		test := func(m map[string]any, flags ...string) {
			t.Run(strings.Join(flags, " "), func(t *ftt.Test) {
				actualFlags, err := MarshalUntyped(m)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actualFlags, should.Match(flags))
			})
		}

		test(nil)
		test(
			msg("x", 1),
			"-x", "1")
		test(
			msg("x", "a b"),
			"-x", "a b")
		test(
			msg("x", repeated(1, 2)),
			"-x", "1", "-x", "2")
		test(
			msg("b", true),
			"-b")
		test(
			msg("b", false),
			"-b=false")
		test(
			msg("m", msg("x", 1)),
			"-m.x", "1")
		test(
			msg(
				"m", repeated(msg("x", 1), msg("x", 2)),
			),
			"-m.x", "1", "-m", "-m.x", "2")
	})
}
