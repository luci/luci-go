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

package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestElideNestedPaths(t *testing.T) {
	t.Parallel()
	doElision := func(deps []string) []string {
		// Ignore OS-dependent path sep
		return elideNestedPaths(deps, "/")
	}

	Convey(`Mixed`, t, func() {
		deps := []string{
			"ab/foo",
			"ab/",
			"foo",
			"b/c/",
			"b/a",
			"b/c/a",
			"ab/cd/",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/", "b/a", "b/c/", "foo"})
	})

	Convey(`All files`, t, func() {
		deps := []string{
			"ab/foo",
			"ab/cd/foo",
			"foo",
			"ab/bar",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/bar", "ab/cd/foo", "ab/foo", "foo"})
	})

	Convey(`Cousin paths`, t, func() {
		deps := []string{
			"ab/foo", // This is a file
			"ab/cd/",
			"ab/ef/",
			"ab/bar",
		}
		So(doElision(deps), ShouldResemble, []string{"ab/bar", "ab/cd/", "ab/ef/", "ab/foo"})
	})

	Convey(`Interesting dirs`, t, func() {
		deps := []string{
			"a/b/",
			"a/b/c/",
			"a/bc/",
			"a/bc/d/",
			"a/bcd/",
			"a/c/",
		}
		// Make sure:
		// 1. "a/b/" elides "a/b/c/", but not "a/bc/"
		// 2. "a/bc/" elides "a/bc/d/", but not "a/bcd/"
		So(doElision(deps), ShouldResemble, []string{"a/b/", "a/bc/", "a/bcd/", "a/c/"})
	})

	Convey(`Interesting files`, t, func() {
		deps := []string{
			"a/b",
			"a/bc",
			"a/bcd",
			"a/c",
		}
		// Make sure "a/b" elides neither "a/bc" nor "a/bcd"
		So(doElision(deps), ShouldResemble, []string{"a/b", "a/bc", "a/bcd", "a/c"})
	})
}
