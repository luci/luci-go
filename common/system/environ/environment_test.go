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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run(`Source environment slice translates correctly to/from an Env.`, t, func(t *ftt.Test) {
		t.Run(`Case insensitive (e.g., Windows)`, func(t *ftt.Test) {
			pretendWindows()

			env := New([]string{
				"",
				"FOO",
				"BAR=BAZ",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})
			assert.Loosely(t, env, should.Resemble(Env{
				env: map[string]string{
					"FOO": "FOO=",
					"BAR": "bar=baz",
					"QUX": "qux=quux=quuuuuuux",
				},
			}))

			assert.Loosely(t, env.Sorted(), should.Resemble([]string{
				"FOO=",
				"bar=baz",
				"qux=quux=quuuuuuux",
			}))

			assert.Loosely(t, env.Get(""), should.BeEmpty)
			assert.Loosely(t, env.Get("FOO"), should.BeEmpty)
			assert.Loosely(t, env.Get("BAR"), should.Equal("baz"))
			assert.Loosely(t, env.Get("bar"), should.Equal("baz"))
			assert.Loosely(t, env.Get("qux"), should.Equal("quux=quuuuuuux"))
			assert.Loosely(t, env.Get("QuX"), should.Equal("quux=quuuuuuux"))
		})

		t.Run(`Case sensitive (e.g., POSIX)`, func(t *ftt.Test) {
			pretendLinux()

			env := New([]string{
				"",
				"FOO",
				"BAR=BAZ",
				"bar=baz",
				"qux=quux=quuuuuuux",
			})
			assert.Loosely(t, env, should.Resemble(Env{
				env: map[string]string{
					"FOO": "FOO=",
					"BAR": "BAR=BAZ",
					"bar": "bar=baz",
					"qux": "qux=quux=quuuuuuux",
				},
			}))

			assert.Loosely(t, env.Sorted(), should.Resemble([]string{
				"BAR=BAZ",
				"FOO=",
				"bar=baz",
				"qux=quux=quuuuuuux",
			}))

			assert.Loosely(t, env.Get(""), should.BeEmpty)
			assert.Loosely(t, env.Get("FOO"), should.BeEmpty)
			assert.Loosely(t, env.Get("BAR"), should.Equal("BAZ"))
			assert.Loosely(t, env.Get("bar"), should.Equal("baz"))
			assert.Loosely(t, env.Get("qux"), should.Equal("quux=quuuuuuux"))
			assert.Loosely(t, env.Get("QuX"), should.BeEmpty)
		})
	})
}

func TestEnvironmentManipulation(t *testing.T) {
	ftt.Run(`A zero-valued Env`, t, func(t *ftt.Test) {
		pretendLinux()

		var env Env
		assert.Loosely(t, env.Len(), should.BeZero)

		t.Run(`Can be sorted.`, func(t *ftt.Test) {
			assert.Loosely(t, env.Sorted(), should.BeNil)
		})

		t.Run(`Can call Get`, func(t *ftt.Test) {
			v, ok := env.Lookup("foo")
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, v, should.BeEmpty)
		})

		t.Run(`Can be cloned`, func(t *ftt.Test) {
			assert.Loosely(t, env.Clone(), should.Resemble(New(nil)))
		})

		t.Run(`Set panics`, func(t *ftt.Test) {
			assert.Loosely(t, func() { env.Set("foo", "bar") }, should.Panic)
		})
	})

	ftt.Run(`An empty Env`, t, func(t *ftt.Test) {
		pretendLinux()

		env := New(nil)
		assert.Loosely(t, env.Len(), should.BeZero)

		t.Run(`Can be sorted.`, func(t *ftt.Test) {
			assert.Loosely(t, env.Sorted(), should.BeNil)
		})

		t.Run(`Can call Get`, func(t *ftt.Test) {
			v, ok := env.Lookup("foo")
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, v, should.BeEmpty)
		})

		t.Run(`Can call Set`, func(t *ftt.Test) {
			env.Set("foo", "bar")
			assert.Loosely(t, env.Len(), should.Equal(1))
			assert.Loosely(t, env.Sorted(), should.Resemble([]string{"foo=bar"}))
		})

		t.Run(`Can be cloned`, func(t *ftt.Test) {
			assert.Loosely(t, env.Clone(), should.Resemble(New(nil)))
		})
	})

	ftt.Run(`A testing Env`, t, func(t *ftt.Test) {
		pretendWindows()

		env := New([]string{
			"PYTHONPATH=/foo:/bar:/baz",
			"http_proxy=wiped-out-by-next",
			"http_proxy=http://example.com",
			"novalue",
		})
		assert.Loosely(t, env.Len(), should.Equal(3))

		t.Run(`Can Get values.`, func(t *ftt.Test) {
			v, ok := env.Lookup("PYTHONPATH")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("/foo:/bar:/baz"))

			v, ok = env.Lookup("http_proxy")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.Equal("http://example.com"))

			v, ok = env.Lookup("novalue")
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, v, should.BeEmpty)
		})

		t.Run(`Will note missing values.`, func(t *ftt.Test) {
			_, ok := env.Lookup("missing")
			assert.Loosely(t, ok, should.BeFalse)

			_, ok = env.Lookup("")
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run(`Can be converted into a map and enumerated`, func(t *ftt.Test) {
			assert.Loosely(t, env.Map(), should.Resemble(map[string]string{
				"PYTHONPATH": "/foo:/bar:/baz",
				"http_proxy": "http://example.com",
				"novalue":    "",
			}))

			t.Run(`Can perform iteration`, func(t *ftt.Test) {
				buildMap := make(map[string]string)
				assert.Loosely(t, env.Iter(func(k, v string) error {
					buildMap[k] = v
					return nil
				}), should.BeNil)
				assert.Loosely(t, env.Map(), should.Resemble(buildMap))
			})

			t.Run(`Can have elements removed through iteration`, func(t *ftt.Test) {
				env.RemoveMatch(func(k, v string) bool {
					switch k {
					case "PYTHONPATH", "novalue":
						return true
					default:
						return false
					}
				})
				assert.Loosely(t, env.Map(), should.Resemble(map[string]string{
					"http_proxy": "http://example.com",
				}))
			})
		})

		t.Run(`Can update its values.`, func(t *ftt.Test) {
			orig := env.Clone()

			// Update PYTHONPATH, confirm that it updated correctly.
			v, _ := env.Lookup("PYTHONPATH")
			env.Set("PYTHONPATH", "/override:"+v)
			assert.Loosely(t, env.Sorted(), should.Resemble([]string{
				"PYTHONPATH=/override:/foo:/bar:/baz",
				"http_proxy=http://example.com",
				"novalue=",
			}))

			// Use a different-case key, and confirm that it still updated correctly.
			t.Run(`When case insensitive, will update common keys.`, func(t *ftt.Test) {
				env.Set("pYtHoNpAtH", "/override:"+v)
				assert.Loosely(t, env.Sorted(), should.Resemble([]string{
					"http_proxy=http://example.com",
					"novalue=",
					"pYtHoNpAtH=/override:/foo:/bar:/baz",
				}))
				assert.Loosely(t, env.Get("PYTHONPATH"), should.Equal("/override:/foo:/bar:/baz"))

				assert.Loosely(t, env.Remove("HTTP_PROXY"), should.BeTrue)
				assert.Loosely(t, env.Remove("nonexistent"), should.BeFalse)
				assert.Loosely(t, env.Sorted(), should.Resemble([]string{
					"novalue=",
					"pYtHoNpAtH=/override:/foo:/bar:/baz",
				}))

				// Test that the clone didn't change.
				assert.Loosely(t, orig.Sorted(), should.Resemble([]string{
					"PYTHONPATH=/foo:/bar:/baz",
					"http_proxy=http://example.com",
					"novalue=",
				}))

				orig.Update(New([]string{
					"http_PROXY=foo",
					"HTTP_PROXY=FOO",
					"newkey=value",
				}))
				assert.Loosely(t, orig.Sorted(), should.Resemble([]string{
					"HTTP_PROXY=FOO",
					"PYTHONPATH=/foo:/bar:/baz",
					"newkey=value",
					"novalue=",
				}))
			})
		})
	})
}

func TestEnvironmentConstruction(t *testing.T) {
	pretendLinux()

	ftt.Run(`Can load an initial set of values from a map`, t, func(t *ftt.Test) {
		env := New(nil)
		env.Load(map[string]string{
			"FOO": "BAR",
			"foo": "bar",
		})
		assert.Loosely(t, env, should.Resemble(Env{
			env: map[string]string{
				"FOO": "FOO=BAR",
				"foo": "foo=bar",
			},
		}))
	})
}

func TestEnvironmentContext(t *testing.T) {
	pretendLinux()

	ftt.Run(`Can set and retrieve env from context`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Default is system`, func(t *ftt.Test) {
			assert.Loosely(t, FromCtx(ctx), should.Resemble(System()))
		})

		t.Run(`Setting nil works`, func(t *ftt.Test) {
			ctx = (Env{}).SetInCtx(ctx)
			env := FromCtx(ctx)
			// We specifically want FromCtx to always return a mutable Env, even if
			// the one in context is nil.
			assert.Loosely(t, env.env, should.NotBeNil)
			assert.Loosely(t, env, should.Resemble(Env{env: map[string]string{}}))
		})

		t.Run(`Can set in context`, func(t *ftt.Test) {
			env := New(nil)
			env.Load(map[string]string{
				"FOO":  "BAR",
				"COOL": "Stuff",
			})

			ctx = env.SetInCtx(ctx)

			t.Run(`And get a copy back`, func(t *ftt.Test) {
				ptr := func(e Env) uintptr {
					return reflect.ValueOf(e.env).Pointer()
				}

				env2 := FromCtx(ctx)
				assert.Loosely(t, ptr(env2), should.NotEqual(ptr(env)))
				assert.Loosely(t, env2, should.Resemble(env))

				assert.Loosely(t, ptr(FromCtx(ctx)), should.NotEqual(ptr(env)))
				assert.Loosely(t, ptr(FromCtx(ctx)), should.NotEqual(ptr(env2)))
			})

			t.Run(`Mutating after installation has no effect`, func(t *ftt.Test) {
				env.Set("COOL", "Nope")

				env2 := FromCtx(ctx)
				assert.Loosely(t, env2, should.NotResemble(env))

				env2.Set("COOL", "Nope")
				assert.Loosely(t, env2, should.Resemble(env))
			})
		})
	})
}
