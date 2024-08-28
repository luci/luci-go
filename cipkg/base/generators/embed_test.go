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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run("Test embedded files", t, func(t *ftt.Test) {
		ctx := context.Background()
		plats := Platforms{}

		t.Run("Test normal", func(t *ftt.Test) {
			a, err := embeddedFilesTestGen.Generate(ctx, plats)
			assert.Loosely(t, err, should.BeNil)

			embedFiles := testutils.Assert[*core.Action_Copy](t, a.Spec)
			assert.Loosely(t, embedFiles.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
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
			}))
		})
		t.Run("Test subdir", func(t *ftt.Test) {
			a, err := embeddedFilesTestGen.SubDir("testdata").Generate(ctx, plats)
			assert.Loosely(t, err, should.BeNil)

			embedFiles := testutils.Assert[*core.Action_Copy](t, a.Spec)
			assert.Loosely(t, embedFiles.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
				"embed": {
					Content: &core.ActionFilesCopy_Source_Embed_{
						Embed: &core.ActionFilesCopy_Source_Embed{Ref: embeddedFilesTestGen.ref, Path: "testdata/embed"},
					},
					Mode: 0o444,
				},
			}))
		})
	})
}
