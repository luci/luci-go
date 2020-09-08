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

package resultdb

import (
	//"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock/testclock"

	//"cloud.google.com/go/spanner"
	//durpb "github.com/golang/protobuf/ptypes/duration"

	//"go.chromium.org/luci/resultdb/internal/invocations"
	//"go.chromium.org/luci/resultdb/internal/spanutil"
	//"go.chromium.org/luci/resultdb/internal/testutil"
	//"go.chromium.org/luci/resultdb/internal/testutil/insert"
	//"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// TODO-NOW: Tests for this:
//      Unit:
//	  Permissions
//	  Validation
// realm/timerange/pagesize/predicate
//	  TODO: Unit-tests for iterators.
//	End-to-End:
//	  No Permission
//	  Invalid Request
//	  SinglePage at each call
//		More than page_size
//		less than page_size
//		no results
//	  MultiplePages at each call:
//		eventually more results than page_size
//		eventually less results than page_size
//		no results
func TestValidateGetTestResultHistoryRequest(t *testing.T) {
	t.Parallel()

	Convey(`ValidateGetTestResultHistoryRequest`, t, func() {
		recently, err := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		So(err, ShouldBeNil)
		earlier, err := ptypes.TimestampProto(testclock.TestRecentTimeUTC.Add(-24 * time.Hour))
		So(err, ShouldBeNil)

		Convey(`Valid`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject/testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldBeNil)
		})

		Convey(`missing realm`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "realm is required")
		})

		Convey(`bad pageSize`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject/testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     -10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "page_size, if specified, must be a positive integer")
		})

		Convey(`bad variant`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject/testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{
						Earliest: earlier,
						Latest:   recently,
					},
				},
				PageSize:     10,
				TestIdRegexp: "ninja://:blink_web_tests/fast/.*",
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{"$os": "Mac-10.15"},
						},
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "variant_predicate is invalid")
		})
	})
}
