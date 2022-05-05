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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetTestResultHistoryRequest(t *testing.T) {
	t.Parallel()

	Convey(`ValidateGetTestResultHistoryRequest`, t, func() {
		recently := timestamppb.New(testclock.TestRecentTimeUTC)
		earlier := timestamppb.New(testclock.TestRecentTimeUTC.Add(-24 * time.Hour))

		Convey(`valid`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
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
						Contains: pbutil.Variant("os", "Mac-10.15"),
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldBeNil)
		})

		Convey(`missing realm`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "realm is required")
		})

		Convey(`invalid realm`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "invalid/realm",
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "realm: bad global realm name")
		})

		Convey(`bad pageSize`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{},
				},
				PageSize: -10,
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "page_size, if specified, must be a positive integer")
		})

		Convey(`bad variant`, func() {
			req := &pb.GetTestResultHistoryRequest{
				Realm: "testproject:testrealm",
				Range: &pb.GetTestResultHistoryRequest_TimeRange{
					TimeRange: &pb.TimeRange{},
				},
				VariantPredicate: &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: pbutil.Variant("$os", "Mac-10.15"),
					},
				},
			}
			So(validateGetTestResultHistoryRequest(req), ShouldErrLike, "variant_predicate")
		})
	})
}

func TestGetTestResultHistory(t *testing.T) {
	Convey(`TestGetTestResultHistory`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx = clock.Set(ctx, testclock.New(testclock.TestRecentTimeUTC.Add(time.Hour)))
		ctx = auth.WithState(
			ctx,
			&authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				},
			})
		srv := newTestResultDBService()

		earliest := timestamppb.New(testclock.TestRecentTimeUTC)
		afterLatest := timestamppb.New(testclock.TestRecentTimeUTC.Add(3 * time.Minute))

		req := &pb.GetTestResultHistoryRequest{
			Range: &pb.GetTestResultHistoryRequest_TimeRange{
				TimeRange: &pb.TimeRange{
					Earliest: earliest,
					Latest:   afterLatest,
				},
			},
			VariantPredicate: &pb.VariantPredicate{
				Predicate: &pb.VariantPredicate_Contains{
					Contains: pbutil.Variant("dummy", "true"),
				},
			},
		}
		Convey(`unsuccessful`, func() {
			Convey(`no realm`, func() {
				res, err := srv.GetTestResultHistory(ctx, req)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument)
				So(err, ShouldErrLike, "realm is required")
				So(res, ShouldBeNil)
			})

			Convey(`unauthorized`, func() {
				req.Realm = "testproject:secretrealm"
				res, err := srv.GetTestResultHistory(ctx, req)
				So(err, ShouldHaveAppStatus, codes.PermissionDenied)
				So(res, ShouldBeNil)
			})
		})
	})
}
