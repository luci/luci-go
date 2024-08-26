// Copyright 2017 The LUCI Authors.
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

package stringtemplate

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing substitution resolution`, t, func(t *ftt.Test) {
		shouldResolve := func(v string, subst map[string]string) string {
			val, err := Resolve(v, subst)
			assert.Loosely(t, err, should.BeNil)
			return val
		}

		resolveErr := func(v string) error {
			_, err := Resolve(v, nil)
			return err
		}

		t.Run(`Can resolve a string without any substitutions.`, func(t *ftt.Test) {
			assert.Loosely(t, shouldResolve("", nil), should.BeEmpty)
			assert.Loosely(t, shouldResolve("foo", nil), should.Equal("foo"))
			assert.Loosely(t, shouldResolve(`$${foo}`, nil), should.Equal(`${foo}`))
		})

		t.Run(`Will error if there is an invalid substitution.`, func(t *ftt.Test) {
			assert.Loosely(t, resolveErr("$"), should.ErrLike("invalid template"))
			assert.Loosely(t, resolveErr("hello$"), should.ErrLike("invalid template"))
			assert.Loosely(t, resolveErr("foo-${bar"), should.ErrLike("invalid template"))
			assert.Loosely(t, resolveErr("${not valid}"), should.ErrLike("invalid template"))

			assert.Loosely(t, resolveErr("${uhoh}"), should.ErrLike("no substitution for"))
			assert.Loosely(t, resolveErr("$noooo"), should.ErrLike("no substitution for"))
		})

		t.Run(`With substitutions defined, can apply.`, func(t *ftt.Test) {
			m := map[string]string{
				"pants":      "shirt",
				"wear_pants": "12345",
			}

			assert.Loosely(t, shouldResolve("", m), should.BeEmpty)

			assert.Loosely(t, shouldResolve("$pants", m), should.Equal("shirt"))
			assert.Loosely(t, shouldResolve("foo/$pants", m), should.Equal("foo/shirt"))

			assert.Loosely(t, shouldResolve("${wear_pants}", m), should.Equal("12345"))
			assert.Loosely(t, shouldResolve("foo/${wear_pants}", m), should.Equal("foo/12345"))
			assert.Loosely(t, shouldResolve("foo/${wear_pants}/bar", m), should.Equal("foo/12345/bar"))
			assert.Loosely(t, shouldResolve("${wear_pants}/bar", m), should.Equal("12345/bar"))
			assert.Loosely(t, shouldResolve("foo/${wear_pants}/bar/${wear_pants}/baz", m), should.Equal("foo/12345/bar/12345/baz"))

			assert.Loosely(t, shouldResolve(`$$pants`, m), should.Equal("$pants"))
			assert.Loosely(t, shouldResolve(`$${pants}`, m), should.Equal("${pants}"))
			assert.Loosely(t, shouldResolve(`$$$pants`, m), should.Equal("$shirt"))
			assert.Loosely(t, shouldResolve(`$$${pants}`, m), should.Equal("$shirt"))
			assert.Loosely(t, shouldResolve(`foo/${wear_pants}/bar/$${wear_pants}/baz`, m), should.Equal(`foo/12345/bar/${wear_pants}/baz`))
		})
	})
}
