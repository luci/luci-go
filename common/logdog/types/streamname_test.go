// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package types

import (
	"flag"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStreamNameAsFlag(t *testing.T) {
	Convey(`Given an FlagSet configured with a StreamName flag`, t, func() {
		var stream StreamName
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Var(&stream, "stream", "The stream name.")

		Convey(`When the stream flag is set to "test"`, func() {
			err := fs.Parse([]string{"-stream", "test"})
			So(err, ShouldBeNil)

			Convey(`The stream variable should be populated with "test".`, func() {
				So(stream, ShouldEqual, "test")
			})
		})

		Convey(`An invalid stream name should fail to parse.`, func() {
			err := fs.Parse([]string{"-stream", "_beginsWithUnderscore"})
			So(err, ShouldNotBeNil)
		})
	})
}

func TestStreamName(t *testing.T) {
	Convey(`MakeStreamName`, t, func() {
		type e struct {
			t []string // Test value.
			e string   // Expected value.
		}
		for _, entry := range []e{
			{[]string{}, "FILL"},
			{[]string{""}, "FILL"},
			{[]string{"", ""}, "FILL/FILL"},
			{[]string{"", "foo", "ba!r", "¿baz"}, "FILL/foo/ba_r/FILL_baz"},
			{[]string{"foo", "bar baz"}, "foo/bar_baz"},
		} {
			Convey(fmt.Sprintf(`Transforms "%#v" into "%s".`, entry.t, entry.e), func() {
				s := MakeStreamName("FILL", entry.t...)
				So(s, ShouldEqual, StreamName(entry.e))
				So(s.Validate(), ShouldBeNil)
			})
		}
	})

	Convey(`StreamName.Trim`, t, func() {
		type e struct {
			t string // Test value.
			e string // Expected value.
		}
		for _, entry := range []e{
			{``, ``},
			{`foo/bar`, `foo/bar`},
			{`/foo/bar`, `foo/bar`},
			{`foo/bar/`, `foo/bar`},
			{`//foo//bar///`, `foo//bar`},
		} {
			Convey(fmt.Sprintf(`On "%s", returns "%s".`, entry.t, entry.e), func() {
				So(StreamName(entry.t).Trim(), ShouldEqual, entry.e)
			})
		}
	})

	Convey(`StreamName.Validate`, t, func() {
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
				Convey(fmt.Sprintf(`"%s" is valid.`, entry.t), func() {
					So(StreamName(entry.t).Validate(), ShouldBeNil)
				})
			} else {
				Convey(fmt.Sprintf(`"%s" is invalid.`, entry.t), func() {
					So(StreamName(entry.t).Validate(), ShouldNotBeNil)
				})
			}
		}
	})

	Convey(`StreamName.Join`, t, func() {
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
			Convey(fmt.Sprintf(`Joining "%s" to "%s" yields "%s".`, entry.a, entry.b, entry.e), func() {
				So(StreamName(entry.a).Join(StreamName(entry.b)), ShouldEqual, StreamPath(entry.e))
			})
		}
	})

	Convey(`StreamName.Segments, StreamName.SegmentCount`, t, func() {
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
			Convey(fmt.Sprintf(`Stream Name "%s" has %d segments: %v`, entry.s, entry.n, entry.p), func() {
				So(entry.s.Segments(), ShouldResemble, entry.p)
				So(len(entry.s.Segments()), ShouldEqual, entry.s.SegmentCount())
				So(len(entry.s.Segments()), ShouldEqual, entry.n)
			})
		}
	})
}

func TestStreamPath(t *testing.T) {
	Convey(`StreamPath.Split, StreamPath.Validate`, t, func() {
		type e struct {
			p      string // The stream path.
			prefix string // The split prefix.
			tail   string // The split tail.
			valid  bool
		}
		for _, entry := range []e{
			{"a/+/b", "a", "b", true},
			{"a/+/b/+/c", "a", "b/+/c", false},
			{"/+/", "", "", false},
			{"foo/bar/+/baz/qux", "foo/bar", "baz/qux", true},
		} {
			Convey(fmt.Sprintf(`Stream Path "%s" splits into "%s" and "%s".`, entry.p, entry.prefix, entry.tail), func() {
				prefix, tail := StreamPath(entry.p).Split()
				So(prefix, ShouldEqual, entry.prefix)
				So(tail, ShouldEqual, entry.tail)
			})

			if entry.valid {
				Convey(fmt.Sprintf(`Stream Path "%s" is valid`, entry.p), func() {
					So(StreamPath(entry.p).Validate(), ShouldBeNil)
				})
			} else {
				Convey(fmt.Sprintf(`Stream Path "%s" is not valid`, entry.p), func() {
					So(StreamPath(entry.p).Validate(), ShouldNotBeNil)
				})
			}
		}
	})
}
