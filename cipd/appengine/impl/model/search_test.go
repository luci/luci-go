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

package model

import (
	"strings"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()

		iid := func(i int) string {
			ch := string([]byte{'0' + byte(i)})
			return strings.Repeat(ch, 40)
		}

		ids := func(inst []*Instance) []string {
			out := make([]string, len(inst))
			for i, ent := range inst {
				out[i] = ent.InstanceID
			}
			return out
		}

		put := func(when int, iid string, tags ...string) {
			inst := &Instance{
				InstanceID:   iid,
				Package:      PackageKey(ctx, "pkg"),
				RegisteredTs: testTime.Add(time.Duration(when) * time.Second),
			}
			ents := make([]*Tag, len(tags))
			for i, t := range tags {
				ents[i] = &Tag{
					ID:           TagID(common.MustParseInstanceTag(t)),
					Instance:     datastore.KeyForObj(ctx, inst),
					Tag:          t,
					RegisteredTs: testTime.Add(time.Duration(when) * time.Second),
				}
			}
			So(datastore.Put(ctx, inst, ents), ShouldBeNil)
		}

		Convey("Search by one tag works", func() {
			expectedIIDs := make([]string, 10)
			for i := 0; i < 10; i++ {
				put(i, iid(i), "k:v0")
				expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
			}

			Convey("Empty", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "missing"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(out, ShouldHaveLength, 0)
			})

			Convey("No pagination", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, expectedIIDs)
			})

			Convey("With pagination", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 6, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldNotBeNil)
				So(ids(out), ShouldResemble, expectedIIDs[:6])

				out, cur, err = SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 6, cur)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, expectedIIDs[6:10])
			})

			Convey("Handle missing instances", func() {
				datastore.Delete(ctx, &Instance{
					InstanceID: iid(5),
					Package:    PackageKey(ctx, "pkg"),
				})

				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, append(append([]string(nil),
					expectedIIDs[:4]...),
					expectedIIDs[5:]...))
			})
		})

		Convey("Search by many tags works", func() {
			expectedIIDs := make([]string, 10)
			for i := 0; i < 10; i++ {
				tags := []string{"k:v0"}
				if i%2 == 0 {
					tags = append(tags, "k:v1")
				}
				if i%3 == 0 {
					tags = append(tags, "k:v2")
				}
				put(i, iid(i), tags...)
				expectedIIDs[9-i] = iid(i) // sorted by creation time, most recent first
			}

			Convey("Empty result", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "missing"},
					{Key: "k", Value: "v2"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(out, ShouldHaveLength, 0)
			})

			Convey("No pagination", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v1"}, // dup
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, []string{
					iid(8), iid(6), iid(4), iid(2), iid(0),
				})
			})

			Convey("With pagination", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
				}, 3, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldNotBeNil)
				So(ids(out), ShouldResemble, []string{
					iid(8), iid(6), iid(4),
				})

				out, cur, err = SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
				}, 3, cur)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, []string{
					iid(2), iid(0),
				})
			})

			Convey("More filters", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v0"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v2"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, []string{
					iid(6), iid(0),
				})
			})

			Convey("Tags order doesn't matter", func() {
				out, cur, err := SearchInstances(ctx, "pkg", []*api.Tag{
					{Key: "k", Value: "v2"},
					{Key: "k", Value: "v1"},
					{Key: "k", Value: "v0"},
				}, 11, nil)
				So(err, ShouldBeNil)
				So(cur, ShouldBeNil)
				So(ids(out), ShouldResemble, []string{
					iid(6), iid(0),
				})
			})
		})
	})
}
