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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/proto/google"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLegacyMetadata(t *testing.T) {
	t.Parallel()

	Convey("With legacy entities", t, func() {
		impl := legacyStorageImpl{}

		ctx := gaetesting.TestingContext()
		ts := time.Unix(1525136124, 0).UTC()

		root := rootKey(ctx)
		So(datastore.Put(ctx, []*packageACL{
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
		}), ShouldBeNil)

		rootMeta := rootMetadata()

		// Expected metadata per prefix.
		expected := map[string]*api.PrefixMetadata{
			"a": {
				Prefix:      "a",
				Fingerprint: "BK-o5e-PimWmXtF3zdzvjiyAqSU",
				UpdateTime:  google.NewTimestamp(ts.Add(5 * time.Second)), // WRITER:a mod time
				UpdateUser:  "user:a-writer-mod@example.com",
				Acls: []*api.PrefixMetadata_ACL{
					{Role: api.Role_OWNER, Principals: []string{"user:a-owner@example.com", "group:a-owner"}},
					{Role: api.Role_WRITER, Principals: []string{"user:a-writer@example.com", "group:a-writer"}},
					{Role: api.Role_READER, Principals: []string{"user:a-reader@example.com", "group:a-reader"}},
				},
			},
			"a/b": {
				Prefix:      "a/b",
				Fingerprint: "RyIXeT0HBpfv5Lj8FLqMzCu60ZI",
				UpdateTime:  google.NewTimestamp(ts),
				UpdateUser:  "user:b-owner-mod@example.com",
			},
			"a/b/c/d": {
				Prefix:      "a/b/c/d",
				Fingerprint: "4B97z37yN22RnBHS336ROctEC2w",
				UpdateTime:  google.NewTimestamp(ts),
				UpdateUser:  "user:d-owner-mod@example.com",
				Acls: []*api.PrefixMetadata_ACL{
					// Note: bad:ident is skipped here.
					{Role: api.Role_OWNER, Principals: []string{"user:d-owner@example.com", "group:d-owner"}},
				},
			},
		}

		Convey("GetMetadata returns root metadata which has fingerprint", func() {
			md, err := impl.GetMetadata(ctx, "")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta})
			So(rootMeta, ShouldResembleProto, &api.PrefixMetadata{
				Acls: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_OWNER,
						Principals: []string{"group:administrators"},
					},
				},
				Fingerprint: "G7Hov8WrEwWHx1dQd7SMsKJERUI",
			})
		})

		Convey("GetMetadata handles one prefix", func() {
			md, err := impl.GetMetadata(ctx, "a")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{
				rootMeta,
				expected["a"],
			})
		})

		Convey("GetMetadata handles many prefixes", func() {
			// Returns only existing metadata, silently skipping undefined.
			md, err := impl.GetMetadata(ctx, "a/b/c/d/e/")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{
				rootMeta,
				expected["a"],
				expected["a/b"],
				expected["a/b/c/d"],
			})
		})

		Convey("GetMetadata handles root metadata", func() {
			md, err := impl.GetMetadata(ctx, "")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta})
		})

		Convey("GetMetadata fails on bad prefix", func() {
			_, err := impl.GetMetadata(ctx, "???")
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("UpdateMetadata noop call with existing metadata", func() {
			updated, err := impl.UpdateMetadata(ctx, "a", func(md *api.PrefixMetadata) error {
				So(md, ShouldResembleProto, expected["a"])
				return nil
			})
			So(err, ShouldBeNil)
			So(updated, ShouldResembleProto, expected["a"])
		})

		Convey("UpdateMetadata refuses to update root metadata", func() {
			_, err := impl.UpdateMetadata(ctx, "", func(md *api.PrefixMetadata) error {
				panic("must not be called")
			})
			So(err, ShouldErrLike, "the root metadata is not modifiable")
		})

		Convey("UpdateMetadata updates existing metadata", func() {
			modTime := ts.Add(10 * time.Second)

			newMD := proto.Clone(expected["a"]).(*api.PrefixMetadata)
			newMD.UpdateTime = google.NewTimestamp(modTime)
			newMD.UpdateUser = "user:updater@example.com"
			newMD.Acls[0].Principals = []string{
				"group:new-owning-group",
				"user:new-owner@example.com",
				"group:another-group",
			}

			updated, err := impl.UpdateMetadata(ctx, "a", func(md *api.PrefixMetadata) error {
				So(md, ShouldResembleProto, expected["a"])
				*md = *newMD
				return nil
			})
			So(err, ShouldBeNil)

			// The returned metadata is different from newMD: order of principals is
			// not preserved, and the fingerprint is populated.
			newMD.Acls[0].Principals = []string{
				"user:new-owner@example.com",
				"group:new-owning-group",
				"group:another-group",
			}
			newMD.Fingerprint = "MCRIAGe9tfXGxAZ-mTQbjQiJAlA" // new FP
			So(updated, ShouldResembleProto, newMD)

			// GetMetadata sees the new metadata.
			md, err := impl.GetMetadata(ctx, "a")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, newMD})

			// Only touched "OWNER:..." legacy entity, since only owners changed.
			legacy := prefixACLs(ctx, "a", nil)
			So(datastore.Get(ctx, legacy), ShouldBeNil)
			So(legacy, ShouldResemble, []*packageACL{
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
			})
		})

		Convey("UpdateMetadata noop call with missing metadata", func() {
			updated, err := impl.UpdateMetadata(ctx, "z", func(md *api.PrefixMetadata) error {
				So(md, ShouldResemble, &api.PrefixMetadata{Prefix: "z"})
				return nil
			})
			So(err, ShouldBeNil)
			So(updated, ShouldBeNil)

			// Still missing.
			md, err := impl.GetMetadata(ctx, "z")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta})
		})

		Convey("UpdateMetadata creates new metadata", func() {
			updated, err := impl.UpdateMetadata(ctx, "z", func(md *api.PrefixMetadata) error {
				So(md, ShouldResemble, &api.PrefixMetadata{Prefix: "z"})
				md.UpdateTime = google.NewTimestamp(ts)
				md.UpdateUser = "user:updater@example.com"
				md.Acls = []*api.PrefixMetadata_ACL{
					{
						Role: api.Role_READER,
					},
					{
						Role:       api.Role_WRITER,
						Principals: []string{"group:a", "user:a@example.com"},
					},
					{
						Role:       api.Role_OWNER,
						Principals: []string{"group:b"},
					},
				}
				return nil
			})
			So(err, ShouldBeNil)

			// Changes compared to what was stored in the callback:
			//  * Acls are ordered by Role now.
			//  * READER is missing, the principals list was empty.
			//  * Principals are sorted by "users first, then groups".
			expected := &api.PrefixMetadata{
				Prefix:      "z",
				Fingerprint: "ppDqWKGcl8Pu1hMiXQ1hac0vAH0",
				UpdateTime:  google.NewTimestamp(ts),
				UpdateUser:  "user:updater@example.com",
				Acls: []*api.PrefixMetadata_ACL{
					{
						Role:       api.Role_OWNER,
						Principals: []string{"group:b"},
					},
					{
						Role:       api.Role_WRITER,
						Principals: []string{"user:a@example.com", "group:a"},
					},
				},
			}
			So(updated, ShouldResembleProto, expected)

			// Stored indeed.
			md, err := impl.GetMetadata(ctx, "z")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta, expected})
		})

		Convey("UpdateMetadata call with failing callback", func() {
			cbErr := errors.New("blah")
			updated, err := impl.UpdateMetadata(ctx, "z", func(md *api.PrefixMetadata) error {
				md.UpdateUser = "user:must-be-ignored@example.com"
				return cbErr
			})
			So(err, ShouldEqual, cbErr) // exact same error object
			So(updated, ShouldBeNil)

			// Still missing.
			md, err := impl.GetMetadata(ctx, "z")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{rootMeta})
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
		Convey(fmt.Sprintf("works for %q", c.id), t, func() {
			role, pfx, err := (&packageACL{ID: c.id}).parseKey()
			So(role, ShouldEqual, c.role)
			So(pfx, ShouldEqual, c.prefix)
			if c.err == "" {
				So(err, ShouldBeNil)
			} else {
				So(err, ShouldErrLike, c.err)
			}
		})
	}
}

func TestMetadataGraph(t *testing.T) {
	t.Parallel()

	Convey("With metadataGraph", t, func() {
		ctx := gaetesting.TestingContext()
		ts := time.Unix(1525136124, 0).UTC()

		gr := metadataGraph{}
		gr.init(&api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_OWNER,
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

			err := n.traverse(nil, func(n *metadataNode, md []*api.PrefixMetadata) (bool, error) {
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
			So(err, ShouldBeNil)
			return
		}

		insert("OWNER", "a/b/c/d", "owner-abc")
		insert("OWNER", "a", "owner-a")
		insert("OWNER", "b", "owner-b")
		insert("READER", "a", "reader-a")
		insert("READER", "a/b", "reader-ab")
		insert("READER", "a/bc", "reader-abc")
		insert("BOGUS", "a/b", "bogus-ab")

		Convey("Traverse from the root", func() {
			So(freezeAndVisit(""), ShouldResemble, []visited{
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
			})
		})

		Convey("Traverse from some prefix", func() {
			So(freezeAndVisit("a/b"), ShouldResemble, []visited{
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
			})
		})

		Convey("Traverse from some deep prefix", func() {
			So(freezeAndVisit("a/b/c/d/e"), ShouldResemble, []visited{
				{
					"a/b/c/d/e", []string{
						":OWNER:group:root",
						"a:OWNER:group:owner-a",
						"a:READER:group:reader-a",
						"a/b:READER:group:reader-ab",
						"a/b/c/d:OWNER:group:owner-abc",
					},
				},
			})
		})

		Convey("Traverse from some non-existing prefix", func() {
			So(freezeAndVisit("z/z/z"), ShouldResemble, []visited{
				{
					"z/z/z", []string{":OWNER:group:root"},
				},
			})
		})
	})
}
