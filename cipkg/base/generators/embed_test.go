// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"context"
	"embed"
	"io/fs"
	"path"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"

	. "github.com/smartystreets/goconvey/convey"
)

//go:embed embed.go embed_test.go testdata
var embeddedFilesTestEmbed embed.FS
var embeddedFilesTestGen = InitEmbeddedFS(
	"embedded_files_test", embeddedFilesTestEmbed,
).WithModeOverride(func(name string) (fs.FileMode, error) {
	if path.Dir(name) == "testdata" {
		return fs.ModePerm, nil
	}
	return 0o444, nil
})

func TestEmbeddedFiles(t *testing.T) {
	Convey("Test embedded files", t, func() {
		ctx := context.Background()
		plats := Platforms{}

		Convey("Test normal", func() {
			a, err := embeddedFilesTestGen.Generate(ctx, plats)
			So(err, ShouldBeNil)

			embedFiles := testutils.Assert[*core.Action_Copy](t, a.Spec)
			So(embedFiles.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
				"embed.go": {
					Content: &core.ActionFilesCopy_Source_Embed_{
						Embed: &core.ActionFilesCopy_Source_Embed{Ref: embeddedFilesTestGen.ref, Path: "embed.go"},
					},
					Mode: 0o444,
				},
				"embed_test.go": {
					Content: &core.ActionFilesCopy_Source_Embed_{
						Embed: &core.ActionFilesCopy_Source_Embed{Ref: embeddedFilesTestGen.ref, Path: "embed_test.go"},
					},
					Mode: 0o444,
				},
				filepath.FromSlash("testdata/embed"): {
					Content: &core.ActionFilesCopy_Source_Embed_{
						Embed: &core.ActionFilesCopy_Source_Embed{Ref: embeddedFilesTestGen.ref, Path: "testdata/embed"},
					},
					Mode: 0o777,
				},
			})
		})
		Convey("Test subdir", func() {
			a, err := embeddedFilesTestGen.SubDir("testdata").Generate(ctx, plats)
			So(err, ShouldBeNil)

			embedFiles := testutils.Assert[*core.Action_Copy](t, a.Spec)
			So(embedFiles.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
				"embed": {
					Content: &core.ActionFilesCopy_Source_Embed_{
						Embed: &core.ActionFilesCopy_Source_Embed{Ref: embeddedFilesTestGen.ref, Path: "testdata/embed"},
					},
					Mode: 0o444,
				},
			})
		})
	})
}
