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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/testutils"
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

		t.Run("headers", func(t *ftt.Test) {
			headers := map[string]string{
				"Referer":    "https://www.amd.com/",
				"User-Agent": "curl/8.5.0",
			}
			gs, err := FetchURLs("urls", []*FetchURL{
				{
					URL:      "https://host/path1",
					Filename: "something1",
					Mode:     0o777,
					Headers:  headers,
				},
			})
			assert.NoErr(t, err)

			a, err := gs[0].Generate(ctx, plats)
			assert.NoErr(t, err)
			headers["Extra"] = "mutated"
			// Headers influence the generated stable ID compared to headerless "urls_9k6l6isoio".
			assert.That(t, a.Name, should.NotEqual("urls_9k6l6isoio"))
			assert.Loosely(t, testutils.Assert[*core.Action_Url](t, a.Spec).Url, should.Match(&core.ActionURLFetch{
				Name: "something1",
				Url:  "https://host/path1",
				Mode: 0o777,
				Headers: map[string]string{
					"Referer":    "https://www.amd.com/",
					"User-Agent": "curl/8.5.0",
				},
			}))

			for range 10 {
				gs2, err := FetchURLs("urls", []*FetchURL{
					{
						URL:      "https://host/path1",
						Filename: "something1",
						Mode:     0o777,
						Headers: map[string]string{
							"User-Agent": "curl/8.5.0",
							"Referer":    "https://www.amd.com/",
						},
					},
				})
				assert.NoErr(t, err)
				a2, err := gs2[0].Generate(ctx, plats)
				assert.NoErr(t, err)
				assert.That(t, a2.Name, should.Equal(a.Name))
			}
		})

		t.Run("HashFromString", func(t *ftt.Test) {
			for name, want := range map[string]core.HashAlgorithm{
				"md5":    core.HashAlgorithm_HASH_MD5,
				"sha1":   core.HashAlgorithm_HASH_SHA1,
				"sha256": core.HashAlgorithm_HASH_SHA256,
				"sha512": core.HashAlgorithm_HASH_SHA512,
			} {
				got, err := HashFromString(name)
				assert.NoErr(t, err)
				assert.That(t, got, should.Equal(want))
			}
			_, err := HashFromString("unknown")
			assert.Loosely(t, err, should.ErrLike("unknown hash algorithem"))
		})
	})
}
