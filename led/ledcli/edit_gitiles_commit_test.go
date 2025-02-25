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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseGitilesURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		url    string
		ref    string
		err    string
		commit *bbpb.GitilesCommit
	}{
		// Failure cases
		{
			url: "",
			ref: "",
			err: "Only *.googlesource.com URLs are supported",
		},
		{
			url: "https://some.gitiles.com/c/bar/+/123",
			ref: "",
			err: "Only *.googlesource.com URLs are supported",
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/invalid/ref",
			ref: "",
			err: "Commit ref should start with `refs/`: \"invalid/ref\"",
		},
		{
			url: "https://chromium-review.googlesource.com/c/chromium/src/+/4812898",
			ref: "",
			err: "Please specify Gitiles URL instead of Gerrit URL",
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/refs/tags/120.0.6045.58",
			ref: "refs/tags/120.0.6045.58",
			err: "Please remove `-ref` flag from the command, ref is already found from gitiles url",
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5000d",
			ref: "",
			err: "Please provide commit ref through `-ref` flag",
		},
		{
			url: "https://chromium.googlesource.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5999d",
			ref: "main",
			err: "Commit ref should start with `refs/`: \"main\"",
		},
		// Success cases
		{
			url: "https://chromium.googlesource.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			ref: "refs/heads/main",
			commit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "b09958322314a4d429f5e335e1a8b8ccb7c5520d",
				Ref:     "refs/heads/main",
			},
		},
		{
			url: "https://chromium.git.corp.google.com/chromium/src/+/b09958322314a4d429f5e335e1a8b8ccb7c5520d",
			ref: "refs/heads/main",
			commit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "b09958322314a4d429f5e335e1a8b8ccb7c5520d",
				Ref:     "refs/heads/main",
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

	ftt.Run(`parseGitilesURL`, t, func(t *ftt.Test) {
		for _, tc := range cases {
			tc := tc
			t.Run(fmt.Sprintf("%q", tc.url), func(t *ftt.Test) {
				commit, err := parseGitilesURL(tc.url, tc.ref)
				if tc.err != "" {
					assert.Loosely(t, commit, should.BeNil)
					assert.Loosely(t, err, should.ErrLike(tc.err))
				} else {
					assert.Loosely(t, commit, should.Match(tc.commit))
					assert.Loosely(t, err, should.BeNil)
				}
			})
		}

	})
}
