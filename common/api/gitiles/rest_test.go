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
	"net/http/httptest"
	"sort"
	"testing"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestNewRESTClient(t *testing.T) {
	t.Parallel()

	ftt.Run("accept valid gitiles host", t, func(t *ftt.Test) {
		c, err := NewRESTClient(&http.Client{}, "chromium.googlesource.com", false)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("accept valid gitiles host with dash", t, func(t *ftt.Test) {
		c, err := NewRESTClient(&http.Client{}, "chromium-foo.googlesource.com", false)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Reject invalid gitiles host", t, func(t *ftt.Test) {
		c, err := NewRESTClient(&http.Client{}, "chromium.hijacked.com", false)
		assert.Loosely(t, c, should.BeNil)
		assert.Loosely(t, err, should.ErrLike("is not a valid Gitiles host"))
	})

	ftt.Run("Reject invalid gitiles host with matching suffix", t, func(t *ftt.Test) {
		c, err := NewRESTClient(&http.Client{}, "chromium.hijacked.com/chromium.googlesource.com", false)
		assert.Loosely(t, c, should.BeNil)
		assert.Loosely(t, err, should.ErrLike("is not a valid Gitiles host"))
	})
}

func TestLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Log with bad project", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {})
		defer srv.Close()
		_, err := c.Log(ctx, &gitiles.LogRequest{})
		assert.Loosely(t, err, should.ErrLike("project is required"))
	})

	ftt.Run("Log w/o pages", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n{\"log\": [%s, %s]}\n", fakeCommit1Str, fakeCommit2Str)
		})
		defer srv.Close()

		t.Run("Return All", func(t *ftt.Test) {
			res, err := c.Log(ctx, &gitiles.LogRequest{
				Project:            "repo",
				Committish:         "8de6836858c99e48f3c58164ab717bda728e95dd",
				ExcludeAncestorsOf: "master",
				PageSize:           10,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(res.Log), should.Equal(2))
			assert.Loosely(t, res.Log[0].Author.Name, should.Equal("Author 1"))
			assert.Loosely(t, res.Log[1].Id, should.Equal("dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40"))
		})
	})
}

func TestRefs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("bad project", t, func(t *ftt.Test) {
		c := &client{BaseURL: "https://a.googlesource.com/a"}
		_, err := c.Refs(ctx, &gitiles.RefsRequest{
			RefsPath: gitiles.AllRefs,
		})
		assert.Loosely(t, err, should.ErrLike(`project is required`))
	})

	ftt.Run("bad RefsPath", t, func(t *ftt.Test) {
		c := &client{BaseURL: "https://a.googlesource.com/a"}
		_, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: "bad",
		})
		assert.Loosely(t, err, should.ErrLike(`refsPath must be "refs" or start with "refs/"`))
	})

	ftt.Run("Refs All", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"refs/heads/master": { "value": "deadbeef" },
					"refs/heads/infra/config": { "value": "0000beef" },
					"refs/changes/01/123001/1": { "value": "123dead001beef1" },
					"refs/other/ref": { "value": "ba6" },
					"123deadbeef123": { "target": "f00" },
					"HEAD": {
						"value": "deadbeef",
						"target": "refs/heads/master"
					}
				}
			`)
		})
		defer srv.Close()

		res, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: gitiles.AllRefs,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.Revisions, should.Resemble(map[string]string{
			"HEAD":                     "refs/heads/master",
			"refs/heads/master":        "deadbeef",
			"refs/heads/infra/config":  "0000beef",
			"refs/other/ref":           "ba6",
			"refs/changes/01/123001/1": "123dead001beef1",
			// Skipping "123dead001beef1" which has no value.
		}))
	})
	ftt.Run("Refs heads", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"master": { "value": "deadbeef" },
					"infra/config": { "value": "0000beef" }
				}
			`)
		})
		defer srv.Close()

		res, err := c.Refs(ctx, &gitiles.RefsRequest{
			Project:  "repo",
			RefsPath: gitiles.Branches,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.Revisions, should.Resemble(map[string]string{
			"refs/heads/master":       "deadbeef",
			"refs/heads/infra/config": "0000beef",
		}))
	})
}

func TestGetProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("GetProject", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"name": "bar",
					"clone_url": "https://foo.googlesource.com/bar"
				}
			`)
		})
		defer srv.Close()

		res, err := c.GetProject(ctx, &gitiles.GetProjectRequest{Name: "bar"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.Resemble(&gitiles.Project{
			Name:     "bar",
			CloneUrl: "https://foo.googlesource.com/bar",
		}))
	})
}

func TestProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("List Projects", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"All-Projects": {
						"name": "All-Projects",
						"clone_url": "https://foo.googlesource.com/All-Projects",
						"description": "foo"
					},
					"bar": {
						"name": "bar",
						"clone_url": "https://foo.googlesource.com/bar",
						"description": "bar description"
					}
				}
			`)
		})
		defer srv.Close()

		res, err := c.Projects(ctx, &gitiles.ProjectsRequest{})
		assert.Loosely(t, err, should.BeNil)
		// Sort project to make it deterministic
		sort.Strings(res.Projects)
		assert.Loosely(t, res.Projects, should.Resemble([]string{
			"All-Projects",
			"bar",
		}))
	})
}

func TestList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListFiles", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintln(w, `)]}'
				{
					"id": "860c9c8901a8de14bf9f6210257f2400ac19d11e",
					"entries": [
					{
						"mode": 33188,
						"type": "blob",
						"id": "7e5b457d492b50762386611fc7f1302f23b313cf",
						"name": "foo.txt"
					},
					{
						"mode": 33188,
						"type": "blob",
						"id": "a07e845539145d1fb697c20b75689b25e266d6d6",
						"name": "bar.txt"
					},
					{
						"mode": 33188,
						"type": "tree",
						"id": "b90e845539145d1fb697c20b75689b25e266d6c3",
						"name": "directory"
					},
					{
						"mode": 33188,
						"type": "random type",
						"id": "k93b845539145d1fb697c20b75689b25e266d6q1",
						"name": "new"
					}
					]
				}
			`)
		})
		defer srv.Close()

		res, err := c.ListFiles(ctx, &gitiles.ListFilesRequest{
			Project:    "project",
			Committish: "main",
			Path:       "path/to/dir",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res.Files, should.Resemble([]*git.File{
			{
				Mode: 33188,
				Id:   "7e5b457d492b50762386611fc7f1302f23b313cf",
				Path: "foo.txt",
				Type: git.File_BLOB,
			},
			{
				Mode: 33188,
				Id:   "a07e845539145d1fb697c20b75689b25e266d6d6",
				Path: "bar.txt",
				Type: git.File_BLOB,
			},
			{
				Mode: 33188,
				Id:   "b90e845539145d1fb697c20b75689b25e266d6c3",
				Path: "directory",
				Type: git.File_TREE,
			},
			{
				Mode: 33188,
				Id:   "k93b845539145d1fb697c20b75689b25e266d6q1",
				Path: "new",
				Type: git.File_UNKNOWN,
			},
		}))
	})
}

func newMockClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, gitiles.GitilesClient) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	return srv, &client{BaseURL: srv.URL}
}

var (
	fakeCommit1Str = `{
		"commit": "0b2c5409e58a71c691b05454b55cc5580cc822d1",
		"tree": "3c6f95bc757698cd6aca3c49f88f640fd145ea69",
		"parents": [ "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40" ],
		"author": {
			"name": "Author 1",
			"email": "author1@example.com",
			"time": "Mon Jul 17 15:02:43 2017 -0800"
		},
		"committer": {
			"name": "Commit Bot",
			"email": "commit-bot@chromium.org",
			"time": "Mon Jul 17 15:02:43 2017 +0000"
		},
		"message": "Import wpt@d96d68ed964f9bfc2bb248c2d2fab7a8870dc685\\n\\nCr-Commit-Position: refs/heads/master@{#487078}"
	}`
	fakeCommit2Str = `{
		"commit": "dc1dbf1aa56e4dd4cbfaab61c4d30a35adce5f40",
		"tree": "1ba2335c07915c31597b97a8d824aecc85a996f6",
		"parents": ["8de6836858c99e48f3c58164ab717bda728e95dd"],
		"author": {
			"name": "Author 2",
			"email": "author-2@example.com",
			"time": "Mon Jul 17 15:01:13 2017"
		},
		"committer": {
			"name": "Commit Bot",
			"email": "commit-bot@chromium.org",
			"time": "Mon Jul 17 15:01:13 2017"
		},
		"message": "[Web Payments] User weak ptr in Payment Request\u0027s error callback\\n\\nBug: 742329\\nReviewed-on: https://chromium-review.googlesource.com/570982\\nCr-Commit-Position: refs/heads/master@{#487077}"
  }`
)
