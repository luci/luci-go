// Copyright 2023 The LUCI Authors.
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

package common

import (
	"testing"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGitiles(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateGitilesLocation", t, func(t *ftt.Test) {
		t.Run("valid", func(t *ftt.Test) {
			loc := &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/ok",
				Ref:  "refs/heads/main",
				Path: "infra/config/generated",
			}
			assert.Loosely(t, ValidateGitilesLocation(loc), should.BeNil)
		})

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("ref", func(t *ftt.Test) {
				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{}), should.ErrLike(`ref must start with 'refs/'`))
			})

			t.Run("path", func(t *ftt.Test) {
				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{Ref: "refs/heads/infra", Path: "/abc"}), should.ErrLike(`path must not start with '/'`))
			})

			t.Run("repo", func(t *ftt.Test) {
				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{Ref: "refs/heads/infra", Path: "abc"}), should.ErrLike("repo: not specified"))
				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "hostname",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), should.ErrLike("repo: only https scheme is supported"))

				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/project/.git",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), should.ErrLike("repo: must not end with '.git'"))

				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/a/project/",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), should.ErrLike("repo: must not have '/a/' prefix of a path component"))

				assert.Loosely(t, ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/project/",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), should.ErrLike("repo: must not end with '/'"))
			})
		})
	})
}
