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

package filesystem

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
)

func withFolder(files map[string]string, cb func(folder string)) {
	folder, err := ioutil.TempDir("", "fs_test_")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(folder)

	for fpath, content := range files {
		if content == "" {
			content = fpath
		}
		fpath = filepath.Join(folder, filepath.FromSlash(fpath))
		if err := os.MkdirAll(filepath.Dir(fpath), 0777); err != nil {
			panic(err)
		}
		if err := os.WriteFile(fpath, []byte(content), 0666); err != nil {
			panic(err)
		}
	}

	cb(folder)
}

func TestFSImpl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	withFolder(map[string]string{
		"projects/doodly/something/file.cfg": "",
		"projects/foobar/something/file.cfg": "",
		"projects/foobar/another/file.cfg":   "",
		"services/foosrv/something.cfg":      "",
		"projects/foobar.json": `{
			"Name": "A cool project",
			"Url": "https://something.example.com"
		}`,
	}, func(folder string) {
		Convey("basic Test Filesystem config client", t, func() {
			const expectedRev = "a1b9f654acc5008452980a98ec930cbfdeec82d6"

			client, err := New(folder)
			So(err, ShouldBeNil)

			Convey("GetConfig", func() {
				expect := &config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/foobar",
						Path:        "something/file.cfg",
						ContentHash: "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8",
						Revision:    expectedRev,
						ViewURL:     "file://./something/file.cfg",
					},
					Content: "projects/foobar/something/file.cfg",
				}

				Convey("All content", func() {
					cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", false)
					So(err, ShouldBeNil)
					So(cfg, ShouldResemble, expect)
				})

				Convey("services", func() {
					cfg, err := client.GetConfig(ctx, "services/foosrv", "something.cfg", false)
					So(err, ShouldBeNil)
					So(cfg, ShouldResemble, &config.Config{
						Meta: config.Meta{
							ConfigSet:   "services/foosrv",
							Path:        "something.cfg",
							ContentHash: "v1:536a41710e0cb4f21950d5e0e32642bda58fce9a",
							Revision:    expectedRev,
							ViewURL:     "file://./something.cfg",
						},
						Content: "services/foosrv/something.cfg",
					})
				})

				Convey("just meta", func() {
					cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", true)
					So(err, ShouldBeNil)
					So(cfg.ContentHash, ShouldEqual, "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8")
					So(cfg.Content, ShouldEqual, "")

					Convey("make sure it doesn't poison the cache", func() {
						cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", false)
						So(err, ShouldBeNil)
						So(cfg, ShouldResemble, expect)
					})
				})
			})

			Convey("GetConfigs", func() {
				cfg, err := client.GetConfigs(ctx, "projects/foobar", nil, false)
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, map[string]config.Config{
					"another/file.cfg": {
						Meta: config.Meta{
							ConfigSet:   "projects/foobar",
							Path:        "another/file.cfg",
							ContentHash: "v1:1b136cf89ccbd1d5f42cecae874367b0258d810d",
							Revision:    expectedRev,
							ViewURL:     "file://./another/file.cfg",
						},
						Content: "projects/foobar/another/file.cfg",
					},
					"something/file.cfg": {
						Meta: config.Meta{
							ConfigSet:   "projects/foobar",
							Path:        "something/file.cfg",
							ContentHash: "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8",
							Revision:    expectedRev,
							ViewURL:     "file://./something/file.cfg",
						},
						Content: "projects/foobar/something/file.cfg",
					},
				})
			})

			Convey("ListFiles", func() {
				cfg, err := client.ListFiles(ctx, "projects/foobar")
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, []string{
					"another/file.cfg",
					"something/file.cfg",
				})
			})

			Convey("GetProjectConfigs", func() {
				cfgs, err := client.GetProjectConfigs(ctx, "something/file.cfg", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, []config.Config{
					{
						Meta: config.Meta{
							ConfigSet:   "projects/doodly",
							Path:        "something/file.cfg",
							ContentHash: "v1:5a2f9983dbb615a58e1d267633396e72f6710ef2",
							Revision:    expectedRev,
							ViewURL:     "file://./something/file.cfg",
						},
						Content: "projects/doodly/something/file.cfg",
					},
					{
						Meta: config.Meta{
							ConfigSet:   "projects/foobar",
							Path:        "something/file.cfg",
							ContentHash: "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8",
							Revision:    expectedRev,
							ViewURL:     "file://./something/file.cfg",
						},
						Content: "projects/foobar/something/file.cfg",
					},
				})
			})

			Convey("GetProjects", func() {
				projs, err := client.GetProjects(ctx)
				So(err, ShouldBeNil)
				So(projs, ShouldResemble, []config.Project{
					{
						ID:       "doodly",
						Name:     "doodly",
						RepoType: "FILESYSTEM",
					},
					{
						ID:       "foobar",
						Name:     "A cool project",
						RepoType: "FILESYSTEM",
						RepoURL:  &url.URL{Scheme: "https", Host: "something.example.com"},
					},
				})
			})

		})
	})

	withFolder(map[string]string{
		"projects/doodly/file.cfg": "",
		"projects/woodly/file.cfg": "",
	}, func(folder string) {
		Convey("rereads configs in sloppy mode", t, func() {
			client, err := New(folder)
			So(err, ShouldBeNil)

			cfgs, err := client.GetProjectConfigs(ctx, "file.cfg", false)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/doodly",
						Path:        "file.cfg",
						ContentHash: "v1:a4f9e9ab503a00964c88e696067feed0702a81b3",
						Revision:    "42a1ca1c43387844fafbb1c958a0d12f3ea61347",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/doodly/file.cfg",
				}, {
					Meta: config.Meta{
						ConfigSet:   "projects/woodly",
						Path:        "file.cfg",
						ContentHash: "v1:722c6274d1e657691764199fa1095114a3569dee",
						Revision:    "42a1ca1c43387844fafbb1c958a0d12f3ea61347",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/woodly/file.cfg",
				},
			})

			err = os.WriteFile(
				filepath.Join(folder, filepath.FromSlash("projects/doodly/file.cfg")),
				[]byte("blarg"),
				0666)
			So(err, ShouldBeNil)

			cfgs, err = client.GetProjectConfigs(ctx, "file.cfg", false)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/doodly",
						Path:        "file.cfg",
						ContentHash: "v1:a593942cb7ea9ffcd8ccf2f0fa23c338e23bfecd",
						Revision:    "e841b8e92756491dae3302e1e31f673e0613ad0c",
						ViewURL:     "file://./file.cfg",
					},
					Content: "blarg",
				}, {
					Meta: config.Meta{
						ConfigSet:   "projects/woodly",
						Path:        "file.cfg",
						ContentHash: "v1:722c6274d1e657691764199fa1095114a3569dee",
						Revision:    "e841b8e92756491dae3302e1e31f673e0613ad0c",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/woodly/file.cfg",
				},
			})
		})
	})

	versioned := map[string]string{
		"v1/projects/foobar/something/file.cfg": "",
		"v2/projects/foobar/something/file.cfg": "",
	}

	withFolder(versioned, func(folder string) {
		symlink := filepath.Join(folder, "link")

		Convey("Test versioned Filesystem", t, func() {
			So(errors.FilterFunc(os.Remove(symlink), os.IsNotExist), ShouldBeNil)
			So(os.Symlink(filepath.Join(folder, "v1"), symlink), ShouldBeNil)
			client, err := New(symlink)
			So(err, ShouldBeNil)

			Convey("v1", func() {
				cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", false)
				So(err, ShouldBeNil)
				So(cfg.Content, ShouldEqual, "v1/projects/foobar/something/file.cfg")

				Convey("v2", func() {
					So(errors.Filter(os.Remove(symlink), os.ErrNotExist), ShouldBeNil)
					So(os.Symlink(filepath.Join(folder, "v2"), symlink), ShouldBeNil)

					cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", false)
					So(err, ShouldBeNil)
					So(cfg.Content, ShouldEqual, "v2/projects/foobar/something/file.cfg")
				})
			})

		})
	})

}
