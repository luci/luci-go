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

package environ

import (
	"context"
	"reflect"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// Note: all tests here are NOT marked with t.Parallel() because they mutate
// global 'normalizeKeyCase'.

func pretendWindows() {
	normalizeKeyCase = strings.ToUpper
}

func pretendLinux() {
	normalizeKeyCase = func(k string) string { return k }
}

func TestEnvironmentConversion(t *testing.T) {
	Convey(`Source environment slice translates correctly to/from an Env.`, t, func() {
		Convey(`Case insensitive (e.g., Windows)`, func() {
			pretendWindows()

			env := New([]string{
				"",
				"FOO",
				"BAR=BAZ",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})
			So(env, ShouldResemble, Env{
				env: map[string]string{
					"FOO": "FOO=",
					"BAR": "bar=baz",
					"QUX": "qux=quux=quuuuuuux",
				},
			})

			So(env.Sorted(), ShouldResemble, []string{
				"FOO=",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})

			So(env.Get(""), ShouldEqual, "")
			So(env.Get("FOO"), ShouldEqual, "")
			So(env.Get("BAR"), ShouldEqual, "baz")
			So(env.Get("bar"), ShouldEqual, "baz")
			So(env.Get("qux"), ShouldEqual, "quux=quuuuuuux")
			So(env.Get("QuX"), ShouldEqual, "quux=quuuuuuux")
		})

		Convey(`Case sensitive (e.g., POSIX)`, func() {
			pretendLinux()

			env := New([]string{
				"",
				"FOO",
				"BAR=BAZ",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})
			So(env, ShouldResemble, Env{
				env: map[string]string{
					"FOO": "FOO=",
					"BAR": "BAR=BAZ",
					"bar": "bar=baz",
					"qux": "qux=quux=quuuuuuux",
				},
			})

			So(env.Sorted(), ShouldResemble, []string{
				"BAR=BAZ",
				"FOO=",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})

			So(env.Get(""), ShouldEqual, "")
			So(env.Get("FOO"), ShouldEqual, "")
			So(env.Get("BAR"), ShouldEqual, "BAZ")
			So(env.Get("bar"), ShouldEqual, "baz")
			So(env.Get("qux"), ShouldEqual, "quux=quuuuuuux")
			So(env.Get("QuX"), ShouldEqual, "")
		})
	})
}

func TestEnvironmentManipulation(t *testing.T) {
	Convey(`A zero-valued Env`, t, func() {
		pretendLinux()

		var env Env
		So(env.Len(), ShouldEqual, 0)

		Convey(`Can be sorted.`, func() {
			So(env.Sorted(), ShouldBeNil)
		})

		Convey(`Can call Get`, func() {
			v, ok := env.Lookup("foo")
			So(ok, ShouldBeFalse)
			So(v, ShouldEqual, "")
		})

		Convey(`Can be cloned`, func() {
			So(env.Clone(), ShouldResemble, New(nil))
		})

		Convey(`Set panics`, func() {
			So(func() { env.Set("foo", "bar") }, ShouldPanic)
		})
	})

	Convey(`An empty Env`, t, func() {
		pretendLinux()

		env := New(nil)
		So(env.Len(), ShouldEqual, 0)

		Convey(`Can be sorted.`, func() {
			So(env.Sorted(), ShouldBeNil)
		})

		Convey(`Can call Get`, func() {
			v, ok := env.Lookup("foo")
			So(ok, ShouldBeFalse)
			So(v, ShouldEqual, "")
		})

		Convey(`Can call Set`, func() {
			env.Set("foo", "bar")
			So(env.Len(), ShouldEqual, 1)
			So(env.Sorted(), ShouldResemble, []string{"foo=bar"})
		})

		Convey(`Can be cloned`, func() {
			So(env.Clone(), ShouldResemble, New(nil))
		})
	})

	Convey(`A testing Env`, t, func() {
		pretendWindows()

		env := New([]string{
			"PYTHONPATH=/foo:/bar:/baz",
			"http_proxy=wiped-out-by-next",
			"http_proxy=http://example.com",
			"novalue",
		})
		So(env.Len(), ShouldEqual, 3)

		Convey(`Can Get values.`, func() {
			v, ok := env.Lookup("PYTHONPATH")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "/foo:/bar:/baz")

			v, ok = env.Lookup("http_proxy")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "http://example.com")

			v, ok = env.Lookup("novalue")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "")
		})

		Convey(`Will note missing values.`, func() {
			_, ok := env.Lookup("missing")
			So(ok, ShouldBeFalse)

			_, ok = env.Lookup("")
			So(ok, ShouldBeFalse)
		})

		Convey(`Can be converted into a map and enumerated`, func() {
			So(env.Map(), ShouldResemble, map[string]string{
				"PYTHONPATH": "/foo:/bar:/baz",
				"http_proxy": "http://example.com",
				"novalue":    "",
			})

			Convey(`Can perform iteration`, func() {
				buildMap := make(map[string]string)
				So(env.Iter(func(k, v string) error {
					buildMap[k] = v
					return nil
				}), ShouldBeNil)
				So(env.Map(), ShouldResemble, buildMap)
			})

			Convey(`Can have elements removed through iteration`, func() {
				env.RemoveMatch(func(k, v string) bool {
					switch k {
					case "PYTHONPATH", "novalue":
						return true
					default:
						return false
					}
				})
				So(env.Map(), ShouldResemble, map[string]string{
					"http_proxy": "http://example.com",
				})
			})
		})

		Convey(`Can update its values.`, func() {
			orig := env.Clone()

			// Update PYTHONPATH, confirm that it updated correctly.
			v, _ := env.Lookup("PYTHONPATH")
			env.Set("PYTHONPATH", "/override:"+v)
			So(env.Sorted(), ShouldResemble, []string{
				"PYTHONPATH=/override:/foo:/bar:/baz",
				"http_proxy=http://example.com",
				"novalue=",
			})

			// Use a different-case key, and confirm that it still updated correctly.
			Convey(`When case insensitive, will update common keys.`, func() {
				env.Set("pYtHoNpAtH", "/override:"+v)
				So(env.Sorted(), ShouldResemble, []string{
					"http_proxy=http://example.com",
					"novalue=",
					"pYtHoNpAtH=/override:/foo:/bar:/baz",
				})
				So(env.Get("PYTHONPATH"), ShouldEqual, "/override:/foo:/bar:/baz")

				So(env.Remove("HTTP_PROXY"), ShouldBeTrue)
				So(env.Remove("nonexistent"), ShouldBeFalse)
				So(env.Sorted(), ShouldResemble, []string{
					"novalue=",
					"pYtHoNpAtH=/override:/foo:/bar:/baz",
				})

				// Test that the clone didn't change.
				So(orig.Sorted(), ShouldResemble, []string{
					"PYTHONPATH=/foo:/bar:/baz",
					"http_proxy=http://example.com",
					"novalue=",
				})

				orig.Update(New([]string{
					"http_PROXY=foo",
					"HTTP_PROXY=FOO",
					"newkey=value",
				}))
				So(orig.Sorted(), ShouldResemble, []string{
					"HTTP_PROXY=FOO",
					"PYTHONPATH=/foo:/bar:/baz",
					"newkey=value",
					"novalue=",
				})
			})
		})
	})
}

func TestEnvironmentConstruction(t *testing.T) {
	pretendLinux()

	Convey(`Can load an initial set of values from a map`, t, func() {
		env := New(nil)
		env.Load(map[string]string{
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
}

func TestEnvironmentContext(t *testing.T) {
	pretendLinux()

	Convey(`Can set and retrieve env from context`, t, func() {
		ctx := context.Background()

		Convey(`Default is system`, func() {
			So(FromCtx(ctx), ShouldResemble, System())
		})

		Convey(`Setting nil works`, func() {
			ctx = (Env{}).SetInCtx(ctx)
			env := FromCtx(ctx)
			// We specifically want FromCtx to always return a mutable Env, even if
			// the one in context is nil.
			So(env, ShouldNotBeNil)
			So(env, ShouldResemble, Env{env: map[string]string{}})
		})

		Convey(`Can set in context`, func() {
			env := New(nil)
			env.Load(map[string]string{
				"FOO":  "BAR",
				"COOL": "Stuff",
			})

			ctx = env.SetInCtx(ctx)

			Convey(`And get a copy back`, func() {
				ptr := func(e Env) uintptr {
					return reflect.ValueOf(e.env).Pointer()
				}

				env2 := FromCtx(ctx)
				So(ptr(env2), ShouldNotEqual, ptr(env))
				So(env2, ShouldResemble, env)

				So(ptr(FromCtx(ctx)), ShouldNotEqual, ptr(env))
				So(ptr(FromCtx(ctx)), ShouldNotEqual, ptr(env2))
			})

			Convey(`Mutating after installation has no effect`, func() {
				env.Set("COOL", "Nope")

				env2 := FromCtx(ctx)
				So(env2, ShouldNotEqual, env)
				So(env2, ShouldNotResemble, env)

				env2.Set("COOL", "Nope")
				So(env2, ShouldResemble, env)
			})
		})
	})
}
