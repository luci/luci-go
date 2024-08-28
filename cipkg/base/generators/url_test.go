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

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFetchURLs(t *testing.T) {
	ftt.Run("Test fetch urls", t, func(t *ftt.Test) {
		ctx := context.Background()
		plats := Platforms{}

		t.Run("ok", func(t *ftt.Test) {
			g := &FetchURLs{
				Name: "urls",
				URLs: map[string]FetchURL{
					"something1": {
						URL:  "https://host/path1",
						Mode: 0o777,
					},
					"dir1/something2": {
						URL:           "https://host/path2",
						HashAlgorithm: core.HashAlgorithm_HASH_MD5,
						HashValue:     "abcdef",
					},
				},
			}
			a, err := g.Generate(ctx, plats)
			assert.Loosely(t, err, should.BeNil)

			url := testutils.Assert[*core.Action_Copy](t, a.Spec)
			assert.Loosely(t, url.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
				"something1": {
					Content: &core.ActionFilesCopy_Source_Output_{
						Output: &core.ActionFilesCopy_Source_Output{Name: "urls_2o025r0794", Path: "file"},
					},
					Mode: 0o777,
				},
				"dir1/something2": {
					Content: &core.ActionFilesCopy_Source_Output_{
						Output: &core.ActionFilesCopy_Source_Output{Name: "urls_om04u163h4", Path: "file"},
					},
					Mode: 0o666,
				},
			}))

			{
				assert.Loosely(t, a.Deps, should.HaveLength(2))
				for _, d := range a.Deps {
					u := testutils.Assert[*core.Action_Url](t, d.Spec)
					switch d.Name {
					case "urls_2o025r0794":
						assert.Loosely(t, u.Url, should.Match(&core.ActionURLFetch{
							Url: "https://host/path1",
						}))
					case "urls_om04u163h4":
						assert.Loosely(t, u.Url, should.Match(&core.ActionURLFetch{
							Url:           "https://host/path2",
							HashAlgorithm: core.HashAlgorithm_HASH_MD5,
							HashValue:     "abcdef",
						}))
					}
				}
			}
		})

		// context info should be inherited since urls and url are treated as one
		// thing.
		t.Run("context info", func(t *ftt.Test) {
			g := &FetchURLs{
				Name:     "urls",
				Metadata: &core.Action_Metadata{ContextInfo: "info"},
				URLs: map[string]FetchURL{
					"something1": {
						URL:  "https://host/path1",
						Mode: 0o777,
					},
				},
			}
			a, err := g.Generate(ctx, plats)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, a.Metadata.GetContextInfo(), should.Equal("info"))
			assert.Loosely(t, a.Deps, should.HaveLength(1))
			assert.Loosely(t, a.Deps[0].Metadata.GetContextInfo(), should.Equal("info"))
		})

		t.Run("stable", func(t *ftt.Test) {
			g := &FetchURLs{
				Name: "urls",
				URLs: map[string]FetchURL{
					"something1": {
						URL:  "https://host/path1",
						Mode: 0o777,
					},
					"dir1/something2": {
						URL:           "https://host/path2",
						HashAlgorithm: core.HashAlgorithm_HASH_MD5,
						HashValue:     "abcdef",
					},
				},
			}
			a, err := g.Generate(ctx, plats)
			assert.Loosely(t, err, should.BeNil)

			for range 10 {
				aa, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, aa, should.Match(a))
			}
		})
	})

}
