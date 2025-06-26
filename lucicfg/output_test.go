// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("With temp dir", t, func(t *ftt.Test) {
		tmp, err := os.MkdirTemp("", "lucicfg")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tmp)

		path := func(p string) string {
			return filepath.Join(tmp, filepath.FromSlash(p))
		}

		read := func(p string) string {
			body, err := os.ReadFile(path(p))
			assert.Loosely(t, err, should.BeNil)
			return string(body)
		}

		write := func(p, body string) {
			assert.Loosely(t, os.WriteFile(path(p), []byte(body), 0600), should.BeNil)
		}

		original := map[string][]byte{
			"a.cfg":        []byte("a\n"),
			"subdir/b.cfg": []byte("b\n"),
		}
		assert.Loosely(t, os.Mkdir(path("subdir"), 0700), should.BeNil)
		for k, v := range original {
			write(k, string(v))
		}

		t.Run("Writing", func(t *ftt.Test) {
			out := Output{
				Data: map[string]Datum{
					"a":     BlobDatum("111"),
					"dir/a": BlobDatum("222"),
				},
			}
			changed, unchanged, err := out.Write(tmp, false)
			assert.Loosely(t, changed, should.Match([]string{"a", "dir/a"}))
			assert.Loosely(t, unchanged, should.HaveLength(0))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, read("a"), should.Match("111"))
			assert.Loosely(t, read("dir/a"), should.Match("222"))

			out.Data["a"] = BlobDatum("333")
			changed, unchanged, err = out.Write(tmp, false)
			assert.Loosely(t, changed, should.Match([]string{"a"}))
			assert.Loosely(t, unchanged, should.Match([]string{"dir/a"}))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, read("a"), should.Match("333"))
		})

		t.Run("DiscardChangesToUntracked", func(t *ftt.Test) {
			generated := func() Output {
				return Output{
					Data: map[string]Datum{
						"a.cfg":        BlobDatum("new a\n"),
						"subdir/b.cfg": BlobDatum("new b\n"),
					},
				}
			}

			t.Run("No untracked", func(t *ftt.Test) {
				out := generated()
				assert.Loosely(t, out.DiscardChangesToUntracked(ctx, []string{"**/*"}, "-"), should.BeNil)
				assert.Loosely(t, out.Data, should.Match(generated().Data))
			})

			t.Run("Untracked files are restored from disk", func(t *ftt.Test) {
				out := generated()
				assert.Loosely(t, out.DiscardChangesToUntracked(ctx, []string{"!*/b.cfg"}, tmp), should.BeNil)
				assert.Loosely(t, out.Data, should.Match(map[string]Datum{
					"a.cfg":        generated().Data["a.cfg"],
					"subdir/b.cfg": BlobDatum(original["subdir/b.cfg"]),
				}))
			})

			t.Run("Untracked files are discarded when dumping to stdout", func(t *ftt.Test) {
				out := generated()
				assert.Loosely(t, out.DiscardChangesToUntracked(ctx, []string{"!*/b.cfg"}, "-"), should.BeNil)
				assert.Loosely(t, out.Data, should.Match(map[string]Datum{
					"a.cfg": generated().Data["a.cfg"],
				}))
			})

			t.Run("Untracked files are discarded if don't exist on disk", func(t *ftt.Test) {
				out := Output{
					Data: map[string]Datum{
						"c.cfg": BlobDatum("generated"),
					},
				}
				assert.Loosely(t, out.DiscardChangesToUntracked(ctx, []string{"!c.cfg"}, tmp), should.BeNil)
				assert.Loosely(t, out.Data, should.HaveLength(0))
			})
		})

		t.Run("Reading", func(t *ftt.Test) {
			out := Output{
				Data: map[string]Datum{
					"m1": BlobDatum("111"),
					"m2": BlobDatum("222"),
				},
			}

			t.Run("Success", func(t *ftt.Test) {
				write("m1", "new 1")
				write("m2", "new 2")

				assert.Loosely(t, out.Read(tmp), should.BeNil)
				assert.Loosely(t, out.Data, should.Match(map[string]Datum{
					"m1": BlobDatum("new 1"),
					"m2": BlobDatum("new 2"),
				}))
			})

			t.Run("Missing file", func(t *ftt.Test) {
				write("m1", "new 1")
				assert.Loosely(t, out.Read(tmp), should.NotBeNil)
			})
		})

		t.Run("Compares protos semantically", func(t *ftt.Test) {
			// Write the initial version.
			out := Output{
				Data: map[string]Datum{
					"m1": &MessageDatum{Header: "", Message: testMessage(111, 0)},
					"m2": &MessageDatum{Header: "# Header\n", Message: testMessage(222, 0)},
				},
			}
			changed, unchanged, err := out.Write(tmp, false)
			assert.Loosely(t, changed, should.Match([]string{"m1", "m2"}))
			assert.Loosely(t, unchanged, should.HaveLength(0))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, read("m1"), should.Match("i: 111\n"))
			assert.Loosely(t, read("m2"), should.Match("# Header\ni: 222\n"))

			t.Run("Ignores formatting", func(t *ftt.Test) {
				// Mutate m2 in insignificant way (strip the header).
				write("m2", "i:     222")

				// If using semantic comparison, recognizes nothing has changed.
				cmp, err := out.Compare(tmp, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cmp, should.Match(map[string]CompareResult{
					"m1": Identical,
					"m2": SemanticallyEqual,
				}))

				// Byte-to-byte comparison recognizes the change.
				cmp, err = out.Compare(tmp, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cmp, should.Match(map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				}))

				t.Run("Write, force=false", func(t *ftt.Test) {
					// Output didn't really change, so nothing is overwritten.
					changed, unchanged, err := out.Write(tmp, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, changed, should.HaveLength(0))
					assert.Loosely(t, unchanged, should.Match([]string{"m1", "m2"}))
				})

				t.Run("Write, force=true", func(t *ftt.Test) {
					// We ask to overwrite files even if they all are semantically same.
					changed, unchanged, err := out.Write(tmp, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, changed, should.Match([]string{"m2"}))
					assert.Loosely(t, unchanged, should.Match([]string{"m1"}))

					// Overwrote it on disk.
					assert.Loosely(t, read("m2"), should.Match("# Header\ni: 222\n"))
				})
			})

			t.Run("Detects real changes", func(t *ftt.Test) {
				// Overwrite m2 with something semantically different.
				write("m2", "i: 333")

				// Detected it.
				cmp, err := out.Compare(tmp, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cmp, should.Match(map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				}))

				// Writes it to disk, even when force=false.
				changed, unchanged, err := out.Write(tmp, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, changed, should.Match([]string{"m2"}))
				assert.Loosely(t, unchanged, should.Match([]string{"m1"}))
			})

			t.Run("Handles bad protos", func(t *ftt.Test) {
				// Overwrite m2 with some garbage.
				write("m2", "not a text proto")

				// Detected the file as changed.
				cmp, err := out.Compare(tmp, true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cmp, should.Match(map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				}))
			})
		})
	})

	ftt.Run("ConfigSets", t, func(t *ftt.Test) {
		out := Output{
			Data: map[string]Datum{
				"f1":          BlobDatum("0"),
				"dir1/f2":     BlobDatum("1"),
				"dir1/f3":     BlobDatum("2"),
				"dir1/sub/f4": BlobDatum("3"),
				"dir2/f5":     BlobDatum("4"),
			},
			Roots: map[string]string{},
		}

		// Same data, as raw bytes.
		everything := map[string][]byte{}
		for k, v := range out.Data {
			everything[k], _ = v.Bytes()
		}

		configSets := func() []ConfigSet {
			cs, err := out.ConfigSets()
			assert.Loosely(t, err, should.BeNil)
			return cs
		}

		t.Run("No roots", func(t *ftt.Test) {
			assert.Loosely(t, configSets(), should.HaveLength(0))
		})

		t.Run("Empty set", func(t *ftt.Test) {
			out.Roots["set"] = "zzz"
			assert.Loosely(t, configSets(), should.Match([]ConfigSet{
				{
					Name: "set",
					Data: map[string][]byte{},
				},
			}))
		})

		t.Run("`.` root", func(t *ftt.Test) {
			out.Roots["set"] = "."
			assert.Loosely(t, configSets(), should.Match([]ConfigSet{
				{
					Name: "set",
					Data: everything,
				},
			}))
		})

		t.Run("Subdir root", func(t *ftt.Test) {
			out.Roots["set"] = "dir1/."
			assert.Loosely(t, configSets(), should.Match([]ConfigSet{
				{
					Name: "set",
					Data: map[string][]byte{
						"f2":     []byte("1"),
						"f3":     []byte("2"),
						"sub/f4": []byte("3"),
					},
				},
			}))
		})

		t.Run("Multiple roots", func(t *ftt.Test) {
			out.Roots["set1"] = "dir1"
			out.Roots["set2"] = "dir2"
			out.Roots["set3"] = "dir1/sub" // intersecting sets are OK
			assert.Loosely(t, configSets(), should.Match([]ConfigSet{
				{
					Name: "set1",
					Data: map[string][]byte{
						"f2":     []byte("1"),
						"f3":     []byte("2"),
						"sub/f4": []byte("3"),
					},
				},
				{
					Name: "set2",
					Data: map[string][]byte{
						"f5": []byte("4"),
					},
				},
				{
					Name: "set3",
					Data: map[string][]byte{
						"f4": []byte("3"),
					},
				},
			}))
		})
	})
}
