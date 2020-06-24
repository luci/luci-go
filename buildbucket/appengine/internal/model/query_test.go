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

// Package model in internal contains intermediate models
package model

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/buildbucket/appengine/internal/utils"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestNewSearchQuery(t *testing.T) {
	t.Parallel()

	Convey("NewSearchQuery", t, func() {
		Convey("valid input", func() {
			gerritChanges := make([]*pb.GerritChange, 1)
			gerritChanges[0] = &pb.GerritChange{
				Host:     "a",
				Project:  "b",
				Change:   1,
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
					CreatedBy:     "user:abc",
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
			query, err := NewSearchQuery(req)

			expectedStartTime := time.Unix(1592701200, 0).UTC()
			expectedEndTime := time.Unix(1592704800, 0).UTC()
			expected := &SearchQuery{
				Project:             "",
				BucketId:            "infra/ci",
				Builder:             "test",
				Tags:                []string{"k1:v1", "patch/gerrit/a/1/1"},
				Status:              pb.Status_ENDED_MASK,
				CreatedBy:           "user:abc",
				StartTime:           &expectedStartTime,
				EndTime:             &expectedEndTime,
				IncludeExperimental: false,
				BuildHigh:           utils.ToInt64Ptr(int64(201)),
				BuildLow:            utils.ToInt64Ptr(int64(99)),
				Canary:              utils.ToBoolPtr(true),
				PageSize:            0,
				StartCursor:         "",
			}

			So(err, ShouldBeNil)
			So(query, ShouldResemble, expected)
		})
		Convey("empty predict", func() {
			req := &pb.SearchBuildsRequest{
				PageToken: "aa",
				PageSize:  2,
			}
			query, err := NewSearchQuery(req)

			So(err, ShouldBeNil)
			So(query, ShouldResemble, &SearchQuery{
				PageSize:    2,
				StartCursor: "aa",
			})
		})
		Convey("invalid create time", func() {
			req := &pb.SearchBuildsRequest{
				Predicate: &pb.BuildPredicate{
					CreateTime: &pb.TimeRange{
						StartTime: nil,
					},
				},
			}
			query, err := NewSearchQuery(req)

			So(query, ShouldBeNil)
			So(err, ShouldErrLike, "CreateTime.StartTime: timestamp: nil Timestamp")
		})
	})
}
