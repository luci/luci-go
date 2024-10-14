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

package gerrit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGerritURL(t *testing.T) {
	t.Parallel()
	ftt.Run("Malformed", t, func(t *ftt.Test) {
		f := func(arg string) {
			assert.Loosely(t, ValidateGerritURL(arg), should.NotBeNil)
			_, err := NormalizeGerritURL(arg)
			assert.Loosely(t, err, should.NotBeNil)
		}

		f("what/\\is\this")
		f("https://example.com/")
		f("http://bad-protocol-review.googlesource.com/")
		f("no-protocol-review.googlesource.com/")
		f("https://a-review.googlesource.com/path-and#fragment")
		f("https://a-review.googlesource.com/any-path-actually")
	})

	ftt.Run("OK", t, func(t *ftt.Test) {
		f := func(arg, exp string) {
			assert.Loosely(t, ValidateGerritURL(arg), should.BeNil)
			act, err := NormalizeGerritURL(arg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, act, should.Equal(exp))
		}
		f("https://a-review.googlesource.com", "https://a-review.googlesource.com/")
		f("https://a-review.googlesource.com/", "https://a-review.googlesource.com/")
		f("https://chromium-review.googlesource.com/", "https://chromium-review.googlesource.com/")
		f("https://chromium-review.googlesource.com", "https://chromium-review.googlesource.com/")
	})
}

func TestNewClient(t *testing.T) {
	t.Parallel()
	ftt.Run("Malformed", t, func(t *ftt.Test) {
		f := func(arg string) {
			_, err := NewClient(http.DefaultClient, arg)
			assert.Loosely(t, err, should.NotBeNil)
		}
		f("badurl")
		f("http://a.googlesource.com")
		f("https://a/")
	})
	ftt.Run("OK", t, func(t *ftt.Test) {
		f := func(arg string) {
			_, err := NewClient(http.DefaultClient, arg)
			assert.Loosely(t, err, should.BeNil)
		}
		f("https://a-review.googlesource.com/")
		f("https://a-review.googlesource.com")
	})
}

func TestQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ChangeQuery", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n[%s]\n", fakeCL1Str)
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			cls, more, err := c.ChangeQuery(ctx,
				ChangeQueryParams{
					Query: "some_query",
				})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(cls), should.Equal(1))
			assert.Loosely(t, cls[0].Owner.AccountID, should.Equal(1118104))
			assert.Loosely(t, more, should.BeFalse)
		})
	})

	ftt.Run("ChangeQuery with more changes", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n[%s]\n", fakeCL2Str)
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			cls, more, err := c.ChangeQuery(ctx,
				ChangeQueryParams{
					Query: "4efbec9a685b238fced35b81b7f3444dc60150b1",
				})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(cls), should.Equal(1))
			assert.Loosely(t, cls[0].Owner.AccountID, should.Equal(1178184))
			assert.Loosely(t, more, should.BeFalse)
		})
	})

	ftt.Run("ChangeQuery returns no changes", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, ")]}'\n[]\n", fakeCL2Str)
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			cls, more, err := c.ChangeQuery(ctx,
				ChangeQueryParams{
					Query: "4efbec9a685b238fced35b81b7f3444dc60150b1",
				})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls, should.Resemble([]*Change{}))
			assert.Loosely(t, more, should.BeFalse)
		})
	})
}

func TestChangeDetails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Details", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL3Str)
		})
		defer srv.Close()

		t.Run("WithOptions", func(t *ftt.Test) {
			options := ChangeDetailsParams{Options: []string{"CURRENT_REVISION"}}
			cl, err := c.ChangeDetails(ctx, "629279", options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cl.RevertOf, should.Equal(629277))
			assert.Loosely(t, cl.CurrentRevision, should.Equal("1ee75012c0de"))
		})

	})

	ftt.Run("Retry", t, func(t *ftt.Test) {
		var attempts int
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			// First attempt fails, second succeeds.
			if attempts == 0 {
				w.WriteHeader(500)
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintf(w, "Internal server error")
			} else {
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, ")]}'\n%s\n", fakeCL3Str)
			}
			attempts++
		})
		defer srv.Close()

		cl, err := c.ChangeDetails(ctx, "629279", ChangeDetailsParams{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cl.RevertOf, should.Equal(629277))
		assert.Loosely(t, cl.CurrentRevision, should.Equal("1ee75012c0de"))
		assert.Loosely(t, attempts, should.Equal(2))
	})
}

func TestListChangeComments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListComments", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeComments1Str)
		})
		defer srv.Close()

		t.Run("WithOptions", func(t *ftt.Test) {
			comments, err := c.ListChangeComments(ctx, "629279", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, comments["foo"][0].Line, should.Equal(3))
			assert.Loosely(t, comments["foo"][0].Range.StartLine, should.Equal(3))
			assert.Loosely(t, comments["bar"][0].Line, should.Equal(21))
		})

	})

}

func TestListRobotComments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("ListRobotComments", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeRobotComments1Str)
		})
		defer srv.Close()

		t.Run("WithOptions", func(t *ftt.Test) {
			comments, err := c.ListRobotComments(ctx, "629279", "deadbeef")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, comments["foo"][0].Line, should.Equal(3))
			assert.Loosely(t, comments["foo"][0].Range.StartLine, should.Equal(3))
			assert.Loosely(t, comments["foo"][0].RobotID, should.Equal("somerobot"))
			assert.Loosely(t, comments["foo"][0].RobotRunID, should.Equal("run1"))
			assert.Loosely(t, comments["bar"][0].Line, should.Equal(21))
		})
	})
}

func TestAccountQuery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Account-Query", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeAccounts1Str)
		})
		defer srv.Close()

		c.Run("WithOptions", func(c *ftt.Test) {
			accounts, more, err := client.AccountQuery(ctx, AccountQueryParams{Query: "email:nobody@example.com"})
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, more, should.Equal(false))
			assert.Loosely(c, accounts[0].Name, should.Equal("John Doe"))
			assert.Loosely(c, accounts[1].Name, should.Equal("Jane Doe"))
		})
	})
}

func TestChangesSubmittedTogether(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("SubmittedTogether", t, func(t *ftt.Test) {
		var nonVisibleResp string
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n{ \"changes\":[%s,%s]%s}\n", fakeCL1Str, fakeCL6Str, nonVisibleResp)
		})
		defer srv.Close()

		t.Run("WithCurrentRevisionOptions", func(t *ftt.Test) {
			nonVisibleResp = ""
			options := ChangeDetailsParams{Options: []string{"CURRENT_REVISION"}}
			cls, err := c.ChangesSubmittedTogether(ctx, "627036", options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls.Changes[0].CurrentRevision, should.Equal("eb2388b592a9"))
			assert.Loosely(t, cls.Changes[1].CurrentRevision, should.Equal("d6375c2ea5b0"))
		})
		t.Run("WithNonVisibleChangesOptions", func(t *ftt.Test) {
			nonVisibleResp = ",\"non_visible_changes\":1"
			options := ChangeDetailsParams{Options: []string{"CURRENT_REVISION", "NON_VISIBLE_CHANGES"}}
			cls, err := c.ChangesSubmittedTogether(ctx, "627036", options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls.Changes[0].CurrentRevision, should.Equal("eb2388b592a9"))
			assert.Loosely(t, cls.Changes[1].CurrentRevision, should.Equal("d6375c2ea5b0"))
			assert.Loosely(t, cls.NonVisibleChanges, should.Equal(1))
		})

	})
}

func TestMergeable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("GetMergeable", t, func(t *ftt.Test) {
		var resp string
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", resp)
		})
		defer srv.Close()

		t.Run("yes", func(t *ftt.Test) {
			resp = `{
				"submit_type": "REBASE_ALWAYS",
				"strategy": "recursive",
				"mergeable": true,
				"commit_merged": false,
				"content_merged": false
			}`
			cls, err := c.GetMergeable(ctx, "627036", "eb2388b592a9")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls.Mergeable, should.Equal(true))
		})

		t.Run("no", func(t *ftt.Test) {
			resp = `{
				"submit_type": "REBASE_ALWAYS",
				"strategy": "recursive",
				"mergeable": false,
				"commit_merged": false,
				"content_merged": false
			}`
			cls, err := c.GetMergeable(ctx, "646267", "d6375c2ea5b0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cls.Mergeable, should.Equal(false))
		})

	})
}

func TestChangeLabels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Labels", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL5Str)
		})
		defer srv.Close()

		t.Run("All", func(t *ftt.Test) {
			options := ChangeDetailsParams{Options: []string{"DETAILED_LABELS"}}
			cl, err := c.ChangeDetails(ctx, "629279", options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(cl.Labels["Code-Review"].All), should.Equal(2))
			assert.Loosely(t, cl.Labels["Code-Review"].All[0].Value, should.Equal(-1))
			assert.Loosely(t, cl.Labels["Code-Review"].All[0].Username, should.Equal("jdoe"))
			assert.Loosely(t, cl.Labels["Code-Review"].All[1].Value, should.Equal(1))
			assert.Loosely(t, cl.Labels["Code-Review"].All[1].Username, should.Equal("jroe"))
			assert.Loosely(t, len(cl.Labels["Code-Review"].Values), should.Equal(5))
			assert.Loosely(t, len(cl.Labels["Verified"].Values), should.Equal(3))
		})

	})

}

func TestCreateChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("CreateChange", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ci ChangeInput
			err := json.NewDecoder(r.Body).Decode(&ci)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			change := Change{
				ID:       fmt.Sprintf("%s~%s~I8473b95934b5732ac55d26311a706c9c2bde9941", ci.Project, ci.Branch),
				ChangeID: "I8473b95934b5732ac55d26311a706c9c2bde9941",
				Project:  ci.Project,
				Branch:   ci.Branch,
				Subject:  ci.Subject,
				Topic:    ci.Topic,
				Status:   "NEW",
				// the rest omitted for brevity...
			}
			var buffer bytes.Buffer
			err = json.NewEncoder(&buffer).Encode(&change)
			assert.Loosely(c, err, should.BeNil)
			fmt.Fprintf(w, ")]}'\n%s\n", buffer.String())
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			ci := ChangeInput{
				Project: "infra/luci-go",
				Branch:  "master",
				Subject: "Let's make a thing. Yeah, a thing.",
				Topic:   "something-something",
			}
			change, err := client.CreateChange(ctx, &ci)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Project, should.Resemble(ci.Project))
			assert.Loosely(c, change.Branch, should.Resemble(ci.Branch))
			assert.Loosely(c, change.Subject, should.Resemble(ci.Subject))
			assert.Loosely(c, change.Topic, should.Resemble(ci.Topic))
			assert.Loosely(c, change.Status, should.Match("NEW"))
		})

	})

	ftt.Run("CreateChange but project non-existent", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "No such project: blah")
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			ci := ChangeInput{
				Project: "blah",
				Branch:  "master",
				Subject: "beep bop boop I'm a robot",
				Topic:   "haha",
			}
			_, err := c.CreateChange(ctx, &ci)
			assert.Loosely(t, err, should.NotBeNil)
		})

	})
}

func TestAbandonChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("AbandonChange", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ai AbandonInput
			err := json.NewDecoder(r.Body).Decode(&ai)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL4Str)
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			change, err := client.AbandonChange(ctx, "629279", nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("ABANDONED"))
		})

		c.Run("Basic with message", func(c *ftt.Test) {
			ai := AbandonInput{
				Message: "duplicate",
			}
			change, err := client.AbandonChange(ctx, "629279", &ai)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("ABANDONED"))
		})
	})

	ftt.Run("AbandonChange but change non-existent", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "No such change: 629279")
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			_, err := c.AbandonChange(ctx, "629279", nil)
			assert.Loosely(t, err, should.NotBeNil)
		})

	})
}

func TestRebase(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("RebaseChange", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri RestoreInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL1Str)
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			change, err := client.RebaseChange(ctx, "627036", nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("NEW"))
		})

		c.Run("Basic with overridden base revision", func(c *ftt.Test) {
			ri := RebaseInput{
				Base:               "abc123",
				OnBehalfOfUploader: true,
				AllowConflicts:     false,
			}
			change, err := client.RebaseChange(ctx, "627036", &ri)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("NEW"))
		})
	})

	ftt.Run("RebaseChange with nontrivial merge conflict", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri RebaseInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(409)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "change has conflicts")
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			_, err := client.RebaseChange(ctx, "627036", nil)
			assert.Loosely(c, err, should.NotBeNil)
		})
	})
}

func TestRestoreChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("RestoreChange", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri RestoreInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, ")]}'\n%s\n", fakeCL1Str)
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			change, err := client.RestoreChange(ctx, "627036", nil)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("NEW"))
		})

		c.Run("Basic with message", func(c *ftt.Test) {
			ri := RestoreInput{
				Message: "restored",
			}
			change, err := client.RestoreChange(ctx, "627036", &ri)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, change.Status, should.Match("NEW"))
		})
	})

	ftt.Run("RestoreChange but change not abandoned", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri RestoreInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(409)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "change is new")
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			_, err := client.RestoreChange(ctx, "627036", nil)
			assert.Loosely(c, err, should.NotBeNil)
		})
	})

	ftt.Run("RestoreChange but change non-existent", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri RestoreInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(404)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "No such change: 629279")
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			_, err := client.RestoreChange(ctx, "629279", nil)
			assert.Loosely(c, err, should.NotBeNil)
		})
	})
}

func TestCreateBranch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	bi := BranchInput{
		Ref:      "branch",
		Revision: "08a8326653eaa5f7aeea30348b63bf5e9595dc11",
	}

	ftt.Run("CreateBranch", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var bi BranchInput
			err := json.NewDecoder(r.Body).Decode(&bi)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			info := BranchInfo{
				Ref:      "branch",
				Revision: "08a8326653eaa5f7aeea30348b63bf5e9595dc11",
			}
			var buffer bytes.Buffer
			err = json.NewEncoder(&buffer).Encode(&info)
			assert.Loosely(c, err, should.BeNil)
			fmt.Fprintf(w, ")]}'\n%s\n", buffer.String())
		})
		defer srv.Close()
		c.Run("Basic", func(c *ftt.Test) {
			info, err := client.CreateBranch(ctx, "project", &bi)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, info.Ref, should.Equal("branch"))
			assert.Loosely(c, info.Revision, should.Equal("08a8326653eaa5f7aeea30348b63bf5e9595dc11"))
		})
	})

	ftt.Run("Not authorized", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var bi BranchInput
			err := json.NewDecoder(r.Body).Decode(&bi)
			assert.Loosely(c, err, should.BeNil)

			w.WriteHeader(403)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "Not authorized to create ref")
		})
		defer srv.Close()

		c.Run("Basic", func(c *ftt.Test) {
			_, err := client.CreateBranch(ctx, "project", &bi)
			assert.Loosely(c, err, should.NotBeNil)
		})
	})
}

func TestIsPureRevert(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("IsPureRevert", t, func(t *ftt.Test) {
		t.Run("Bad change id", func(t *ftt.Test) {
			srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintf(w, "Not found: 629277")
			})
			defer srv.Close()

			_, err := c.IsChangePureRevert(ctx, "629277")
			assert.Loosely(t, err, should.NotBeNil)
		})
		t.Run("Not revert", func(t *ftt.Test) {
			srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintf(w, "No ID was provided and change isn't a revert")
			})
			defer srv.Close()

			r, err := c.IsChangePureRevert(ctx, "629277")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeFalse)
		})
		t.Run("Not pure revert", func(t *ftt.Test) {
			srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, ")]}'\n%s\n", "{\"is_pure_revert\":false}")
			})
			defer srv.Close()

			r, err := c.IsChangePureRevert(ctx, "629277")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeFalse)
		})
		t.Run("Pure revert", func(t *ftt.Test) {
			srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, ")]}'\n%s\n", "{\"is_pure_revert\":true}")
			})
			defer srv.Close()

			r, err := c.IsChangePureRevert(ctx, "629277")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeTrue)
		})
	})
}

func TestDirectSetReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("SetReview", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()

			var ri ReviewInput
			err := json.NewDecoder(r.Body).Decode(&ri)
			assert.Loosely(c, err, should.BeNil)

			var rr ReviewResult
			rr.Labels = ri.Labels
			rr.Reviewers = make(map[string]AddReviewerResult, len(ri.Reviewers))
			for _, reviewer := range ri.Reviewers {
				result := AddReviewerResult{
					Input: reviewer.Reviewer,
				}
				info := ReviewerInfo{AccountInfo: AccountInfo{AccountID: 12345}}
				switch reviewer.State {
				case "REVIEWER":
					result.Reviewers = []ReviewerInfo{info}
				case "CC":
					result.CCs = []ReviewerInfo{info}
				}
				rr.Reviewers[reviewer.Reviewer] = result
			}

			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")

			var buffer bytes.Buffer
			err = json.NewEncoder(&buffer).Encode(&rr)
			assert.Loosely(c, err, should.BeNil)
			fmt.Fprintf(w, ")]}'\n%s\n", buffer.String())
		})
		defer srv.Close()

		c.Run("Set review", func(c *ftt.Test) {
			_, err := client.SetReview(ctx, "629279", "current", &ReviewInput{})
			assert.Loosely(c, err, should.BeNil)
		})

		c.Run("Set label", func(c *ftt.Test) {
			ri := ReviewInput{Labels: map[string]int{"Code-Review": 1}}
			result, err := client.SetReview(ctx, "629279", "current", &ri)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, result.Labels, should.Resemble(ri.Labels))
		})

		c.Run("Set reviewers", func(c *ftt.Test) {
			ri := ReviewInput{
				Reviewers: []ReviewerInput{
					{
						Reviewer: "test@example.com",
						State:    "REVIEWER",
					},
					{
						Reviewer: "test2@example.com",
						State:    "CC",
					},
				},
			}
			result, err := client.SetReview(ctx, "629279", "current", &ri)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, len(result.Reviewers), should.Equal(2))
			assert.Loosely(c, len(result.Reviewers["test@example.com"].Reviewers), should.Equal(1))
			assert.Loosely(c, len(result.Reviewers["test2@example.com"].CCs), should.Equal(1))
		})
	})

	ftt.Run("SetReview but change non-existent", t, func(t *ftt.Test) {
		srv, c := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "No such change: 629279")
		})
		defer srv.Close()

		t.Run("Basic", func(t *ftt.Test) {
			_, err := c.SetReview(ctx, "629279", "current", &ReviewInput{})
			assert.Loosely(t, err, should.NotBeNil)
		})

	})
}

func TestSubmit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ftt.Run("Submit", t, func(c *ftt.Test) {
		srv, client := newMockClient(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			var si SubmitInput
			err := json.NewDecoder(r.Body).Decode(&si)
			assert.Loosely(c, err, should.BeNil)
			var cr Change
			w.WriteHeader(200)
			w.Header().Set("Content-Type", "application/json")
			var buffer bytes.Buffer
			err = json.NewEncoder(&buffer).Encode(&cr)
			assert.Loosely(c, err, should.BeNil)
			fmt.Fprintf(w, ")]}'\n%s\n", buffer.String())
		})
		defer srv.Close()

		c.Run("Submit", func(c *ftt.Test) {
			_, err := client.Submit(ctx, "629279", &SubmitInput{})
			assert.Loosely(c, err, should.BeNil)
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

var (
	fakeCL1Str = `{
	    "id": "infra%2Fluci%2Fluci-go~master~I4c01b6686740f15844dc86aab73ee4ce00b90fe3",
	    "project": "infra/luci/luci-go",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "I4c01b6686740f15844dc86aab73ee4ce00b90fe3",
	    "subject": "gitiles: Implement forward log.",
	    "status": "NEW",
	    "current_revision": "eb2388b592a9",
	    "created": "2017-08-22 18:46:58.000000000",
	    "updated": "2017-08-23 22:33:34.000000000",
	    "submit_type": "REBASE_ALWAYS",
	    "mergeable": true,
	    "insertions": 154,
	    "deletions": 23,
	    "unresolved_comment_count": 3,
	    "has_review_started": true,
	    "_number": 627036,
	    "owner": {
		"_account_id": 1118104
	    },
	    "reviewers": {
		    "CC": [
			    {"_account_id": 1118110},
			    {"_account_id": 1118111},
			    {"_account_id": 1118112}
		    ],
		    "REVIEWER": [
			    {"_account_id": 1118120},
			    {"_account_id": 1118121},
			    {"_account_id": 1118122}
		    ],
		    "REMOVED": [
			    {"_account_id": 1118130},
			    {"_account_id": 1118131},
			    {"_account_id": 1118132}
		    ]
	    }
	}`
	fakeCL2Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "subject": "[Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "MERGED",
	    "created": "2017-08-23 17:25:40.000000000",
	    "updated": "2017-08-23 22:51:03.000000000",
	    "submitted": "2017-08-23 22:51:03.000000000",
	    "insertions": 4,
	    "deletions": 1,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629277,
	    "owner": {
		"_account_id": 1178184
	    },
	    "_has_more_changes": true
	}`
	fakeCL3Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94a000046da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94a000046da0b08500f738904d15",
	    "subject": "Revert of [Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "MERGED",
	    "current_revision" : "1ee75012c0de",
	    "revert_of": 629277,
	    "created": "2017-08-23 18:25:40.000000000",
	    "updated": "2017-08-23 23:51:03.000000000",
	    "submitted": "2017-08-23 23:51:03.000000000",
	    "insertions": 1,
	    "deletions": 4,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629279,
	    "owner": {
		"_account_id": 1178184
	    }
	}`
	fakeCL4Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "subject": "[Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "ABANDONED",
	    "created": "2017-08-23 17:25:40.000000000",
	    "updated": "2017-08-23 22:51:03.000000000",
	    "submitted": "2017-08-23 22:51:03.000000000",
	    "insertions": 4,
	    "deletions": 1,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629279,
	    "owner": {
		"_account_id": 1178184
	    },
	    "_has_more_changes": true
	}`
	fakeCL5Str = `{
	    "id": "infra%2Finfra~master~Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "project": "infra/infra",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Ia292f77ae6bd94afbd746da0b08500f738904d15",
	    "subject": "[Findit] Add flake analyzer forced rerun instructions to makefile.",
	    "status": "ABANDONED",
	    "created": "2017-08-23 17:25:40.000000000",
	    "updated": "2017-08-23 22:51:03.000000000",
	    "submitted": "2017-08-23 22:51:03.000000000",
	    "insertions": 4,
	    "deletions": 1,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 629279,
	    "owner": {
		"_account_id": 1178184
	    },
	    "labels": {
		 "Verified": {
			 "all": [{
				 "value": 0,
				 "_account_id": 1000096,
				 "name": "John Doe",
				 "email": "john.doe@example.com",
				 "username": "jdoe"
			 },
			 {
				 "value": 0,
				 "_account_id": 1000097,
				 "name": "Jane Roe",
				 "email": "jane.roe@example.com",
				 "username": "jroe"
			 }],
			  "values": {
				  "-1": "Fails",
				  " 0": "No score",
				  "+1": "Verified"
			  }

		 },
		 "Code-Review": {
			 "disliked": {
				 "_account_id": 1000096,
				"name": "John Doe",
				"email": "john.doe@example.com",
				"username": "jdoe"
			 },
			 "all": [{
				"value": -1,
				"_account_id": 1000096,
				"name": "John Doe",
				"email": "john.doe@example.com",
				"username": "jdoe"
			 },
			 {
				"value": 1,
				"_account_id": 1000097,
				"name": "Jane Roe",
				"email": "jane.roe@example.com",
				"username": "jroe"
			}],
			"values": {
				"-2": "This shall not be merged",
				"-1": "I would prefer this is not merged as is",
				" 0": "No score",
				"+1": "Looks good to me, but someone else must approve",
				"+2": "Looks good to me, approved"
			}
		}
	    }
	}`
	fakeCL6Str = `{
	    "id": "infra%2Fluci%2Fluci-go~master~Id37e51c3b84bfc41bc88fa237ddf722f934f4fa4",
	    "project": "infra/luci/luci-go",
	    "branch": "master",
	    "hashtags": [],
	    "change_id": "Id37e51c3b84bfc41bc88fa237ddf722f934f4fa4",
	    "subject": "[vpython]: Re-add deprecated \"-spec\" flag.",
	    "status": "NEW",
	    "current_revision": "d6375c2ea5b0",
	    "created": "2017-08-21 18:46:58.000000000",
	    "updated": "2017-08-22 22:33:34.000000000",
	    "submit_type": "REBASE_ALWAYS",
	    "mergeable": true,
	    "insertions": 6,
	    "deletions": 0,
	    "unresolved_comment_count": 0,
	    "has_review_started": true,
	    "_number": 646267,
	    "owner": {
		"_account_id": 1118104
	    },
	    "reviewers": {
		    "CC": [
			    {"_account_id": 1118110},
			    {"_account_id": 1118111},
			    {"_account_id": 1118112}
		    ],
		    "REVIEWER": [
			    {"_account_id": 1118120},
			    {"_account_id": 1118121},
			    {"_account_id": 1118122}
		    ],
		    "REMOVED": [
			    {"_account_id": 1118130},
			    {"_account_id": 1118131},
			    {"_account_id": 1118132}
		    ]
	    }
	}`
	fakeComments1Str = `{
		"foo": [
	        {
	            "id": "61d1fbfb_63e8c695",
	            "author": {
	                "_account_id": 1002228,
	                "name": "John Doe",
	                "email": "johndoe@example.com"
	            },
	            "change_message_id": "c24215a84fdc9cec42c2d5eec4f488d172d39d7e",
	            "patch_set": 1,
	            "line": 3,
	            "range": {
	                "start_line": 3,
	                "start_character": 7,
	                "end_line": 3,
	                "end_character": 55
	            },
	            "updated": "2020-07-28 14:04:31.000000000",
	            "message": "",
	            "unresolved": true,
	            "in_reply_to": "",
	            "commit_id": "08a8326653eaa5f7aeea30348b63bf5e9595dc11"
	        }
	    ],
	    "bar": [
	        {
	            "id": "63e8c695_61d1fbfb",
	            "author": {
	                "_account_id": 1002228,
	                "name": "John Doe",
	                "email": "johndoe@example.com"
	            },
	            "change_message_id": "c24215a84fdc9cec42c2d5eec4f488d172d39d7e",
	            "patch_set": 1,
	            "line": 21,
	            "updated": "2020-07-28 14:04:31.000000000",
	            "message": "",
	            "unresolved": true,
	            "in_reply_to": "",
	            "commit_id": "08a8326653eaa5f7aeea30348b63bf5e9595dc11"
	        }
	    ]
	}`
	fakeRobotComments1Str = `{
		"foo": [
	        {
	            "id": "61d1fbfb_63e8c695",
	            "author": {
	                "_account_id": 1001234,
	                "name": "A Robot",
	                "email": "robot@example.com"
	            },
	            "change_message_id": "c24215a84fdc9cec42c2d5eec4f488d172d39d7e",
	            "patch_set": 1,
	            "line": 3,
	            "range": {
	                "start_line": 3,
	                "start_character": 7,
	                "end_line": 3,
	                "end_character": 55
	            },
	            "updated": "2020-07-28 14:04:31.000000000",
	            "message": "",
	            "in_reply_to": "",
	            "commit_id": "08a8326653eaa5f7aeea30348b63bf5e9595dc11",
				"robot_id": "somerobot",
				"robot_run_id": "run1"
	        }
	    ],
	    "bar": [
	        {
	            "id": "63e8c695_61d1fbfb",
	            "author": {
	                "_account_id": 1001234,
	                "name": "A Robot",
	                "email": "robot@example.com"
	            },
	            "change_message_id": "c24215a84fdc9cec42c2d5eec4f488d172d39d7e",
	            "patch_set": 1,
	            "line": 21,
	            "updated": "2020-07-28 14:04:31.000000000",
	            "message": "",
	            "in_reply_to": "",
	            "commit_id": "08a8326653eaa5f7aeea30348b63bf5e9595dc11",
				"robot_id": "somerobot",
				"robot_run_id": "run1"
	        }
	    ]
	}`
	fakeAccounts1Str = `[
	    {
	        "_account_id": 1002228,
	        "name": "John Doe",
	        "email": "johndoe@example.com"
	    },
	    {
	        "_account_id": 1002228,
	        "name": "Jane Doe",
	        "email": "janedoe@example.com"
	    }
	]`
)

////////////////////////////////////////////////////////////////////////////////

func newMockClient(handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *Client) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	pu, _ := url.Parse(srv.URL)
	// Tests shouldn't sleep, so make sure we don't wait between request
	// attempts.
	retryStrategy := func() retry.Iterator {
		return &retry.Limited{Retries: 10, Delay: 0}
	}
	c := &Client{http.DefaultClient, *pu, retryStrategy}
	return srv, c
}
