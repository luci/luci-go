// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMemoryImpl(t *testing.T) {
	Convey("with memory implementation", t, func() {
		ctx := context.Background()
		impl := New(map[config.Set]Files{
			"services/abc": {
				"file": "body",
			},
			"projects/proj1": {
				"file": "project1 file",
			},
			"projects/proj2": {
				"file":         "project2 file",
				"another/file": "project2 another file",
			},
			"projects/proj1/refs/heads/master": {
				"file": "project1 master ref",
			},
			"projects/proj1/refs/heads/other": {
				"file": "project1 other ref",
			},
			"projects/proj2/refs/heads/master": {
				"file": "project2 master ref",
			},
			"projects/proj3/refs/heads/blah": {
				"filezzz": "project2 blah ref",
			},
		})

		Convey("GetConfig works", func() {
			cfg, err := impl.GetConfig(ctx, "services/abc", "file", false)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &config.Config{
				Meta: config.Meta{
					ConfigSet:   "services/abc",
					Path:        "file",
					ContentHash: "v2:ace00670121e87a8e442ea9c1b74c16e95564f9d9ffcdb503a0b44db763c220a",
					Revision:    "4435ce6f8ad97b8b3df8bddf1c9cbe88feed13fb",
					ViewURL:     "https://example.com/view/here/file",
				},
				Content: "body",
			})
		})

		Convey("GetConfig metaOnly works", func() {
			cfg, err := impl.GetConfig(ctx, "services/abc", "file", true)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &config.Config{
				Meta: config.Meta{
					ConfigSet:   "services/abc",
					Path:        "file",
					ContentHash: "v2:ace00670121e87a8e442ea9c1b74c16e95564f9d9ffcdb503a0b44db763c220a",
					Revision:    "4435ce6f8ad97b8b3df8bddf1c9cbe88feed13fb",
					ViewURL:     "https://example.com/view/here/file",
				},
			})
		})

		Convey("ListFiles", func() {
			templates, err := impl.ListFiles(ctx, "projects/proj2")
			So(err, ShouldBeNil)
			So(templates, ShouldResemble, []string{
				"another/file",
				"file",
			})
		})

		Convey("GetConfig missing set", func() {
			cfg, err := impl.GetConfig(ctx, "missing/set", "path", false)
			So(cfg, ShouldBeNil)
			So(err, ShouldEqual, config.ErrNoConfig)
		})

		Convey("GetConfig missing path", func() {
			cfg, err := impl.GetConfig(ctx, "services/abc", "missing file", false)
			So(cfg, ShouldBeNil)
			So(err, ShouldEqual, config.ErrNoConfig)
		})

		Convey("GetConfig returns error when set", func() {
			testErr := errors.New("test error")
			SetError(impl, testErr)
			_, err := impl.GetConfig(ctx, "missing/set", "path", false)
			So(err, ShouldEqual, testErr)

			// Resetting error to nil makes things work again.
			SetError(impl, nil)
			cfg, err := impl.GetConfig(ctx, "services/abc", "missing file", false)
			So(cfg, ShouldBeNil)
			So(err, ShouldEqual, config.ErrNoConfig)
		})

		Convey("GetConfigByHash works", func() {
			body, err := impl.GetConfigByHash(ctx, "v2:ace00670121e87a8e442ea9c1b74c16e95564f9d9ffcdb503a0b44db763c220a")
			So(err, ShouldBeNil)
			So(body, ShouldEqual, "body")
		})

		Convey("GetConfigByHash missing hash", func() {
			body, err := impl.GetConfigByHash(ctx, "v1:blarg")
			So(err, ShouldEqual, config.ErrNoConfig)
			So(body, ShouldEqual, "")
		})

		Convey("GetConfigSetLocation works", func() {
			loc, err := impl.GetConfigSetLocation(ctx, "services/abc")
			So(err, ShouldBeNil)
			So(loc, ShouldNotBeNil)
		})

		Convey("GetConfigSetLocation returns ErrNoConfig for invalid config set", func() {
			_, err := impl.GetConfigSetLocation(ctx, "services/invalid")
			So(err, ShouldEqual, config.ErrNoConfig)
		})

		Convey("GetProjectConfigs works", func() {
			cfgs, err := impl.GetProjectConfigs(ctx, "file", false)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1",
						Path:        "file",
						ContentHash: "v2:844b762dbd1107bf48cd0f13092f1aa310465f058044fb7a4b10eac1217c5622",
						Revision:    "d7d38dcf39d73e6a323ca3326d82b4d6d2a3cf94",
						ViewURL:     "https://example.com/view/here/file",
					},
					Content: "project1 file",
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj2",
						Path:        "file",
						ContentHash: "v2:0098b08f0108cd69b0cc27d152c319dd47e1cfb184f8ee335efa9148fdc204e3",
						Revision:    "d8d48bd9c29f7a3cb1a88fe69028b74f71f22fb4",
						ViewURL:     "https://example.com/view/here/file",
					},
					Content: "project2 file",
				},
			})
		})

		Convey("GetProjectConfigs metaOnly works", func() {
			cfgs, err := impl.GetProjectConfigs(ctx, "file", true)
			So(err, ShouldBeNil)
			So(cfgs, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1",
						Path:        "file",
						ContentHash: "v2:844b762dbd1107bf48cd0f13092f1aa310465f058044fb7a4b10eac1217c5622",
						Revision:    "d7d38dcf39d73e6a323ca3326d82b4d6d2a3cf94",
						ViewURL:     "https://example.com/view/here/file",
					},
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj2",
						Path:        "file",
						ContentHash: "v2:0098b08f0108cd69b0cc27d152c319dd47e1cfb184f8ee335efa9148fdc204e3",
						Revision:    "d8d48bd9c29f7a3cb1a88fe69028b74f71f22fb4",
						ViewURL:     "https://example.com/view/here/file",
					},
				},
			})
		})

		Convey("GetProjectConfigs unknown file", func() {
			cfgs, err := impl.GetProjectConfigs(ctx, "unknown file", false)
			So(err, ShouldBeNil)
			So(len(cfgs), ShouldEqual, 0)
		})

		Convey("GetProjects works", func() {
			proj, err := impl.GetProjects(ctx)
			So(err, ShouldBeNil)
			So(proj, ShouldResemble, []config.Project{
				{
					ID:       "proj1",
					Name:     "Proj1",
					RepoType: config.GitilesRepo,
				},
				{
					ID:       "proj2",
					Name:     "Proj2",
					RepoType: config.GitilesRepo,
				},
				{
					ID:       "proj3",
					Name:     "Proj3",
					RepoType: config.GitilesRepo,
				},
			})
		})

		Convey("GetRefConfigs works", func() {
			cfg, err := impl.GetRefConfigs(ctx, "file", false)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1/refs/heads/master",
						Path:        "file",
						ContentHash: "v2:de383bc97e0b10fa2b0b42453ba6c0cefcc515673531bd7991924927c190741a",
						Revision:    "90cefed10b2934dd8c7ac7520c7316ab556869df",
						ViewURL:     "https://example.com/view/here/file",
					},
					Content: "project1 master ref",
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1/refs/heads/other",
						Path:        "file",
						ContentHash: "v2:f2dab8c4bfe3454e72a6ca7cfe92328757982a7e58534a6db2cf7dd5a83e71d6",
						Revision:    "7bbd45f8551410eadc234c449dd7a0b0097b83a2",
						ViewURL:     "https://example.com/view/here/file",
					},
					Content: "project1 other ref",
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj2/refs/heads/master",
						Path:        "file",
						ContentHash: "v2:cb4e59fc6a8b77f236e06dfa0d5ac4f4d50963dd2e8e6289a0976a992564b0ce",
						Revision:    "ea7efe2c57bd89a74b8961b239d709e4f9983a93",
						ViewURL:     "https://example.com/view/here/file",
					},
					Content: "project2 master ref",
				},
			})
		})

		Convey("GetRefConfigs metaOnly works", func() {
			cfg, err := impl.GetRefConfigs(ctx, "file", true)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, []config.Config{
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1/refs/heads/master",
						Path:        "file",
						ContentHash: "v2:de383bc97e0b10fa2b0b42453ba6c0cefcc515673531bd7991924927c190741a",
						Revision:    "90cefed10b2934dd8c7ac7520c7316ab556869df",
						ViewURL:     "https://example.com/view/here/file",
					},
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj1/refs/heads/other",
						Path:        "file",
						ContentHash: "v2:f2dab8c4bfe3454e72a6ca7cfe92328757982a7e58534a6db2cf7dd5a83e71d6",
						Revision:    "7bbd45f8551410eadc234c449dd7a0b0097b83a2",
						ViewURL:     "https://example.com/view/here/file",
					},
				},
				{
					Meta: config.Meta{
						ConfigSet:   "projects/proj2/refs/heads/master",
						Path:        "file",
						ContentHash: "v2:cb4e59fc6a8b77f236e06dfa0d5ac4f4d50963dd2e8e6289a0976a992564b0ce",
						Revision:    "ea7efe2c57bd89a74b8961b239d709e4f9983a93",
						ViewURL:     "https://example.com/view/here/file",
					},
				},
			})
		})

		Convey("GetRefConfigs no configs", func() {
			cfg, err := impl.GetRefConfigs(ctx, "unknown file", false)
			So(err, ShouldBeNil)
			So(len(cfg), ShouldEqual, 0)
		})

		Convey("GetRefs works", func() {
			refs, err := impl.GetRefs(ctx, "proj1")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, []string{"refs/heads/master", "refs/heads/other"})

			refs, err = impl.GetRefs(ctx, "proj2")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, []string{"refs/heads/master"})
		})

		Convey("GetRefs unknown project", func() {
			refs, err := impl.GetRefs(ctx, "unknown project")
			So(err, ShouldBeNil)
			So(len(refs), ShouldEqual, 0)
		})
	})
}
