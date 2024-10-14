// Copyright 2017 The LUCI Authors.
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

package gitiles

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPagingLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("PagingLog", t, func(t *ftt.Test) {
		var reqs []http.Request
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			reqs = append(reqs, *r)

			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n")
			if r.FormValue("s") == "" {
				fmt.Fprintf(w, `{"log": [%s], "next": "next_cursor_value"}`, fakeCommit1Str)
			} else {
				fmt.Fprintf(w, `{"log": [%s]}`, fakeCommit2Str)
			}
		})
		defer srv.Close()

		t.Run("Page till no cursor", func(t *ftt.Test) {
			req := &gitiles.LogRequest{
				Project:            "repo",
				ExcludeAncestorsOf: "master",
				Committish:         "8de6836858c99e48f3c58164ab717bda728e95dd",
			}
			commits, err := PagingLog(ctx, c, req, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reqs, should.HaveLength(2))
			assert.Loosely(t, reqs[0].FormValue("s"), should.BeEmpty)
			assert.Loosely(t, reqs[1].FormValue("s"), should.Equal("next_cursor_value"))
			assert.Loosely(t, len(commits), should.Equal(2))
			assert.Loosely(t, commits[0].Author.Name, should.Equal("Author 1"))
			assert.Loosely(t, commits[1].Id, should.Equal("dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40"))
		})

		t.Run("Page till limit", func(t *ftt.Test) {
			req := &gitiles.LogRequest{
				Project:    "repo",
				Committish: "master",
			}
			commits, err := PagingLog(ctx, c, req, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reqs, should.HaveLength(1))
			assert.Loosely(t, reqs[0].FormValue("n"), should.Equal("1"))
			assert.Loosely(t, len(commits), should.Equal(1))
			assert.Loosely(t, commits[0].Author.Name, should.Equal("Author 1"))
		})
	})
}
