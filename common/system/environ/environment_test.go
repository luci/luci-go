// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package environ

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEnvironmentConversion(t *testing.T) {
	t.Parallel()

	Convey(`Source environment slice translates correctly to/from an Env.`, t, func() {
		env := New([]string{
			"",
			"FOO",
			"BAR=BAZ",
			"bar=baz",
			"qux=quux=quuuuuuux",
		})
		So(env, ShouldResemble, Env{
			env: map[string]string{
				"":    "",
				"FOO": "FOO",
				"BAR": "BAR=BAZ",
				"bar": "bar=baz",
				"qux": "qux=quux=quuuuuuux",
			},
		})

		So(env.Sorted(), ShouldResemble, []string{
			"",
			"BAR=BAZ",
			"FOO",
			"bar=baz",
			"qux=quux=quuuuuuux",
		})
	})
}

func TestEnvironmentManipulation(t *testing.T) {
	t.Parallel()

	Convey(`A zero-valued Env`, t, func() {
		var env Env
		So(env.Len(), ShouldEqual, 0)

		Convey(`Can be sorted.`, func() {
			So(env.Sorted(), ShouldEqual, nil)
		})

		Convey(`Can call Get`, func() {
			v, ok := env.Get("foo")
			So(ok, ShouldBeFalse)
			So(v, ShouldEqual, "")
		})

		Convey(`Can call Set`, func() {
			env.Set("foo", "bar")
			So(env.Len(), ShouldEqual, 1)
			So(env.Sorted(), ShouldResemble, []string{"foo=bar"})
		})

		Convey(`Can be cloned.`, func() {
			So(env.Clone(), ShouldResemble, env)
		})
	})

	Convey(`A testing Env`, t, func() {
		env := New([]string{
			"PYTHONPATH=/foo:/bar:/baz",
			"http_proxy=wiped-out-by-next",
			"http_proxy=http://example.com",
			"novalue",
		})
		So(env.Len(), ShouldEqual, 3)

		Convey(`Can Get values.`, func() {
			v, ok := env.Get("PYTHONPATH")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "/foo:/bar:/baz")

			v, ok = env.Get("http_proxy")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "http://example.com")

			v, ok = env.Get("novalue")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "")
		})

		Convey(`Will note missing values.`, func() {
			_, ok := env.Get("pythonpath")
			So(ok, ShouldBeFalse)

			_, ok = env.Get("")
			So(ok, ShouldBeFalse)
		})

		Convey(`Can update its values.`, func() {
			orig := env.Clone()

			// Update PYTHONPATH, confirm that it updated correctly.
			v, _ := env.Get("PYTHONPATH")
			env.Set("PYTHONPATH", "/override:"+v)
			So(env.Sorted(), ShouldResemble, []string{
				"PYTHONPATH=/override:/foo:/bar:/baz",
				"http_proxy=http://example.com",
				"novalue",
			})

			// Test that the clone didn't change.
			So(orig.Sorted(), ShouldResemble, []string{
				"PYTHONPATH=/foo:/bar:/baz",
				"http_proxy=http://example.com",
				"novalue",
			})
		})
	})
}

func TestEnvironmentConstruction(t *testing.T) {
	t.Parallel()

	Convey(`Can create a new enviornment with Make`, t, func() {
		env := Make(map[string]string{
			"FOO": "BAR",
			"foo": "bar",
		})

		So(env, ShouldResemble, Env{
			env: map[string]string{
				"FOO": "FOO=BAR",
				"foo": "foo=bar",
			},
		})
	})

	Convey(`Make with an nil/empty enviornment returns a zero value.`, t, func() {
		So(Make(nil), ShouldResemble, Env{})
		So(Make(map[string]string{}), ShouldResemble, Env{})
	})

	Convey(`New with a nil/empty slice returns a zero value.`, t, func() {
		So(New(nil), ShouldResemble, Env{})
		So(New([]string{}), ShouldResemble, Env{})
	})
}
