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

package types

import (
	"flag"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStreamNameAsFlag(t *testing.T) {
	t.Parallel()

	ftt.Run(`Given an FlagSet configured with a StreamName flag`, t, func(t *ftt.Test) {
		var stream StreamName
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Var(&stream, "stream", "The stream name.")

		t.Run(`When the stream flag is set to "test"`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-stream", "test"})
			assert.Loosely(t, err, should.BeNil)

			t.Run(`The stream variable should be populated with "test".`, func(t *ftt.Test) {
				assert.Loosely(t, stream, should.Equal(StreamName("test")))
			})
		})

		t.Run(`An invalid stream name should fail to parse.`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-stream", "_beginsWithUnderscore"})
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestStreamName(t *testing.T) {
	t.Parallel()

	ftt.Run(`MakeStreamName`, t, func(t *ftt.Test) {
		type e struct {
			t []string // Test value.
			e string   // Expected value.
		}
		for _, entry := range []e{
			{[]string{""}, "FILL"},
			{[]string{"", ""}, "FILL/FILL"},
			{[]string{"", "foo", "ba!r", "¿baz"}, "FILL/foo/ba_r/FILL_baz"},
			{[]string{"foo", "bar baz"}, "foo/bar_baz"},
		} {
			t.Run(fmt.Sprintf(`Transforms "%#v" into "%s".`, entry.t, entry.e), func(t *ftt.Test) {
				s, err := MakeStreamName("FILL", entry.t...)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Equal(StreamName(entry.e)))
				assert.Loosely(t, s.Validate(), should.BeNil)
			})
		}

		t.Run(`Fails if the fill string is not a valid path.`, func(t *ftt.Test) {
			_, err := MakeStreamName("__not_a_path", "", "b", "c")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = MakeStreamName("", "", "b", "c")
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Doesn't care about the fill string if it's not needed.`, func(t *ftt.Test) {
			_, err := MakeStreamName("", "a", "b", "c")
			assert.Loosely(t, err, should.BeNil)
		})
	})

	ftt.Run(`StreamName.Trim`, t, func(t *ftt.Test) {
		type e struct {
			t string     // Test value.
			e StreamName // Expected value.
		}
		for _, entry := range []e{
			{``, ``},
			{`foo/bar`, `foo/bar`},
			{`/foo/bar`, `foo/bar`},
			{`foo/bar/`, `foo/bar`},
			{`//foo//bar///`, `foo//bar`},
		} {
			t.Run(fmt.Sprintf(`On "%s", returns "%s".`, entry.t, entry.e), func(t *ftt.Test) {
				assert.Loosely(t, StreamName(entry.t).Trim(), should.Equal(entry.e))
			})
		}
	})

	ftt.Run(`StreamName.Namespaces`, t, func(t *ftt.Test) {
		type e struct {
			t string       // Test value.
			e []StreamName // Expected value.
		}
		for _, entry := range []e{
			{``, []StreamName{``}},
			{`foo`, []StreamName{``}},
			{`foo/bar`, []StreamName{`foo/`, ``}},
			{`foo/bar/baz`, []StreamName{`foo/bar/`, `foo/`, ``}},

			// This is malformed input (GIGO), but don't want infinite loop
			{`/`, []StreamName{`/`, ``}},
		} {
			t.Run(fmt.Sprintf(`On "%s", returns "%s".`, entry.t, entry.e), func(t *ftt.Test) {
				assert.Loosely(t, StreamName(entry.t).Namespaces(), should.Resemble(entry.e))
			})
		}
	})

	ftt.Run(`StreamName.AsNamespace`, t, func(t *ftt.Test) {
		type e struct {
			t string     // Test value.
			e StreamName // Expected value.
		}
		for _, entry := range []e{
			{``, ``},
			{`foo`, `foo/`},
			{`foo/`, `foo/`},
			{`foo/bar`, `foo/bar/`},
			{`foo/bar/`, `foo/bar/`},
		} {
			t.Run(fmt.Sprintf(`On "%s", returns "%s".`, entry.t, entry.e), func(t *ftt.Test) {
				assert.Loosely(t, StreamName(entry.t).AsNamespace(), should.Equal(entry.e))
			})
		}
	})

	ftt.Run(`StreamName.Validate`, t, func(t *ftt.Test) {
		type e struct {
			t string // Test value
			v bool   // Is the test value expected to be valid?
		}
		for _, entry := range []e{
			{``, false},         // Empty
			{`/foo/bar`, false}, // Begins with separator.
			{`foo/bar/`, false}, // Ends with separator.
			{`foo/bar`, true},
			{`foo//bar`, false}, // Double separator.
			{`foo^bar`, false},  // Illegal character.
			{`abcdefghijklmnopqrstuvwxyz/ABCDEFGHIJKLMNOPQRSTUVWXYZ/0123456789/a:_-.`, true},
			{`_foo/bar`, false},     // Does not begin with alphanumeric.
			{`foo/_bar/baz`, false}, // Segment begins with non-alphanumeric.
		} {
			if entry.v {
				t.Run(fmt.Sprintf(`"%s" is valid.`, entry.t), func(t *ftt.Test) {
					assert.Loosely(t, StreamName(entry.t).Validate(), should.BeNil)
				})
			} else {
				t.Run(fmt.Sprintf(`"%s" is invalid.`, entry.t), func(t *ftt.Test) {
					assert.Loosely(t, StreamName(entry.t).Validate(), should.NotBeNil)
				})
			}
		}

		t.Run(`A stream name that is too long will not validate.`, func(t *ftt.Test) {
			assert.Loosely(t, StreamName(strings.Repeat("A", MaxStreamNameLength)).Validate(), should.BeNil)
			assert.Loosely(t, StreamName(strings.Repeat("A", MaxStreamNameLength+1)).Validate(), should.NotBeNil)
		})
	})

	ftt.Run(`StreamName.Join`, t, func(t *ftt.Test) {
		type e struct {
			a string // Initial stream name.
			b string // Join value.
			e string // Expected value.
		}
		for _, entry := range []e{
			{"foo", "bar", "foo/+/bar"},
			{"", "", "/+/"},
			{"foo", "", "foo/+/"},
			{"", "bar", "/+/bar"},
			{"/foo/", "/bar/baz/", "foo/+/bar/baz"},
		} {
			t.Run(fmt.Sprintf(`Joining "%s" to "%s" yields "%s".`, entry.a, entry.b, entry.e), func(t *ftt.Test) {
				assert.Loosely(t, StreamName(entry.a).Join(StreamName(entry.b)), should.Equal(StreamPath(entry.e)))
			})
		}
	})

	ftt.Run(`StreamName.Segments, StreamName.SegmentCount`, t, func(t *ftt.Test) {
		type e struct {
			s StreamName // Initial stream name.
			p []string   // Expected split pieces.
			n int        // Expected number of segments.
		}
		for _, entry := range []e{
			{StreamName(""), []string(nil), 0},
			{StreamName("foo"), []string{"foo"}, 1},
			{StreamName("foo/bar"), []string{"foo", "bar"}, 2},
			{StreamName("foo/bar/baz"), []string{"foo", "bar", "baz"}, 3},
		} {
			t.Run(fmt.Sprintf(`Stream Name "%s" has %d segments: %v`, entry.s, entry.n, entry.p), func(t *ftt.Test) {
				assert.Loosely(t, entry.s.Segments(), should.Resemble(entry.p))
				assert.Loosely(t, len(entry.s.Segments()), should.Equal(entry.s.SegmentCount()))
				assert.Loosely(t, len(entry.s.Segments()), should.Equal(entry.n))
			})
		}
	})

	ftt.Run(`StreamName.Split`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			s    StreamName
			base StreamName
			last StreamName
		}{
			{"", "", ""},
			{"foo", "", "foo"},
			{"/foo", "", "foo"},
			{"foo/bar", "foo", "bar"},
			{"foo/bar/baz", "foo/bar", "baz"},
		} {
			t.Run(fmt.Sprintf(`Stream name %q splits into %q and %q.`, tc.s, tc.base, tc.last), func(t *ftt.Test) {
				base, last := tc.s.Split()
				assert.Loosely(t, base, should.Equal(tc.base))
				assert.Loosely(t, last, should.Equal(tc.last))
			})
		}
	})
}

func TestStreamPath(t *testing.T) {
	t.Parallel()

	ftt.Run(`StreamPath.Split, StreamPath.Validate`, t, func(t *ftt.Test) {
		type e struct {
			p      string     // The stream path.
			prefix StreamName // The split prefix.
			name   StreamName // The split name.
			sep    bool
			valid  bool
		}
		for _, entry := range []e{
			{"", "", "", false, false},
			{"foo/+", "foo", "", true, false},
			{"+foo", "+foo", "", false, false},
			{"+/foo", "", "foo", true, false},
			{"a/+/b", "a", "b", true, true},
			{"a/+/", "a", "", true, false},
			{"a/+foo/+/b", "a/+foo", "b", true, false},
			{"fo+o/bar+/+/baz", "fo+o/bar+", "baz", true, false},
			{"a/+/b/+/c", "a", "b/+/c", true, false},
			{"/+/", "", "", true, false},
			{"foo/bar/+/baz/qux", "foo/bar", "baz/qux", true, true},
		} {
			t.Run(fmt.Sprintf(`Stream Path "%s" splits into "%s" and "%s".`, entry.p, entry.prefix, entry.name), func(t *ftt.Test) {
				prefix, sep, name := StreamPath(entry.p).SplitParts()
				assert.Loosely(t, prefix, should.Equal(entry.prefix))
				assert.Loosely(t, sep, should.Equal(entry.sep))
				assert.Loosely(t, name, should.Equal(entry.name))
			})

			if entry.valid {
				t.Run(fmt.Sprintf(`Stream Path "%s" is valid`, entry.p), func(t *ftt.Test) {
					assert.Loosely(t, StreamPath(entry.p).Validate(), should.BeNil)
				})
			} else {
				t.Run(fmt.Sprintf(`Stream Path "%s" is not valid`, entry.p), func(t *ftt.Test) {
					assert.Loosely(t, StreamPath(entry.p).Validate(), should.NotBeNil)
				})
			}
		}
	})

	ftt.Run(`StreamPath.SplitLast`, t, func(t *ftt.Test) {
		for _, tc := range []struct {
			p StreamPath
			c []string
		}{
			{"", []string{""}},
			{"/", []string{"/"}},
			{"foo", []string{"foo"}},
			{"foo/bar/+/baz", []string{"baz", "+", "bar", "foo"}},
			{"/foo/+/bar/", []string{"", "bar", "+", "/foo"}},
		} {
			t.Run(fmt.Sprintf(`Splitting %q repeatedly yields %v`, tc.p, tc.c), func(t *ftt.Test) {
				var parts []string
				p := tc.p
				for {
					var end string
					p, end = p.SplitLast()
					parts = append(parts, end)

					if p == "" {
						break
					}
				}
				assert.Loosely(t, parts, should.Resemble(tc.c))
			})
		}
	})
}
