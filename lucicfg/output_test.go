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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("With temp dir", t, func() {
		tmp, err := ioutil.TempDir("", "lucicfg")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmp)

		path := func(p string) string {
			return filepath.Join(tmp, filepath.FromSlash(p))
		}

		read := func(p string) string {
			body, err := os.ReadFile(path(p))
			So(err, ShouldBeNil)
			return string(body)
		}

		write := func(p, body string) {
			So(os.WriteFile(path(p), []byte(body), 0600), ShouldBeNil)
		}

		original := map[string][]byte{
			"a.cfg":        []byte("a\n"),
			"subdir/b.cfg": []byte("b\n"),
		}
		So(os.Mkdir(path("subdir"), 0700), ShouldBeNil)
		for k, v := range original {
			write(k, string(v))
		}

		Convey("Writing", func() {
			out := Output{
				Data: map[string]Datum{
					"a":     BlobDatum("111"),
					"dir/a": BlobDatum("222"),
				},
			}
			changed, unchanged, err := out.Write(tmp, false)
			So(changed, ShouldResemble, []string{"a", "dir/a"})
			So(unchanged, ShouldHaveLength, 0)
			So(err, ShouldBeNil)

			So(read("a"), ShouldResemble, "111")
			So(read("dir/a"), ShouldResemble, "222")

			out.Data["a"] = BlobDatum("333")
			changed, unchanged, err = out.Write(tmp, false)
			So(changed, ShouldResemble, []string{"a"})
			So(unchanged, ShouldResemble, []string{"dir/a"})
			So(err, ShouldBeNil)

			So(read("a"), ShouldResemble, "333")
		})

		Convey("DiscardChangesToUntracked", func() {
			generated := func() Output {
				return Output{
					Data: map[string]Datum{
						"a.cfg":        BlobDatum("new a\n"),
						"subdir/b.cfg": BlobDatum("new b\n"),
					},
				}
			}

			Convey("No untracked", func() {
				out := generated()
				So(out.DiscardChangesToUntracked(ctx, []string{"**/*"}, "-"), ShouldBeNil)
				So(out.Data, ShouldResemble, generated().Data)
			})

			Convey("Untracked files are restored from disk", func() {
				out := generated()
				So(out.DiscardChangesToUntracked(ctx, []string{"!*/b.cfg"}, tmp), ShouldBeNil)
				So(out.Data, ShouldResemble, map[string]Datum{
					"a.cfg":        generated().Data["a.cfg"],
					"subdir/b.cfg": BlobDatum(original["subdir/b.cfg"]),
				})
			})

			Convey("Untracked files are discarded when dumping to stdout", func() {
				out := generated()
				So(out.DiscardChangesToUntracked(ctx, []string{"!*/b.cfg"}, "-"), ShouldBeNil)
				So(out.Data, ShouldResemble, map[string]Datum{
					"a.cfg": generated().Data["a.cfg"],
				})
			})

			Convey("Untracked files are discarded if don't exist on disk", func() {
				out := Output{
					Data: map[string]Datum{
						"c.cfg": BlobDatum("generated"),
					},
				}
				So(out.DiscardChangesToUntracked(ctx, []string{"!c.cfg"}, tmp), ShouldBeNil)
				So(out.Data, ShouldHaveLength, 0)
			})
		})

		Convey("Reading", func() {
			out := Output{
				Data: map[string]Datum{
					"m1": BlobDatum("111"),
					"m2": BlobDatum("222"),
				},
			}

			Convey("Success", func() {
				write("m1", "new 1")
				write("m2", "new 2")

				So(out.Read(tmp), ShouldBeNil)
				So(out.Data, ShouldResemble, map[string]Datum{
					"m1": BlobDatum("new 1"),
					"m2": BlobDatum("new 2"),
				})
			})

			Convey("Missing file", func() {
				write("m1", "new 1")
				So(out.Read(tmp), ShouldNotBeNil)
			})
		})

		Convey("Compares protos semantically", func() {
			// Write the initial version.
			out := Output{
				Data: map[string]Datum{
					"m1": &MessageDatum{Header: "", Message: testMessage(111, 0)},
					"m2": &MessageDatum{Header: "# Header\n", Message: testMessage(222, 0)},
				},
			}
			changed, unchanged, err := out.Write(tmp, false)
			So(changed, ShouldResemble, []string{"m1", "m2"})
			So(unchanged, ShouldHaveLength, 0)
			So(err, ShouldBeNil)

			So(read("m1"), ShouldResemble, "i: 111\n")
			So(read("m2"), ShouldResemble, "# Header\ni: 222\n")

			Convey("Ignores formatting", func() {
				// Mutate m2 in insignificant way (strip the header).
				write("m2", "i:     222")

				// If using semantic comparison, recognizes nothing has changed.
				cmp, err := out.Compare(tmp, true)
				So(err, ShouldBeNil)
				So(cmp, ShouldResemble, map[string]CompareResult{
					"m1": Identical,
					"m2": SemanticallyEqual,
				})

				// Byte-to-byte comparison recognizes the change.
				cmp, err = out.Compare(tmp, false)
				So(err, ShouldBeNil)
				So(cmp, ShouldResemble, map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				})

				Convey("Write, force=false", func() {
					// Output didn't really change, so nothing is overwritten.
					changed, unchanged, err := out.Write(tmp, false)
					So(err, ShouldBeNil)
					So(changed, ShouldHaveLength, 0)
					So(unchanged, ShouldResemble, []string{"m1", "m2"})
				})

				Convey("Write, force=true", func() {
					// We ask to overwrite files even if they all are semantically same.
					changed, unchanged, err := out.Write(tmp, true)
					So(err, ShouldBeNil)
					So(changed, ShouldResemble, []string{"m2"})
					So(unchanged, ShouldResemble, []string{"m1"})

					// Overwrote it on disk.
					So(read("m2"), ShouldResemble, "# Header\ni: 222\n")
				})
			})

			Convey("Detects real changes", func() {
				// Overwrite m2 with something semantically different.
				write("m2", "i: 333")

				// Detected it.
				cmp, err := out.Compare(tmp, true)
				So(err, ShouldBeNil)
				So(cmp, ShouldResemble, map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				})

				// Writes it to disk, even when force=false.
				changed, unchanged, err := out.Write(tmp, false)
				So(err, ShouldBeNil)
				So(changed, ShouldResemble, []string{"m2"})
				So(unchanged, ShouldResemble, []string{"m1"})
			})

			Convey("Handles bad protos", func() {
				// Overwrite m2 with some garbage.
				write("m2", "not a text proto")

				// Detected the file as changed.
				cmp, err := out.Compare(tmp, true)
				So(err, ShouldBeNil)
				So(cmp, ShouldResemble, map[string]CompareResult{
					"m1": Identical,
					"m2": Different,
				})
			})
		})
	})

	Convey("ConfigSets", t, func() {
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
			So(err, ShouldBeNil)
			return cs
		}

		Convey("No roots", func() {
			So(configSets(), ShouldHaveLength, 0)
		})

		Convey("Empty set", func() {
			out.Roots["set"] = "zzz"
			So(configSets(), ShouldResemble, []ConfigSet{
				{
					Name: "set",
					Data: map[string][]byte{},
				},
			})
		})

		Convey("`.` root", func() {
			out.Roots["set"] = "."
			So(configSets(), ShouldResemble, []ConfigSet{
				{
					Name: "set",
					Data: everything,
				},
			})
		})

		Convey("Subdir root", func() {
			out.Roots["set"] = "dir1/."
			So(configSets(), ShouldResemble, []ConfigSet{
				{
					Name: "set",
					Data: map[string][]byte{
						"f2":     []byte("1"),
						"f3":     []byte("2"),
						"sub/f4": []byte("3"),
					},
				},
			})
		})

		Convey("Multiple roots", func() {
			out.Roots["set1"] = "dir1"
			out.Roots["set2"] = "dir2"
			out.Roots["set3"] = "dir1/sub" // intersecting sets are OK
			So(configSets(), ShouldResemble, []ConfigSet{
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
			})
		})
	})
}
