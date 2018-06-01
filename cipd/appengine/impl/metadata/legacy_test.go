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

		Convey("GetMetadata handles one prefix", func() {
			md, err := impl.GetMetadata(ctx, "a")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{expected["a"]})
		})

		Convey("GetMetadata handles many prefixes", func() {
			// Returns only existing metadata, silently skipping undefined.
			md, err := impl.GetMetadata(ctx, "a/b/c/d/e/")
			So(err, ShouldBeNil)
			So(md, ShouldResembleProto, []*api.PrefixMetadata{
				expected["a"],
				expected["a/b"],
				expected["a/b/c/d"],
			})
		})

		Convey("GetMetadata fails on bad prefix", func() {
			_, err := impl.GetMetadata(ctx, "")
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
			So(md, ShouldResembleProto, []*api.PrefixMetadata{newMD})

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
			So(md, ShouldHaveLength, 0)
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
			So(md, ShouldResembleProto, []*api.PrefixMetadata{expected})
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
			So(md, ShouldHaveLength, 0)
		})
	})
}
