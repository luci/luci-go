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

		Convey("GetConfigs", func() {
			out, err := impl.GetConfigs(ctx, "projects/proj2", nil, false)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, map[string]config.Config{
				"another/file": {
					Meta: config.Meta{
						ConfigSet:   "projects/proj2",
						Path:        "another/file",
						ContentHash: "v2:d63f27b6c9bec5886662ba0378c6b49c08fc68c4b9f0cddf5d558bbe4c82592a",
						Revision:    "d8d48bd9c29f7a3cb1a88fe69028b74f71f22fb4",
						ViewURL:     "https://example.com/view/here/another/file",
					},
					Content: "project2 another file",
				},
				"file": {
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
			})
		})
	})
}
