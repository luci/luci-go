// Copyright 2020 The LUCI Authors.
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

package cqdepend

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParser(t *testing.T) {
	t.Parallel()

	ftt.Run("Parse works", t, func(t *ftt.Test) {
		t.Run("Basic", func(t *ftt.Test) {
			assert.Loosely(t, Parse("Nothing\n\ninteresting."), should.BeNil)
			assert.That(t, Parse("Title.\n\nCq-Depend: 456,123"), should.Match([]Dep{
				{Subdomain: "", Change: 123},
				{Subdomain: "", Change: 456},
			}))
		})
		t.Run("Case and space incensitive", func(t *ftt.Test) {
			assert.That(t, Parse("Title.\n\nCQ-dePend: Any-case:456 , Zx:23 "), should.Match([]Dep{
				{Subdomain: "any-case", Change: 456},
				{Subdomain: "zx", Change: 23},
			}))
		})
		t.Run("Dedup and multiline", func(t *ftt.Test) {
			assert.That(t, Parse("Title.\n\nCq-Depend: 456,123\nCq-Depend: 123,y:456"), should.Match([]Dep{
				{Subdomain: "", Change: 123},
				{Subdomain: "", Change: 456},
				{Subdomain: "y", Change: 456},
			}))
		})
		t.Run("Ignores errors", func(t *ftt.Test) {
			assert.That(t, Parse("Title.\n\nCq-Depend: 2, x;3\nCq-Depend: y-review:4,z:5"), should.Match([]Dep{
				{Subdomain: "", Change: 2},
				{Subdomain: "z", Change: 5},
			}))
		})
		t.Run("Ignores non-footers", func(t *ftt.Test) {
			assert.Loosely(t, Parse("Cq-Depend: 1\n\nCq-Depend: i:2\n\nChange-Id: Ideadbeef"), should.BeNil)
		})
	})
}

func TestParseSingleDep(t *testing.T) {
	t.Parallel()

	ftt.Run("parseSingleDep works", t, func(t *ftt.Test) {
		t.Run("OK", func(t *ftt.Test) {
			d, err := parseSingleDep(" x:123 ")
			assert.NoErr(t, err)
			assert.That(t, d, should.Match(Dep{Change: 123, Subdomain: "x"}))

			d, err = parseSingleDep("123")
			assert.NoErr(t, err)
			assert.That(t, d, should.Match(Dep{Change: 123, Subdomain: ""}))
		})
		t.Run("Invalid format", func(t *ftt.Test) {
			_, err := parseSingleDep("weird/value:here")
			assert.Loosely(t, err, should.ErrLike("must match"))
			_, err = parseSingleDep("https://abc.example.com:123")
			assert.Loosely(t, err, should.ErrLike("must match"))
			_, err = parseSingleDep("abc-review.example.com:1")
			assert.Loosely(t, err, should.ErrLike("must match"))
			_, err = parseSingleDep("no-spaces-around-colon :1")
			assert.Loosely(t, err, should.ErrLike("must match"))
			_, err = parseSingleDep("no-spaces-around-colon: 2")
			assert.Loosely(t, err, should.ErrLike("must match"))
		})
		t.Run("Too large", func(t *ftt.Test) {
			_, err := parseSingleDep("12312123123123123123")
			assert.Loosely(t, err, should.ErrLike("change number too large"))
		})
		t.Run("Disallow -Review", func(t *ftt.Test) {
			_, err := parseSingleDep("x-review:1")
			assert.Loosely(t, err, should.ErrLike("must not include '-review'"))
		})
	})
}
