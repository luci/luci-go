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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPath(t *testing.T) {
	t.Parallel()

	ftt.Run(`Path manipulation tests`, t, func(t *ftt.Test) {
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
			t.Run(fmt.Sprintf(`Test path: prefix=%q, filename=%q, path=%q`, tc.prefix, tc.filename, tc.path), func(t *ftt.Test) {
				t.Run(`The prefix and filenames compose into the path.`, func(t *ftt.Test) {
					assert.Loosely(t, MakePath(tc.prefix, tc.filename), should.Equal(tc.path))
				})

				t.Run(`The path splits into prefix and filename.`, func(t *ftt.Test) {
					p, f := tc.path.Split()
					assert.Loosely(t, p, should.Equal(strings.TrimSuffix(tc.prefix, "/")))
					assert.Loosely(t, f, should.Equal(tc.filename))
				})
			})
		}
	})

	ftt.Run(`Concat tests`, t, func(t *ftt.Test) {
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
			t.Run(fmt.Sprintf(`Concat: %q to %q yields %q`, tc.orig, tc.concat, tc.final), func(t *ftt.Test) {
				assert.Loosely(t, tc.orig.Concat(tc.concat[0], tc.concat[1:]...), should.Equal(tc.final))
			})
		}
	})
}
