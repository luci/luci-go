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
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestElideNestedPaths(t *testing.T) {
	checkIsDir := func(path string) (bool, error) {
		return strings.HasSuffix(path, "/"), nil
	}

	Convey(`Mixed`, t, func() {
		deps := []string{
			"ab/foo.txt",
			"ab/",
			"foo.txt",
			"b/c/",
			"ab/cd/",
			"bar.txt",
		}
		elided, err := elideNestedPaths(deps, checkIsDir)
		So(err, ShouldBeNil)
		So(elided, ShouldResemble, []string{"ab/", "b/c/", "bar.txt", "foo.txt"})
	})

	Convey(`All files`, t, func() {
		deps := []string{"ab/foo.txt", "ab/cd/foo.txt", "foo.txt", "ab/bar.txt"}
		elided, err := elideNestedPaths(deps, checkIsDir)
		So(err, ShouldBeNil)
		So(elided, ShouldResemble, []string{"ab/bar.txt", "ab/cd/foo.txt", "ab/foo.txt", "foo.txt"})
	})

	Convey(`Cousin paths`, t, func() {
		deps := []string{
			"ab/foo.txt",
			"ab/cd/",
			"ab/ef/",
			"ab/bar.txt",
		}
		elided, err := elideNestedPaths(deps, checkIsDir)
		So(err, ShouldBeNil)
		So(elided, ShouldResemble, []string{"ab/bar.txt", "ab/cd/", "ab/ef/", "ab/foo.txt"})
	})

}
