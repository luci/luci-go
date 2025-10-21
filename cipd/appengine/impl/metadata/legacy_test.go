// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

func TestLegacyMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("With legacy entities", t, func(t *ftt.Test) {
		impl := legacyStorageImpl{}

		ctx := memory.Use(context.Background())
		ts := time.Unix(1525136124, 0).UTC()

		root := rootKey(ctx)
		assert.Loosely(t, datastore.Put(ctx, []*packageACL{
			// ACLs for "a".
			{
				ID:         "OWNER:a",
				Parent:     root,
				Users:      []string{"user:a-owner@example.com"},
				Groups:     []string{"a-owner"},
				ModifiedBy: "user:a-owner-mod@example.com",
				ModifiedTS: ts,
			},
			{
				ID:         "WRITER:a",
				Parent:     root,
				Users:      []string{"user:a-writer@example.com"},
				Groups:     []string{"a-writer"},
				ModifiedBy: "user:a-writer-mod@example.com",
				ModifiedTS: ts.Add(5 * time.Second),
			},
			{
				ID:         "READER:a",
				Parent:     root,
				Users:      []string{"user:a-reader@example.com"},
				Groups:     []string{"a-reader"},
				ModifiedBy: "user:a-reader-mod@example.com",
				ModifiedTS: ts,
			},

			// Empty ACLs for "a/b".
			{
				ID:         "OWNER:a/b",
				Parent:     root,
				ModifiedBy: "user:b-owner-mod@example.com",
				ModifiedTS: ts,
				// no Users or Groups here
			},

			// ACLs for "a/b/c/d".
			{
				ID:         "OWNER:a/b/c/d",
				Parent:     root,
				Users:      []string{"user:d-owner@example.com", "bad:ident"},
				Groups:     []string{"d-owner"},
				ModifiedBy: "user:d-owner-mod@example.com",
				ModifiedTS: ts,
			},
		}), should.BeNil)

		rootMeta := rootMetadata()

		// Expected metadata per prefix.
		expected := map[string]*repopb.PrefixMetadata{
			"a": {
				Prefix:      "a",
				Fingerprint: "BK-o5e-PimWmXtF3zdzvjiyAqSU",
				UpdateTime:  timestamppb.New(ts.Add(5 * time.Second)), // WRITER:a mod time
				UpdateUser:  "user:a-writer-mod@example.com",
				Acls: []*repopb.PrefixMetadata_ACL{
					{Role: repopb.Role_OWNER, Principals: []string{"user:a-owner@example.com", "group:a-owner"}},
					{Role: repopb.Role_WRITER, Principals: []string{"user:a-writer@example.com", "group:a-writer"}},
					{Role: repopb.Role_READER, Principals: []string{"user:a-reader@example.com", "group:a-reader"}},
				},
			},
			"a/b": {
				Prefix:      "a/b",
				Fingerprint: "RyIXeT0HBpfv5Lj8FLqMzCu60ZI",
				UpdateTime:  timestamppb.New(ts),
				UpdateUser:  "user:b-owner-mod@example.com",
			},
			"a/b/c/d": {
				Prefix:      "a/b/c/d",
				Fingerprint: "4B97z37yN22RnBHS336ROctEC2w",
				UpdateTime:  timestamppb.New(ts),
				UpdateUser:  "user:d-owner-mod@example.com",
				Acls: []*repopb.PrefixMetadata_ACL{
					// Note: bad:ident is skipped here.
					{Role: repopb.Role_OWNER, Principals: []string{"user:d-owner@example.com", "group:d-owner"}},
				},
			},
		}

		t.Run("GetMetadata returns root metadata which has fingerprint", func(t *ftt.Test) {
			md, err := impl.GetMetadata(ctx, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta}))
			assert.Loosely(t, rootMeta, should.Resemble(&repopb.PrefixMetadata{
				Acls: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"group:administrators"},
					},
				},
				Fingerprint: "G7Hov8WrEwWHx1dQd7SMsKJERUI",
			}))
		})

		t.Run("GetMetadata handles one prefix", func(t *ftt.Test) {
			md, err := impl.GetMetadata(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{
				rootMeta,
				expected["a"],
			}))
		})

		t.Run("GetMetadata handles many prefixes", func(t *ftt.Test) {
			// Returns only existing metadata, silently skipping undefined.
			md, err := impl.GetMetadata(ctx, "a/b/c/d/e/")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{
				rootMeta,
				expected["a"],
				expected["a/b"],
				expected["a/b/c/d"],
			}))
		})

		t.Run("GetMetadata handles root metadata", func(t *ftt.Test) {
			md, err := impl.GetMetadata(ctx, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta}))
		})

		t.Run("GetMetadata fails on bad prefix", func(t *ftt.Test) {
			_, err := impl.GetMetadata(ctx, "???")
			assert.Loosely(t, err, should.ErrLike("invalid package prefix"))
		})

		t.Run("UpdateMetadata noop call with existing metadata", func(t *ftt.Test) {
			updated, err := impl.UpdateMetadata(ctx, "a", func(_ context.Context, md *repopb.PrefixMetadata) error {
				assert.Loosely(t, md, should.Resemble(expected["a"]))
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updated, should.Resemble(expected["a"]))
		})

		t.Run("UpdateMetadata refuses to update root metadata", func(t *ftt.Test) {
			_, err := impl.UpdateMetadata(ctx, "", func(_ context.Context, md *repopb.PrefixMetadata) error {
				panic("must not be called")
			})
			assert.Loosely(t, err, should.ErrLike("the root metadata is not modifiable"))
		})

		t.Run("UpdateMetadata updates existing metadata", func(t *ftt.Test) {
			modTime := ts.Add(10 * time.Second)

			newMD := proto.Clone(expected["a"]).(*repopb.PrefixMetadata)
			newMD.UpdateTime = timestamppb.New(modTime)
			newMD.UpdateUser = "user:updater@example.com"
			newMD.Acls[0].Principals = []string{
				"group:new-owning-group",
				"user:new-owner@example.com",
				"group:another-group",
			}

			updated, err := impl.UpdateMetadata(ctx, "a", func(_ context.Context, md *repopb.PrefixMetadata) error {
				assert.Loosely(t, md, should.Resemble(expected["a"]))
				proto.Merge(md, newMD)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			// The returned metadata is different from newMD: order of principals is
			// not preserved, and the fingerprint is populated.
			newMD.Acls[0].Principals = []string{
				"user:new-owner@example.com",
				"group:new-owning-group",
				"group:another-group",
			}
			newMD.Fingerprint = "MCRIAGe9tfXGxAZ-mTQbjQiJAlA" // new FP
			assert.Loosely(t, updated, should.Resemble(newMD))

			// GetMetadata sees the new metadata.
			md, err := impl.GetMetadata(ctx, "a")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta, newMD}))

			// Only touched "OWNER:..." legacy entity, since only owners changed.
			legacy := prefixACLs(ctx, "a", nil)
			assert.Loosely(t, datastore.Get(ctx, legacy), should.BeNil)
			assert.Loosely(t, legacy, should.Resemble([]*packageACL{
				{
					ID:         "OWNER:a",
					Parent:     root,
					Users:      []string{"user:new-owner@example.com"},
					Groups:     []string{"new-owning-group", "another-group"},
					ModifiedBy: "user:updater@example.com",
					ModifiedTS: modTime,
					Rev:        1,
				},
				// Untouched.
				{
					ID:         "WRITER:a",
					Parent:     root,
					Users:      []string{"user:a-writer@example.com"},
					Groups:     []string{"a-writer"},
					ModifiedBy: "user:a-writer-mod@example.com",
					ModifiedTS: ts.Add(5 * time.Second),
				},
				// Untouched.
				{
					ID:         "READER:a",
					Parent:     root,
					Users:      []string{"user:a-reader@example.com"},
					Groups:     []string{"a-reader"},
					ModifiedBy: "user:a-reader-mod@example.com",
					ModifiedTS: ts,
				},
			}))
		})

		t.Run("UpdateMetadata noop call with missing metadata", func(t *ftt.Test) {
			updated, err := impl.UpdateMetadata(ctx, "z", func(_ context.Context, md *repopb.PrefixMetadata) error {
				assert.Loosely(t, md, should.Resemble(&repopb.PrefixMetadata{Prefix: "z"}))
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updated, should.BeNil)

			// Still missing.
			md, err := impl.GetMetadata(ctx, "z")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta}))
		})

		t.Run("UpdateMetadata creates new metadata", func(t *ftt.Test) {
			updated, err := impl.UpdateMetadata(ctx, "z", func(_ context.Context, md *repopb.PrefixMetadata) error {
				assert.Loosely(t, md, should.Resemble(&repopb.PrefixMetadata{Prefix: "z"}))
				md.UpdateTime = timestamppb.New(ts)
				md.UpdateUser = "user:updater@example.com"
				md.Acls = []*repopb.PrefixMetadata_ACL{
					{
						Role: repopb.Role_READER,
					},
					{
						Role:       repopb.Role_WRITER,
						Principals: []string{"group:a", "user:a@example.com"},
					},
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"group:b"},
					},
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			// Changes compared to what was stored in the callback:
			//  * Acls are ordered by Role now.
			//  * READER is missing, the principals list was empty.
			//  * Principals are sorted by "users first, then groups".
			expected := &repopb.PrefixMetadata{
				Prefix:      "z",
				Fingerprint: "ppDqWKGcl8Pu1hMiXQ1hac0vAH0",
				UpdateTime:  timestamppb.New(ts),
				UpdateUser:  "user:updater@example.com",
				Acls: []*repopb.PrefixMetadata_ACL{
					{
						Role:       repopb.Role_OWNER,
						Principals: []string{"group:b"},
					},
					{
						Role:       repopb.Role_WRITER,
						Principals: []string{"user:a@example.com", "group:a"},
					},
				},
			}
			assert.Loosely(t, updated, should.Resemble(expected))

			// Stored indeed.
			md, err := impl.GetMetadata(ctx, "z")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta, expected}))
		})

		t.Run("UpdateMetadata call with failing callback", func(t *ftt.Test) {
			cbErr := errors.New("blah")
			updated, err := impl.UpdateMetadata(ctx, "z", func(_ context.Context, md *repopb.PrefixMetadata) error {
				md.UpdateUser = "user:must-be-ignored@example.com"
				return cbErr
			})
			assert.Loosely(t, err, should.Equal(cbErr)) // exact same error object
			assert.Loosely(t, updated, should.BeNil)

			// Still missing.
			md, err := impl.GetMetadata(ctx, "z")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, md, should.Resemble([]*repopb.PrefixMetadata{rootMeta}))
		})
	})
}

func TestVisitMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ts := time.Unix(1525136124, 0).UTC()

		impl := legacyStorageImpl{}

		add := func(role, pfx, group string) {
			assert.Loosely(t, datastore.Put(ctx, &packageACL{
				ID:         role + ":" + pfx,
				Parent:     rootKey(ctx),
				ModifiedTS: ts,
				Groups:     []string{group},
			}), should.BeNil)
		}

		type visited struct {
			prefix string
			md     []string // pfx:role:principal, sorted
		}

		visit := func(pfx string) (res []visited) {
			err := impl.VisitMetadata(ctx, pfx, func(p string, md []*repopb.PrefixMetadata) (bool, error) {
				extract := []string{}
				for _, m := range md {
					for _, acl := range m.Acls {
						for _, p := range acl.Principals {
							extract = append(extract, fmt.Sprintf("%s:%s:%s", m.Prefix, acl.Role, p))
						}
					}
				}
				res = append(res, visited{p, extract})
				return true, nil
			})
			assert.Loosely(t, err, should.BeNil)
			return
		}

		add("OWNER", "a", "o-a")
		add("READER", "a", "r-a")

		add("OWNER", "a/b/c", "o-abc")
		add("READER", "a/b/c", "r-abc")
		add("WRITER", "a/b/c", "w-abc")

		add("READER", "a/b/c/d", "r-abcd")

		add("OWNER", "ab", "o-ab")
		add("READER", "ab", "r-ab")

		t.Run("Root listing", func(t *ftt.Test) {
			assert.Loosely(t, visit(""), should.Resemble([]visited{
				{
					"", []string{
						":OWNER:group:administrators",
					},
				},
				{
					"a", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
					},
				},
				{
					"a/b/c", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
						"a/b/c:OWNER:group:o-abc",
						"a/b/c:WRITER:group:w-abc",
						"a/b/c:READER:group:r-abc",
					},
				},
				{
					"a/b/c/d", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
						"a/b/c:OWNER:group:o-abc",
						"a/b/c:WRITER:group:w-abc",
						"a/b/c:READER:group:r-abc",
						"a/b/c/d:READER:group:r-abcd",
					},
				},
				{
					"ab", []string{
						":OWNER:group:administrators",
						"ab:OWNER:group:o-ab",
						"ab:READER:group:r-ab",
					},
				},
			}))
		})

		t.Run("Prefix listing", func(t *ftt.Test) {
			assert.Loosely(t, visit("a"), should.Resemble([]visited{
				{
					"a", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
					},
				},
				{
					"a/b/c", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
						"a/b/c:OWNER:group:o-abc",
						"a/b/c:WRITER:group:w-abc",
						"a/b/c:READER:group:r-abc",
					},
				},
				{
					"a/b/c/d", []string{
						":OWNER:group:administrators",
						"a:OWNER:group:o-a",
						"a:READER:group:r-a",
						"a/b/c:OWNER:group:o-abc",
						"a/b/c:WRITER:group:w-abc",
						"a/b/c:READER:group:r-abc",
						"a/b/c/d:READER:group:r-abcd",
					},
				},
			}))
		})

		t.Run("Missing prefix listing", func(t *ftt.Test) {
			assert.Loosely(t, visit("z/z/z"), should.Resemble([]visited{
				{
					"z/z/z", []string{":OWNER:group:administrators"},
				},
			}))
		})

		t.Run("Callback return value is respected, stopping right away", func(t *ftt.Test) {
			seen := []string{}
			err := impl.VisitMetadata(ctx, "a", func(p string, md []*repopb.PrefixMetadata) (bool, error) {
				seen = append(seen, p)
				return false, nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen, should.Resemble([]string{"a"}))
		})

		t.Run("Callback return value is respected, stopping later", func(t *ftt.Test) {
			seen := []string{}
			err := impl.VisitMetadata(ctx, "a", func(p string, md []*repopb.PrefixMetadata) (bool, error) {
				seen = append(seen, p)
				return p != "a/b/c", nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, seen, should.Resemble([]string{"a", "a/b/c"})) // no a/b/c/d
		})
	})
}

func TestParseKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		id     string
		role   string
		prefix string
		err    string
	}{
		{"OWNER:a/b/c", "OWNER", "a/b/c", ""},
		{"OWNER", "", "", "not <role>:<prefix> pair"},
		{"UNKNOWN:a/b/c", "", "", "unrecognized role"},
		{"OWNER:///", "", "", "invalid package prefix"},
		{"OWNER:", "", "", "invalid package prefix"},
	}

	for _, c := range cases {
		ftt.Run(fmt.Sprintf("works for %q", c.id), t, func(t *ftt.Test) {
			role, pfx, err := (&packageACL{ID: c.id}).parseKey()
			assert.Loosely(t, role, should.Equal(c.role))
			assert.Loosely(t, pfx, should.Equal(c.prefix))
			if c.err == "" {
				assert.Loosely(t, err, should.BeNil)
			} else {
				assert.Loosely(t, err, should.ErrLike(c.err))
			}
		})
	}
}

func TestListACLsByPrefix(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		add := func(role, pfx string) {
			assert.Loosely(t, datastore.Put(ctx, &packageACL{
				ID:     role + ":" + pfx,
				Parent: rootKey(ctx),
				Groups: []string{"blah"}, //  to make sure bodies are fetched too
			}), should.BeNil)
		}

		list := func(role, pfx string) (out []string) {
			acls, err := listACLsByPrefix(ctx, role, pfx)
			assert.Loosely(t, err, should.BeNil)
			for _, acl := range acls {
				assert.Loosely(t, acl.Groups, should.Resemble([]string{"blah"}))
				out = append(out, acl.ID)
			}
			return
		}

		add("OWNER", "a")
		add("OWNER", "a/b/c")
		add("OWNER", "ab")
		add("READER", "a")
		add("READER", "a/b/c")
		add("READER", "a/b/c/d")
		add("READER", "ab")

		t.Run("Root listing", func(t *ftt.Test) {
			assert.Loosely(t, list("OWNER", ""), should.Resemble([]string{
				"OWNER:a", "OWNER:a/b/c", "OWNER:ab",
			}))
			assert.Loosely(t, list("READER", ""), should.Resemble([]string{
				"READER:a", "READER:a/b/c", "READER:a/b/c/d", "READER:ab",
			}))
			assert.Loosely(t, list("WRITER", ""), should.Resemble([]string(nil)))
		})

		t.Run("Non-root listing", func(t *ftt.Test) {
			assert.Loosely(t, list("OWNER", "a"), should.Resemble([]string{"OWNER:a/b/c"}))
			assert.Loosely(t, list("READER", "a"), should.Resemble([]string{"READER:a/b/c", "READER:a/b/c/d"}))
			assert.Loosely(t, list("WRITER", "a"), should.Resemble([]string(nil)))
		})

		t.Run("Non-existing prefix listing", func(t *ftt.Test) {
			assert.Loosely(t, list("OWNER", "z"), should.Resemble([]string(nil)))
		})
	})
}

func TestMetadataGraph(t *testing.T) {
	t.Parallel()

	ftt.Run("With metadataGraph", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ts := time.Unix(1525136124, 0).UTC()

		gr := metadataGraph{}
		gr.init(&repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_OWNER,
					Principals: []string{"group:root"},
				},
			},
		})

		insert := func(role, prefix, group string) {
			gr.insert(ctx, []*packageACL{
				{
					ID:         role + ":" + prefix,
					Parent:     rootKey(ctx),
					Groups:     []string{group},
					ModifiedTS: ts, // to mark as non-empty
				},
			})
		}

		type visited struct {
			prefix string
			md     []string // pfx:role:principal, sorted
		}

		freezeAndVisit := func(node string) (v []visited) {
			n := gr.node(node)
			gr.freeze(ctx)

			err := n.traverse(nil, func(n *metadataNode, md []*repopb.PrefixMetadata) (bool, error) {
				extract := []string{}
				for _, m := range md {
					for _, acl := range m.Acls {
						for _, p := range acl.Principals {
							extract = append(extract, fmt.Sprintf("%s:%s:%s", m.Prefix, acl.Role, p))
						}
					}
				}
				v = append(v, visited{n.prefix, extract})
				return true, nil
			})
			assert.Loosely(t, err, should.BeNil)
			return
		}

		insert("OWNER", "a/b/c/d", "owner-abc")
		insert("OWNER", "a", "owner-a")
		insert("OWNER", "b", "owner-b")
		insert("READER", "a", "reader-a")
		insert("READER", "a/b", "reader-ab")
		insert("READER", "a/bc", "reader-abc")
		insert("BOGUS", "a/b", "bogus-ab")

		t.Run("Traverse from the root", func(t *ftt.Test) {
			assert.Loosely(t, freezeAndVisit(""), should.Resemble([]visited{
				{"", []string{":OWNER:group:root"}},
				{
					"a", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
					},
				},
				{
					"a/b", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
					},
				},
				{
					"a/b/c", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
					},
				},
				{
					"a/b/c/d", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
						"a/b/c/d:OWNER:group:owner-abc",
					},
				},
				{
					"a/bc", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/bc:READER:group:reader-abc",
					},
				},
				{
					"b", []string{
						":OWNER:group:root",
						"b:OWNER:group:owner-b",
					},
				},
			}))
		})

		t.Run("Traverse from some prefix", func(t *ftt.Test) {
			assert.Loosely(t, freezeAndVisit("a/b"), should.Resemble([]visited{
				{
					"a/b", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
					},
				},
				{
					"a/b/c", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
					},
				},
				{
					"a/b/c/d", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
						"a/b/c/d:OWNER:group:owner-abc",
					},
				},
			}))
		})

		t.Run("Traverse from some deep prefix", func(t *ftt.Test) {
			assert.Loosely(t, freezeAndVisit("a/b/c/d/e"), should.Resemble([]visited{
				{
					"a/b/c/d/e", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
						"a/b/c/d:OWNER:group:owner-abc",
					},
				},
			}))
		})

		t.Run("Traverse from some non-existing prefix", func(t *ftt.Test) {
			assert.Loosely(t, freezeAndVisit("z/z/z"), should.Resemble([]visited{
				{
					"z/z/z", []string{":OWNER:group:root"},
				},
			}))
		})
	})
}
