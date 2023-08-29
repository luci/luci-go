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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGitiles(t *testing.T) {
	t.Parallel()

	Convey("ValidateGitilesLocation", t, func() {
		Convey("valid", func() {
			loc := &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/ok",
				Ref:  "refs/heads/main",
				Path: "infra/config/generated",
			}
			So(ValidateGitilesLocation(loc), ShouldBeNil)
		})

		Convey("invalid", func() {
			Convey("ref", func() {
				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{}), ShouldErrLike, `ref must start with 'refs/'`)
			})

			Convey("path", func() {
				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{Ref: "refs/heads/infra", Path: "/abc"}), ShouldErrLike, `path must not start with '/'`)
			})

			Convey("repo", func() {
				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{Ref: "refs/heads/infra", Path: "abc"}), ShouldErrLike, "repo: not specified")
				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "hostname",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), ShouldErrLike, "repo: only https scheme is supported")

				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/project/.git",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), ShouldErrLike, "repo: must not end with '.git'")

				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/a/project/",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), ShouldErrLike, "repo: must not have '/a/' prefix of a path component")

				So(ValidateGitilesLocation(&cfgcommonpb.GitilesLocation{
					Repo: "https://a.googlesource.com/project/",
					Ref:  "refs/heads/infra",
					Path: "dir/abc",
				}), ShouldErrLike, "repo: must not end with '/'")
			})
		})
	})
}
