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

package streamproto

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTagMapFlag(t *testing.T) {
	ftt.Run(`An empty TagMap`, t, func(t *ftt.Test) {
		tm := TagMap{}

		t.Run(`SortedKeys will return nil.`, func(t *ftt.Test) {
			assert.Loosely(t, tm.SortedKeys(), should.BeNil)
		})

		t.Run(`When used as a flag`, func(t *ftt.Test) {
			fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
			fs.Var(&tm, "tag", "Testing tag.")

			t.Run(`Can successfully parse multiple parameters.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-tag", "foo=FOO", "-tag", "bar=BAR", "-tag", "baz"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tm, should.Match(TagMap{"foo": "FOO", "bar": "BAR", "baz": ""}))

				t.Run(`Will build a correct string.`, func(t *ftt.Test) {
					assert.Loosely(t, tm.String(), should.Equal(`bar=BAR,baz,foo=FOO`))
				})
			})

			t.Run(`Loaded with {"foo": "bar", "baz": "qux"}`, func(t *ftt.Test) {
				tm["foo"] = "bar"
				tm["baz"] = "qux"

				t.Run(`Can be converted into JSON.`, func(t *ftt.Test) {
					d, err := json.Marshal(&tm)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, string(d), should.Equal(`{"baz":"qux","foo":"bar"}`))

					t.Run(`And can be unmarshalled from JSON.`, func(t *ftt.Test) {
						tm := TagMap{}
						err := json.Unmarshal(d, &tm)
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, tm, should.Match(TagMap{
							"foo": "bar",
							"baz": "qux",
						}))
					})
				})
			})

			t.Run(`An empty TagMap JSON will unmarshal into nil.`, func(t *ftt.Test) {
				tm := TagMap{}
				err := json.Unmarshal([]byte(`{}`), &tm)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tm, should.BeNil)
			})

			for _, s := range []string{
				`[{"woot": "invalid"}]`,
				`[{123: abc}]`,
				`[{"key": "invalidl;tag;name"}]`,
			} {
				t.Run(fmt.Sprintf(`Invalid TagMap JSON will fail: %q`, s), func(t *ftt.Test) {
					tm := TagMap{}
					err := json.Unmarshal([]byte(s), &tm)
					assert.Loosely(t, err, should.NotBeNil)
				})
			}
		})
	})
}
