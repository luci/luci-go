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

package stringmapflag

import (
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValueFlag(t *testing.T) {
	ftt.Run(`An empty Value`, t, func(t *ftt.Test) {
		t.Run(`When used as a flag`, func(t *ftt.Test) {
			var tm Value
			fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
			fs.Var(&tm, "tag", "Testing tag.")

			t.Run(`Can successfully parse multiple parameters.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-tag", "foo=FOO", "-tag", "bar=BAR", "-tag", "baz"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tm, should.Match(Value{"foo": "FOO", "bar": "BAR", "baz": ""}))

				t.Run(`Will build a correct string.`, func(t *ftt.Test) {
					assert.Loosely(t, tm.String(), should.Equal(`bar=BAR,baz,foo=FOO`))
				})
			})

			t.Run(`Will refuse to parse an empty key.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-tag", "="})
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run(`Will refuse to parse an empty tag.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-tag", ""})
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run(`Will refuse to parse duplicate tags.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-tag", "foo=FOO", "-tag", "foo=BAR"})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
	})
}

func Example() {
	v := new(Value)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(v, "option", "Appends a key[=value] option.")

	// Simulate user input.
	if err := fs.Parse([]string{
		"-option", "foo=bar",
		"-option", "baz",
	}); err != nil {
		panic(err)
	}

	m, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println("Parsed options:", string(m))

	// Output:
	// Parsed options: {
	//   "baz": "",
	//   "foo": "bar"
	// }
}
