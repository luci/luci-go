// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package filesystem

import (
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
)

func withFolder(files map[string]string, cb func(folder string)) {
	folder, err := ioutil.TempDir("", "fs_test_")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(folder)

	for fpath, content := range files {
		if content == "" {
			content = filepath.ToSlash(fpath)
		}
		fpath = filepath.Join(folder, fpath)
		if err := os.MkdirAll(filepath.Dir(fpath), 0777); err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(fpath, []byte(content), 0555); err != nil {
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
					ConfigSet:   "projects/foobar",
					Path:        "something/file.cfg",
					Content:     "projects/foobar/something/file.cfg",
					ContentHash: "v1:e42874cc28bbba410f56790c24bb6f33e73ab784",
					Revision:    "current",
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
						ConfigSet:   "services/foosrv",
						Path:        "something.cfg",
						Content:     "services/foosrv/something.cfg",
						ContentHash: "v1:71ecbefbed9d895b71205724d3e693bc2ec12246",
						Revision:    "current",
					})
				})

				Convey("refs", func() {
					cfg, err := client.GetConfig(ctx, "projects/foobar/refs/someref", "file.cfg", false)
					So(err, ShouldBeNil)
					So(cfg, ShouldResemble, &config.Config{
						ConfigSet:   "projects/foobar/refs/someref",
						Path:        "file.cfg",
						Content:     "projects/foobar/refs/someref/file.cfg",
						ContentHash: "v1:82b0518dd04288c285023ff0534658e5b0df93d4",
						Revision:    "current",
					})
				})

				Convey("just hash", func() {
					cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", true)
					So(err, ShouldBeNil)
					So(cfg.ContentHash, ShouldEqual, "v1:e42874cc28bbba410f56790c24bb6f33e73ab784")

					Convey("make sure it doesn't poison the cache", func() {
						cfg, err := client.GetConfig(ctx, "projects/foobar", "something/file.cfg", false)
						So(err, ShouldBeNil)
						So(cfg, ShouldResemble, expect)
					})
				})
			})

			Convey("GetConfigByHash", func() {
				cont, err := client.GetConfigByHash(ctx, "v1:e42874cc28bbba410f56790c24bb6f33e73ab784")
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
						ConfigSet:   "projects/doodly",
						Path:        "something/file.cfg",
						Content:     "projects/doodly/something/file.cfg",
						ContentHash: "v1:a3b4b34e5c8dd1dd8dff3e643504ce28f9335e6f",
						Revision:    "current",
					},
					{
						ConfigSet:   "projects/foobar",
						Path:        "something/file.cfg",
						Content:     "projects/foobar/something/file.cfg",
						ContentHash: "v1:e42874cc28bbba410f56790c24bb6f33e73ab784",
						Revision:    "current",
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
						ConfigSet:   "projects/doodly/refs/otherref",
						Path:        "file.cfg",
						Content:     "projects/doodly/refs/otherref/file.cfg",
						ContentHash: "v1:0de822c33630b5be0aa78497c0918e0dd773c7cb",
						Revision:    "current",
					}, {
						ConfigSet:   "projects/doodly/refs/someref",
						Path:        "file.cfg",
						Content:     "projects/doodly/refs/someref/file.cfg",
						ContentHash: "v1:5e9963aa1551a9e9db8e7bebe6164c3b5d8aee97",
						Revision:    "current",
					}, {
						ConfigSet:   "projects/foobar/refs/someref",
						Path:        "file.cfg",
						Content:     "projects/foobar/refs/someref/file.cfg",
						ContentHash: "v1:82b0518dd04288c285023ff0534658e5b0df93d4",
						Revision:    "current",
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
