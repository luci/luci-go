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
			gs, err := FetchURLs("urls", []*FetchURL{
				{
					URL:      "https://host/path1",
					Filename: "something1",
					Mode:     0o777,
				},
				{
					URL:           "https://host/path2",
					HashAlgorithm: core.HashAlgorithm_HASH_MD5,
					Filename:      "something2",
					HashValue:     "abcdef",
				},
			})
			assert.Loosely(t, err, should.BeNil)

			var as []*core.Action
			for _, g := range gs {
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				as = append(as, a)
			}

			{
				assert.Loosely(t, as, should.HaveLength(2))
				assert.Loosely(t, as[0].Name, should.Equal("urls_9k6l6isoio"))
				assert.Loosely(t, testutils.Assert[*core.Action_Url](t, as[0].Spec).Url, should.Match(&core.ActionURLFetch{
					Name: "something1",
					Url:  "https://host/path1",
					Mode: 0o777,
				}))
				assert.Loosely(t, as[1].Name, should.Equal("urls_oeinoi9b4g"))
				assert.Loosely(t, testutils.Assert[*core.Action_Url](t, as[1].Spec).Url, should.Match(&core.ActionURLFetch{
					Name:          "something2",
					Url:           "https://host/path2",
					HashAlgorithm: core.HashAlgorithm_HASH_MD5,
					HashValue:     "abcdef",
				}))
			}
		})

		t.Run("stable", func(t *ftt.Test) {
			gs, err := FetchURLs("urls", []*FetchURL{
				{
					URL:      "https://host/path1",
					Filename: "something1",
					Mode:     0o777,
				},
				{
					URL:           "https://host/path2",
					HashAlgorithm: core.HashAlgorithm_HASH_MD5,
					Filename:      "something2",
					HashValue:     "abcdef",
				},
			})

			gen := func() []*core.Action {
				var as []*core.Action
				for _, g := range gs {
					a, err := g.Generate(ctx, plats)
					assert.Loosely(t, err, should.BeNil)
					as = append(as, a)
				}
				assert.Loosely(t, err, should.BeNil)
				return as
			}

			as := gen()
			for range 10 {
				assert.Loosely(t, gen(), should.Match(as))
			}
		})
	})
}
