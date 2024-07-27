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

package identity

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGlob(t *testing.T) {
	ftt.Run("MakeGlob works", t, func(t *ftt.Test) {
		g, err := MakeGlob("user:*@example.com")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, g, should.Equal(Glob("user:*@example.com")))
		assert.Loosely(t, g.Kind(), should.Equal(User))
		assert.Loosely(t, g.Pattern(), should.Equal("*@example.com"))

		_, err = MakeGlob("bad ident")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Validate works", t, func(t *ftt.Test) {
		assert.Loosely(t, Glob("user:*@example.com").Validate(), should.BeNil)
		assert.Loosely(t, Glob("user:").Validate(), should.NotBeNil)
		assert.Loosely(t, Glob(":abc").Validate(), should.NotBeNil)
		assert.Loosely(t, Glob("abc@example.com").Validate(), should.NotBeNil)
		assert.Loosely(t, Glob("user:\n").Validate(), should.NotBeNil)
	})

	ftt.Run("Kind works", t, func(t *ftt.Test) {
		assert.Loosely(t, Glob("user:*@example.com").Kind(), should.Equal(User))
		assert.Loosely(t, Glob("???").Kind(), should.Equal(Anonymous))
	})

	ftt.Run("Pattern works", t, func(t *ftt.Test) {
		assert.Loosely(t, Glob("service:*").Pattern(), should.Equal("*"))
		assert.Loosely(t, Glob("???").Pattern(), should.BeEmpty)
	})

	ftt.Run("Match works", t, func(t *ftt.Test) {
		trials := []struct {
			g      Glob
			id     Identity
			result bool
		}{
			{"user:abc@example.com", "user:abc@example.com", true},
			{"user:*@example.com", "user:abc@example.com", true},
			{"user:*", "user:abc@example.com", true},
			{"user:prefix-*", "user:prefix-zzz", true},
			{"user:prefix-*", "user:another-prefix-zzz", false},
			{"user:prefix-*-suffix", "user:prefix-zzz-suffix", true},
			{"user:prefix-*-suffix", "user:prefix-zzz-suffizzz", false},
			{"user:*", "user:\n", false},
			{"user:\n", "user:zzz", false},
			{"bad glob", "user:abc@example.com", false},
			{"user:*", "bad ident", false},
			{"user:*", "service:abc", false},
		}
		for _, entry := range trials {
			assert.Loosely(t, entry.g.Match(entry.id), should.Equal(entry.result))
		}
	})

	ftt.Run("translate works", t, func(t *ftt.Test) {
		trials := []struct {
			pat string
			reg string
		}{
			{"", `^$`},
			{"*", `^.*$`},
			{"abc", `^abc$`},
			{".?", `^\.\?$`},
			{"perfix-*@suffix", `^perfix-.*@suffix$`},
		}
		for _, entry := range trials {
			re, err := translate(entry.pat)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, re, should.Equal(entry.reg))
		}
	})

	ftt.Run("translate reject newline", t, func(t *ftt.Test) {
		_, err := translate("blah\nblah")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Preprocess works", t, func(t *ftt.Test) {
		k, r, err := Glob("user:*@example.com").Preprocess()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, k, should.Equal(User))
		assert.Loosely(t, r, should.Equal("^.*@example\\.com$"))
	})

	ftt.Run("Preprocess rejects malformed identity globs", t, func(t *ftt.Test) {
		_, _, err := Glob("*@example.com").Preprocess()
		assert.Loosely(t, err, should.NotBeNil)
		_, _, err = Glob("unknown:*@example.com").Preprocess()
		assert.Loosely(t, err, should.NotBeNil)
		_, _, err = Glob("user:*\n@example.com").Preprocess()
		assert.Loosely(t, err, should.NotBeNil)
	})
}
