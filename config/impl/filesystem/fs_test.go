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
		if err := ioutil.WriteFile(fpath, []byte(content), 0666); err != nil {
			panic(err)
		}
	}

	cb(folder)
}

func TestFSImpl(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	withFolder(map[string]string{
		"projects/doodly/refs/otherref/file.cfg": "",
		"projects/doodly/refs/someref/file.cfg":  "",
		"projects/doodly/something/file.cfg":     "",
		"projects/foobar/refs/someref/file.cfg":  "",
		"projects/foobar/refs/bad.cfg":           "",
		"projects/foobar/something/file.cfg":     "",
		"services/foosrv/something.cfg":          "",
		"projects/foobar.json": `{
			"Name": "A cool project",
			"Url": "https://something.example.com"
		}`,
	}, func(folder string) {
		Convey("basic Test Filesystem config client", t, func() {
			client, err := New(folder)
			So(err, ShouldBeNil)

			Convey("GetConfig", func() {
				expect := &config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/foobar",
						Path:        "something/file.cfg",
						ContentHash: "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8",
						Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
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
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./something.cfg",
						},
						Content: "services/foosrv/something.cfg",
					})
				})

				Convey("refs", func() {
					cfg, err := client.GetConfig(ctx, "projects/foobar/refs/someref", "file.cfg", false)
					So(err, ShouldBeNil)
					So(cfg, ShouldResemble, &config.Config{
						Meta: config.Meta{
							ConfigSet:   "projects/foobar/refs/someref",
							Path:        "file.cfg",
							ContentHash: "v1:85f257991d9a52621072fdcf9c54648fc0dc65ae",
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./file.cfg",
						},
						Content: "projects/foobar/refs/someref/file.cfg",
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

			Convey("ListFiles", func() {
				cfg, err := client.ListFiles(ctx, "projects/foobar")
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, []string{"something/file.cfg"})
			})

			Convey("GetConfigByHash", func() {
				cont, err := client.GetConfigByHash(ctx, "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8")
				So(err, ShouldBeNil)
				So(cont, ShouldEqual, "projects/foobar/something/file.cfg")
			})

			Convey("GetConfigSetLocation", func() {
				csurl, err := client.GetConfigSetLocation(ctx, "projects/foobar")
				So(err, ShouldBeNil)
				So(csurl, ShouldResemble, &url.URL{
					Scheme: "file",
					Path:   filepath.ToSlash(folder) + "/projects/foobar",
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
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./something/file.cfg",
						},
						Content: "projects/doodly/something/file.cfg",
					},
					{
						Meta: config.Meta{
							ConfigSet:   "projects/foobar",
							Path:        "something/file.cfg",
							ContentHash: "v1:72b8fe0ecd5e7560762aed58063aeb3795e69bd8",
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
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

			Convey("GetRefConfigs", func() {
				cfgs, err := client.GetRefConfigs(ctx, "file.cfg", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, []config.Config{
					{
						Meta: config.Meta{
							ConfigSet:   "projects/doodly/refs/otherref",
							Path:        "file.cfg",
							ContentHash: "v1:b6f37a2fa013a9ece56907518c0c832550db27cf",
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./file.cfg",
						},
						Content: "projects/doodly/refs/otherref/file.cfg",
					}, {
						Meta: config.Meta{
							ConfigSet:   "projects/doodly/refs/someref",
							Path:        "file.cfg",
							ContentHash: "v1:fdb2befae2c5c5102d58c3467baa888f04e6d4a7",
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./file.cfg",
						},
						Content: "projects/doodly/refs/someref/file.cfg",
					}, {
						Meta: config.Meta{
							ConfigSet:   "projects/foobar/refs/someref",
							Path:        "file.cfg",
							ContentHash: "v1:85f257991d9a52621072fdcf9c54648fc0dc65ae",
							Revision:    "166cd0b9905bb411204477155f04d9ce8d11423c",
							ViewURL:     "file://./file.cfg",
						},
						Content: "projects/foobar/refs/someref/file.cfg",
					},
				})
			})

			Convey("GetRefs", func() {
				refs, err := client.GetRefs(ctx, "foobar")
				So(err, ShouldBeNil)
				So(refs, ShouldResemble, []string{"refs/someref"})

				refs, err = client.GetRefs(ctx, "doodly")
				So(err, ShouldBeNil)
				So(refs, ShouldResemble, []string{"refs/otherref", "refs/someref"})
			})

		})
	})

	withFolder(map[string]string{
		"projects/doodly/refs/otherref/file.cfg": "",
		"projects/doodly/refs/someref/file.cfg":  "",
	}, func(folder string) {
		Convey("rereads configs in sloppy mode", t, func() {
			client, err := New(folder)
			So(err, ShouldBeNil)

			cfgs, err := client.GetRefConfigs(ctx, "file.cfg", false)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/doodly/refs/otherref",
						Path:        "file.cfg",
						ContentHash: "v1:b6f37a2fa013a9ece56907518c0c832550db27cf",
						Revision:    "a43ec497cc2e05d4ee95bc62a1f009077c313695",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/doodly/refs/otherref/file.cfg",
				}, {
					Meta: config.Meta{
						ConfigSet:   "projects/doodly/refs/someref",
						Path:        "file.cfg",
						ContentHash: "v1:fdb2befae2c5c5102d58c3467baa888f04e6d4a7",
						Revision:    "a43ec497cc2e05d4ee95bc62a1f009077c313695",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/doodly/refs/someref/file.cfg",
				},
			})

			err = ioutil.WriteFile(
				filepath.Join(folder, filepath.FromSlash("projects/doodly/refs/otherref/file.cfg")),
				[]byte("blarg"),
				0666)
			So(err, ShouldBeNil)

			cfgs, err = client.GetRefConfigs(ctx, "file.cfg", false)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/doodly/refs/otherref",
						Path:        "file.cfg",
						ContentHash: "v1:a593942cb7ea9ffcd8ccf2f0fa23c338e23bfecd",
						Revision:    "bc00a88a260b1ebfe1a84d71adea647afea28372",
						ViewURL:     "file://./file.cfg",
					},
					Content: "blarg",
				}, {
					Meta: config.Meta{
						ConfigSet:   "projects/doodly/refs/someref",
						Path:        "file.cfg",
						ContentHash: "v1:fdb2befae2c5c5102d58c3467baa888f04e6d4a7",
						Revision:    "bc00a88a260b1ebfe1a84d71adea647afea28372",
						ViewURL:     "file://./file.cfg",
					},
					Content: "projects/doodly/refs/someref/file.cfg",
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
