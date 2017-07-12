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

	. "github.com/smartystreets/goconvey/convey"
)

func TestNestedFlagSet(t *testing.T) {
	Convey("Given a multi-field FlagSet object", t, func() {
		nfs := FlagSet{}
		s := nfs.F.String("field-str", "", "String field.")
		i := nfs.F.Int("field-int", 0, "Integer field.")
		d := nfs.F.String("field-default", "default",
			"Another string field.")

		Convey("When parsed with a valid field set", func() {
			err := nfs.Parse("field-str=foo,field-int=123")
			Convey("It should parse without error.", FailureHalts, func() {
				So(err, ShouldBeNil)
			})

			Convey("It should parse 'field-str' as 'foo'", func() {
				So(*s, ShouldEqual, "foo")
			})

			Convey("It should parse 'field-int' as 123", func() {
				So(*i, ShouldEqual, 123)
			})

			Convey("It should leave 'field-default' to its default value, 'default'", func() {
				So(*d, ShouldEqual, "default")
			})
		})

		Convey("When parsed with an unexpected field", func() {
			err := nfs.Parse("field-invalid=foo")

			Convey("It should error.", FailureHalts, func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("It should return a valid usage string.", func() {
			So(nfs.Usage(), ShouldEqual, "help[,field-default][,field-int][,field-str]")
		})

		Convey(`When installed as a flag`, func() {
			fs := flag.NewFlagSet("test", flag.PanicOnError)
			fs.Var(&nfs, "flagset", "The FlagSet instance.")

			Convey(`Accepts the FlagSet as a parameter.`, func() {
				fs.Parse([]string{"-flagset", `field-str="hello",field-int=20`})
				So(*s, ShouldEqual, "hello")
				So(*i, ShouldEqual, 20)
				So(*d, ShouldEqual, "default")
			})
		})
	})
}

// Example to demonstrate NestedFlagSet usage.
func ExampleNestedFlagSet() {
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
