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

package search

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/strpair"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNewSearchQuery(t *testing.T) {
	t.Parallel()

	Convey("NewQuery", t, func() {
		Convey("valid input", func() {
			gerritChanges := make([]*pb.GerritChange, 2)
			gerritChanges[0] = &pb.GerritChange{
				Host:     "a",
				Project:  "b",
				Change:   1,
				Patchset: 1,
			}
			gerritChanges[1] = &pb.GerritChange{
				Host:     "a",
				Project:  "b",
				Change:   2,
				Patchset: 1,
			}
			tags := []*pb.StringPair{
				{Key: "k1", Value: "v1"},
			}
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					Builder: &pb.BuilderID{
						Project: "infra",
						Bucket:  "ci",
						Builder: "test",
					},
					Status:        pb.Status_ENDED_MASK,
					GerritChanges: gerritChanges,
					CreatedBy:     string(identity.AnonymousIdentity),
					Tags:          tags,
					CreateTime: &pb.TimeRange{
						StartTime: &timestamp.Timestamp{Seconds: int64(1592701200)},
						EndTime:   &timestamp.Timestamp{Seconds: int64(1592704800)},
					},
					Build: &pb.BuildRange{
						StartBuildId: int64(200),
						EndBuildId:   int64(100),
					},
					Canary: pb.Trinary_YES,
				},
			}
			query, err := NewQuery(req)

			expectedStartTime := time.Unix(1592701200, 0).UTC()
			expectedEndTime := time.Unix(1592704800, 0).UTC()
			expectedTags := strpair.Map{
				"k1": []string{"v1"},
				"buildset": []string{"patch/gerrit/a/1/1", "patch/gerrit/a/2/1"},
			}
			expected := &Query{
				Project:             "infra",
				BucketId:            "ci",
				Builder:             "test",
				Tags:                expectedTags,
				Status:              pb.Status_ENDED_MASK,
				CreatedBy:           identity.AnonymousIdentity,
				StartTime:           expectedStartTime,
				EndTime:             expectedEndTime,
				IncludeExperimental: false,
				BuildIdHigh:         proto.Int64(201),
				BuildIdLow:          proto.Int64(99),
				Canary:              proto.Bool(true),
				PageSize:            100,
				StartCursor:         "",
			}

			So(err, ShouldBeNil)
			So(query, ShouldResemble, expected)
		})

		Convey("empty req", func() {
			query, err := NewQuery(&pb.SearchBuildsRequest{})

			So(err, ShouldBeNil)
			So(query, ShouldResemble, &Query{})
		})

		Convey("empty predict", func() {
			req := &pb.SearchBuildsRequest{
				PageToken: "aa",
				PageSize:  2,
			}
			query, err := NewQuery(req)

			So(err, ShouldBeNil)
			So(query, ShouldResemble, &Query{
				PageSize:    2,
				StartCursor: "aa",
			})
		})

		Convey("invalid identity", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy:     "user:abc",
				},
			}
			query, err := NewQuery(req)

			So(query, ShouldBeNil)
			So(err, ShouldErrLike, `CreateBy: auth: bad value "abc" for identity kind "user"`)
		})

		Convey("invalid create time", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreatedBy:     string(identity.AnonymousIdentity),
					CreateTime: &pb.TimeRange{
						StartTime: &timestamp.Timestamp{Seconds: int64(253402300801)},
					},
				},
			}
			query, err := NewQuery(req)
			So(query, ShouldBeNil)
			So(err, ShouldErrLike, "CreateTime.StartTime: timestamp: seconds:253402300801 after 10000-01-01")
		})
	})
}

func TestFixPageSize(t *testing.T) {
	t.Parallel()

	Convey("normal page size", t, func() {
		So(fixPageSize(200), ShouldEqual, 200)
	})

	Convey("default page size", t, func() {
		So(fixPageSize(0), ShouldEqual, 100)
	})

	Convey("max page size", t, func() {
		So(fixPageSize(1500), ShouldEqual, 1000)
	})
}

func TestToTime(t *testing.T) {
	t.Parallel()
	Convey("normal timestamp", t, func() {
		res, err := toTime(&timestamp.Timestamp{Seconds:1592701200})
		So(err, ShouldBeNil)
		So(res, ShouldEqual,time.Unix(1592701200, 0).UTC())
	})
	Convey("invalid timestamp", t, func() {
		_, err := toTime(&timestamp.Timestamp{Seconds:253402300801})
		So(err, ShouldErrLike, "timestamp: seconds:253402300801 after 10000-01-01")
	})
	Convey("nil timestamp", t, func() {
		res, err := toTime(nil)
		So(err, ShouldBeNil)
		So(res.IsZero(), ShouldBeTrue)
	})
}