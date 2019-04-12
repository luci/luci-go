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

		read := func(p string) []byte {
			body, err := ioutil.ReadFile(path(p))
			So(err, ShouldBeNil)
			return body
		}

		original := map[string][]byte{
			"a.cfg":        []byte("a\n"),
			"subdir/b.cfg": []byte("b\n"),
		}
		So(os.Mkdir(path("subdir"), 0700), ShouldBeNil)
		for k, v := range original {
			So(ioutil.WriteFile(path(k), v, 0600), ShouldBeNil)
		}

		Convey("Writing", func() {
			out := Output{
				Data: map[string][]byte{
					"a":     []byte("111"),
					"dir/a": []byte("222"),
				},
			}
			changed, unchanged, err := out.Write(tmp)
			So(changed, ShouldResemble, []string{"a", "dir/a"})
			So(unchanged, ShouldHaveLength, 0)
			So(err, ShouldBeNil)

			So(read("a"), ShouldResemble, []byte("111"))
			So(read("dir/a"), ShouldResemble, []byte("222"))

			out.Data["a"] = []byte("333")
			changed, unchanged, err = out.Write(tmp)
			So(changed, ShouldResemble, []string{"a"})
			So(unchanged, ShouldResemble, []string{"dir/a"})
			So(err, ShouldBeNil)

			So(read("a"), ShouldResemble, []byte("333"))
		})

		Convey("DiscardChangesToUntracked", func() {
			generated := func() Output {
				return Output{
					Data: map[string][]byte{
						"a.cfg":        []byte("new a\n"),
						"subdir/b.cfg": []byte("new b\n"),
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
				So(out.Data, ShouldResemble, map[string][]byte{
					"a.cfg":        generated().Data["a.cfg"],
					"subdir/b.cfg": original["subdir/b.cfg"],
				})
			})

			Convey("Untracked files are discarded when dumping to stdout", func() {
				out := generated()
				So(out.DiscardChangesToUntracked(ctx, []string{"!*/b.cfg"}, "-"), ShouldBeNil)
				So(out.Data, ShouldResemble, map[string][]byte{
					"a.cfg": generated().Data["a.cfg"],
				})
			})

			Convey("Untracked files are discarded if don't exist on disk", func() {
				out := Output{
					Data: map[string][]byte{
						"c.cfg": []byte("generated"),
					},
				}
				So(out.DiscardChangesToUntracked(ctx, []string{"!c.cfg"}, tmp), ShouldBeNil)
				So(out.Data, ShouldHaveLength, 0)
			})
		})
	})

	Convey("Digests", t, func() {
		out := Output{
			Data: map[string][]byte{
				"a": nil,
				"b": []byte("123"),
			},
		}
		So(out.Digests(), ShouldResemble, map[string]string{
			"a": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"b": "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
		})
	})
}
