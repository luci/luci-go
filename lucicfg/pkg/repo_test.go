// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRepoKeyFromSpec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spec string
		out  string
		err  string
	}{
		{"https://host.example.com/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},
		{"host.example.com/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},
		{"host/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},

		{"", "", "doesn't look like a repository URL"},
		{"host", "", "doesn't look like a repository URL"},
		{"host/", "", "doesn't look like a repository URL"},
		{"host/+/refs/heads/main", "", "doesn't look like a repository URL"},
		{"host/repo", "", "the repo spec is missing a ref"},
	}

	for _, cs := range cases {
		t.Run(cs.spec, func(t *testing.T) {
			out, err := RepoKeyFromSpec(cs.spec)
			if cs.err != "" {
				assert.That(t, err, should.ErrLike(cs.err))
			} else {
				assert.NoErr(t, err)
				assert.That(t, fmt.Sprintf("%s:%s:%s", out.Host, out.Repo, out.Ref), should.Equal(cs.out))
			}
		})
	}
}
