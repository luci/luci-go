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

package ledcli

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseGitilesURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url    string
		err    string
		commit *bbpb.GitilesCommit
	}{
		// Failure cases
		{
			url: "",
			err: "Only *.googlesource.com URLs are supported",
		},
		{
			url: "https://some.gitiles.com/c/bar/+/123",
			err: "Only *.googlesource.com URLs are supported",
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/invalid/ref",
			err: "Commit ref should start with `refs/`: \"invalid/ref\"",
		},
		{
			url: "https://chromium-review.googlesource.com/c/chromium/src/+/4812898",
			err: "Please specify Gitiles URL instead of Gerrit URL",
		},
		// Success cases
		{
			url: "https://chromium.googlesource.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			commit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			},
		},
		{
			url: "https://chromium.git.corp.google.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			commit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			},
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/refs/tags/119.0.6045.58",
			commit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/tags/119.0.6045.58",
			},
		},
	}

	Convey(`parseGitilesURL`, t, func() {
		for _, tc := range cases {
			tc := tc
			Convey(fmt.Sprintf("%q", tc.url), func() {
				commit, err := parseGitilesURL(tc.url)
				if tc.err != "" {
					So(commit, ShouldBeNil)
					So(err, ShouldErrLike, tc.err)
				} else {
					So(commit, ShouldResembleProto, tc.commit)
					So(err, ShouldBeNil)
				}
			})
		}

	})
}
