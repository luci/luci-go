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

package gs

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPath(t *testing.T) {
	t.Parallel()

	Convey(`Path manipulation tests`, t, func() {
		for _, tc := range []struct {
			prefix   string
			filename string
			path     Path
		}{
			{"", "", ""},
			{"bucket/", "", "gs://bucket"},
			{"bucket", "foo/bar", "gs://bucket/foo/bar"},
			{"", "foo/bar", "foo/bar"},
			{"", "/foo/bar///", "/foo/bar///"},
		} {
			Convey(fmt.Sprintf(`Test path: prefix=%q, filename=%q, path=%q`, tc.prefix, tc.filename, tc.path), func() {
				Convey(`The prefix and filenames compose into the path.`, func() {
					So(MakePath(tc.prefix, tc.filename), ShouldEqual, tc.path)
				})

				Convey(`The path splits into prefix and filename.`, func() {
					p, f := tc.path.Split()
					So(p, ShouldEqual, strings.TrimSuffix(tc.prefix, "/"))
					So(f, ShouldEqual, tc.filename)
				})
			})
		}
	})

	Convey(`Concat tests`, t, func() {
		for _, tc := range []struct {
			orig   Path
			concat []string
			final  Path
		}{
			{"gs://foo/bar", []string{"baz"}, "gs://foo/bar/baz"},
			{"foo/bar", []string{"baz"}, "foo/bar/baz"},
			{"/foo/bar/", []string{"/baz/"}, "/foo/bar//baz"},
			{"gs://bucket", []string{"baz"}, "gs://bucket/baz"},
			{"gs://bucket/", []string{"baz"}, "gs://bucket/baz"},
			{"gs://bucket/foo/", []string{"baz"}, "gs://bucket/foo/baz"},
			{"gs://bucket/foo/", []string{"bar//", "baz"}, "gs://bucket/foo/bar/baz"},
		} {
			Convey(fmt.Sprintf(`Concat: %q to %q yields %q`, tc.orig, tc.concat, tc.final), func() {
				So(tc.orig.Concat(tc.concat[0], tc.concat[1:]...), ShouldEqual, tc.final)
			})
		}
	})
}
