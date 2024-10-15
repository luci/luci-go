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

package nestedflagset

import (
	"flag"
	"fmt"

	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNestedFlagSet(t *testing.T) {
	ftt.Run("Given a multi-field FlagSet object", t, func(t *ftt.Test) {
		nfs := FlagSet{}
		s := nfs.F.String("field-str", "", "String field.")
		i := nfs.F.Int("field-int", 0, "Integer field.")
		d := nfs.F.String("field-default", "default",
			"Another string field.")

		t.Run("When parsed with a valid field set", func(t *ftt.Test) {
			err := nfs.Parse("field-str=foo,field-int=123")
			t.Run("It should parse without error.", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("It should parse 'field-str' as 'foo'", func(t *ftt.Test) {
				assert.Loosely(t, *s, should.Equal("foo"))
			})

			t.Run("It should parse 'field-int' as 123", func(t *ftt.Test) {
				assert.Loosely(t, *i, should.Equal(123))
			})

			t.Run("It should leave 'field-default' to its default value, 'default'", func(t *ftt.Test) {
				assert.Loosely(t, *d, should.Equal("default"))
			})
		})

		t.Run("When parsed with an unexpected field", func(t *ftt.Test) {
			err := nfs.Parse("field-invalid=foo")

			t.Run("It should error.", func(t *ftt.Test) {
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run("It should return a valid usage string.", func(t *ftt.Test) {
			assert.Loosely(t, nfs.Usage(), should.Equal("help[,field-default][,field-int][,field-str]"))
		})

		t.Run(`When installed as a flag`, func(t *ftt.Test) {
			fs := flag.NewFlagSet("test", flag.PanicOnError)
			fs.Var(&nfs, "flagset", "The FlagSet instance.")

			t.Run(`Accepts the FlagSet as a parameter.`, func(t *ftt.Test) {
				fs.Parse([]string{"-flagset", `field-str="hello",field-int=20`})
				assert.Loosely(t, *s, should.Equal("hello"))
				assert.Loosely(t, *i, should.Equal(20))
				assert.Loosely(t, *d, should.Equal("default"))
			})
		})
	})
}

// ExampleFlagSet demonstrates nestedflagset.FlagSet usage.
func ExampleFlagSet() {
	nfs := &FlagSet{}
	s := nfs.F.String("str", "", "Nested string option.")
	i := nfs.F.Int("int", 0, "Nested integer option.")

	if err := nfs.Parse(`str="Hello, world!",int=10`); err != nil {
		panic(err)
	}
	fmt.Printf("Parsed str=[%s], int=%d.\n", *s, *i)

	// Output:
	// Parsed str=[Hello, world!], int=10.
}
