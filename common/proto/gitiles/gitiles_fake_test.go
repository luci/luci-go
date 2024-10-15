// Copyright 2020 The LUCI Authors.
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
	"strconv"
	"testing"

	"go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Implements GitilesClient", t, func(t *ftt.Test) {
		var _ GitilesClient = &Fake{}
		assert.Loosely(t, nil, should.BeNil)
	})

	ftt.Run("Edge cases", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			fake := Fake{}
			t.Run("Projects", func(t *ftt.Test) {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetProjects(), should.BeEmpty)
			})
			t.Run("Revs", func(t *ftt.Test) {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				_, err := fake.Refs(ctx, in)
				assert.Loosely(t, err, should.ErrLike("Repository not found"))
			})
		})

		t.Run("Empty project", func(t *ftt.Test) {
			fake := Fake{}
			fake.SetRepository("foo", nil, nil)
			t.Run("Projects", func(t *ftt.Test) {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetProjects(), should.Resemble([]string{"foo"}))
			})

			t.Run("Revs", func(t *ftt.Test) {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				out, err := fake.Refs(ctx, in)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, out.GetRevisions(), should.BeEmpty)
			})
		})

		t.Run("Empty repository", func(t *ftt.Test) {
			fake := Fake{}
			refs := map[string]string{"refs/heads/main": ""}
			fake.SetRepository("foo", refs, nil)
			t.Run("Projects", func(t *ftt.Test) {
				resp, err := fake.Projects(ctx, &ProjectsRequest{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp.GetProjects(), should.Resemble([]string{"foo"}))
			})

			t.Run("Revs", func(t *ftt.Test) {
				in := &RefsRequest{
					Project: "foo",
				}
				// Repository not found
				out, err := fake.Refs(ctx, in)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, out.GetRevisions(), should.Resemble(refs))
			})
		})

		t.Run("Reference points to invalid revision", func(t *ftt.Test) {
			fake := Fake{}
			assert.Loosely(t, func() {
				fake.SetRepository("foo", map[string]string{"foo": "bar"}, nil)
			}, should.Panic)
		})

		t.Run("Duplicate commit", func(t *ftt.Test) {
			fake := Fake{}
			assert.Loosely(t, func() {
				commits := []*git.Commit{
					{
						Id: "bar",
					},
					{
						Id: "bar",
					},
				}
				fake.SetRepository("foo", map[string]string{"foo": "bar"}, commits)
			}, should.Panic)
		})

		t.Run("Broken commit chain", func(t *ftt.Test) {
			fake := Fake{}
			commits := []*git.Commit{
				{
					Id:      "bar",
					Parents: []string{"baz"},
				},
			}
			fake.SetRepository("foo", map[string]string{"refs/heads/main": "bar"}, commits)
			assert.Loosely(t, func() {
				in := &LogRequest{
					Project:    "foo",
					Committish: "refs/heads/main",
				}
				fake.Log(ctx, in)
			}, should.Panic)
		})
	})

	ftt.Run("Linear commits", t, func(t *ftt.Test) {
		// Commit structure: 9 -> 8 -> 7 -> 6 -> 5 -> 4 -> 3 -> 2 -> 1 -> 0
		commits := make([]*git.Commit, 10)
		for i := 0; i < 10; i++ {
			commits[i] = &git.Commit{
				Id: strconv.Itoa(i),
			}
			if i > 0 {
				commits[i].Parents = []string{commits[i-1].GetId()}
			}
		}
		refs := map[string]string{
			"refs/heads/main":   "9",
			"refs/heads/stable": "1",
		}
		fake := Fake{}
		fake.SetRepository("foo", refs, commits)

		t.Run("Revs", func(t *ftt.Test) {
			in := &RefsRequest{
				Project: "foo",
			}
			out, err := fake.Refs(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.GetRevisions(), should.Resemble(refs))
		})

		t.Run("Log pagination main", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/main",
				PageSize:   5,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(out.GetLog()), should.Equal(5))
			// Last element returned is 5, which is the token
			assert.Loosely(t, out.GetNextPageToken(), should.Equal("5"))

			in.PageToken = out.GetNextPageToken()
			out, err = fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(out.GetLog()), should.Equal(5))
			// Last element returned is 0, which is the token
			assert.Loosely(t, out.GetNextPageToken(), should.Equal("0"))

			// We reached the end
			in.PageToken = out.GetNextPageToken()
			out, err = fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(out.GetLog()), should.BeZero)
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
		})

		t.Run("Log stable branch", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/stable",
				PageSize:   5,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(out.GetLog()), should.Equal(2))
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
		})

		t.Run("Log Non existing branch", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/foo",
				PageSize:   5,
			}
			_, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.ErrLike("commit refs/heads/foo not found"))
		})
	})

	ftt.Run("Merge logs", t, func(t *ftt.Test) {
		// Commit structure: 9 -> 8 -> 7 -> 6 -> 5 -> 4 -> 3 -> 2 -> 1 -> 0
		//                          \_ b2      ->     b1  /
		commits := make([]*git.Commit, 12)
		for i := 0; i < 10; i++ {
			commits[i] = &git.Commit{
				Id: strconv.Itoa(i),
			}
			if i > 0 {
				commits[i].Parents = []string{commits[i-1].GetId()}
			}
		}
		commits[10] = &git.Commit{
			Id:      "b1",
			Parents: []string{"3"},
		}
		commits[11] = &git.Commit{
			Id:      "b2",
			Parents: []string{"b1"},
		}
		// Modify commit with ID 8 to merge commit include b2 as parent.
		commits[8].Parents = []string{"7", "b2"}

		refs := map[string]string{
			"refs/heads/main":    "9",
			"refs/heads/feature": "b2",
			"refs/heads/stable":  "7",
		}
		fake := Fake{}
		fake.SetRepository("foo", refs, commits)

		t.Run("Entire log", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/main",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
			assert.Loosely(t, len(out.GetLog()), should.Equal(12))
		})

		t.Run("Partial log, before merge", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/stable",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
			assert.Loosely(t, len(out.GetLog()), should.Equal(8))
		})

		t.Run("Partial log, before merge (feature branch)", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "refs/heads/feature",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
			assert.Loosely(t, len(out.GetLog()), should.Equal(6))
		})

		t.Run("Partial log, specific commit", func(t *ftt.Test) {
			in := &LogRequest{
				Project:    "foo",
				Committish: "8",
				PageSize:   100,
			}
			out, err := fake.Log(ctx, in)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.GetNextPageToken(), should.BeEmpty)
			assert.Loosely(t, len(out.GetLog()), should.Equal(11))
		})
	})
}
