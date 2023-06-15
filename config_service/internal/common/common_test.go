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
)

func TestCommon(t *testing.T) {
	t.Parallel()

	Convey("GitilesURL", t, func() {
		So(GitilesURL(nil), ShouldBeEmpty)
		So(GitilesURL(&cfgcommonpb.GitilesLocation{}), ShouldBeEmpty)

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
			Path: "myfile.cfg",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra/+/refs/heads/main/myfile.cfg")

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra/+/refs/heads/main")

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra")
	})
}
