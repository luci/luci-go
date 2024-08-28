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
	"testing"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCIPDExport(t *testing.T) {
	ftt.Run("Test cipd export", t, func(t *ftt.Test) {
		ctx := context.Background()
		plats := Platforms{}

		g := &CIPDExport{
			Ensure: ensure.File{
				PackagesBySubdir: map[string]ensure.PackageSlice{
					"": {
						{PackageTemplate: "infra/3pp/tools/git", UnresolvedVersion: "version:2@2.36.1.chromium.8"},
					},
				},
			},
			ServiceURL: "http://something",
		}
		a, err := g.Generate(ctx, plats)
		assert.Loosely(t, err, should.BeNil)

		cipd := testutils.Assert[*core.Action_Cipd](t, a.Spec)
		assert.Loosely(t, cipd.Cipd.EnsureFile, should.Equal("infra/3pp/tools/git  version:2@2.36.1.chromium.8\n"))
		assert.Loosely(t, cipd.Cipd.Env, should.Match([]string{
			"CIPD_SERVICE_URL=http://something",
		}))
	})
}
