// Copyright 2019 The LUCI Authors.
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
	"sort"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateQueryTestExonerationsRequest(t *testing.T) {
	t.Parallel()
	Convey(`Valid`, t, func() {
		err := validateQueryTestExonerationsRequest(&pb.QueryTestExonerationsRequest{
			Invocations: []string{"invocations/x"},
			PageSize:    50,
		})
		So(err, ShouldBeNil)
	})

	Convey(`invalid predicate`, t, func() {
		err := validateQueryTestExonerationsRequest(&pb.QueryTestExonerationsRequest{
			Invocations: []string{"x"},
		})
		So(err, ShouldErrLike, `invocations: "x": does not match`)
	})
}

func TestQueryTestExonerations(t *testing.T) {
	Convey(`QueryTestExonerations`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		insertInv := insert.FinalizedInvocationWithInclusions
		insertEx := insert.TestExonerations
		testutil.MustApply(ctx, testutil.CombineMutations(
			insertInv("x", map[string]any{"Realm": "secretproject:testrealm"}, "a"),
			insertInv("a", map[string]any{"Realm": "testproject:testrealm"}, "b"),
			insertInv("b", map[string]any{"Realm": "otherproject:testrealm"}, "c"),
			// The invocation c doesn't have any included invocation.
			insertInv("c", map[string]any{"Realm": "testproject:testrealm"}),
			insertEx("a", "A", pbutil.Variant("v", "a"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS, pb.ExonerationReason_NOT_CRITICAL),
			insertEx("c", "C", pbutil.Variant("v", "c"), pb.ExonerationReason_OCCURS_ON_MAINLINE),
			insertEx("c", "D", pbutil.Variant("v", "d"), pb.ExonerationReason_UNEXPECTED_PASS),
		)...)

		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			_, err := srv.QueryTestExonerations(ctx, &pb.QueryTestExonerationsRequest{
				Invocations: []string{"invocations/x"},
			})
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
		})

		Convey(`Valid with included invocation`, func() {
			res, err := srv.QueryTestExonerations(ctx, &pb.QueryTestExonerationsRequest{
				Invocations: []string{"invocations/a"},
			})
			So(err, ShouldBeNil)
			actual := res.TestExonerations
			sort.Slice(actual, func(i, j int) bool {
				return actual[i].Name < actual[j].Name
			})

			expected := []*pb.TestExoneration{
				{
					Name:            "invocations/a/tests/A/exonerations/0",
					TestId:          "A",
					Variant:         pbutil.Variant("v", "a"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
					ExonerationId:   "0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_OCCURS_ON_OTHER_CLS,
				},
				{
					Name:            "invocations/a/tests/A/exonerations/1",
					TestId:          "A",
					Variant:         pbutil.Variant("v", "a"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "a")),
					ExonerationId:   "1",
					ExplanationHtml: "explanation 1",
					Reason:          pb.ExonerationReason_NOT_CRITICAL,
				},
				{
					Name:            "invocations/c/tests/C/exonerations/0",
					TestId:          "C",
					Variant:         pbutil.Variant("v", "c"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "c")),
					ExonerationId:   "0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
				{
					Name:            "invocations/c/tests/D/exonerations/0",
					TestId:          "D",
					Variant:         pbutil.Variant("v", "d"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "d")),
					ExonerationId:   "0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			}

			So(actual, ShouldResembleProto, expected)

			Convey(`And with missing included invocation`, func() {
				testutil.MustApply(
					ctx,
					// The invocation missinginv is missing in Invocations table.
					insert.Inclusion("a", "missinginv"),
				)
				res, err := srv.QueryTestExonerations(ctx, &pb.QueryTestExonerationsRequest{
					Invocations: []string{"invocations/a"},
				})
				So(err, ShouldBeNil)
				actual := res.TestExonerations
				sort.Slice(actual, func(i, j int) bool {
					return actual[i].Name < actual[j].Name
				})
				So(actual, ShouldResembleProto, expected)
			})
		})

		Convey(`Valid without included invocation`, func() {
			res, err := srv.QueryTestExonerations(ctx, &pb.QueryTestExonerationsRequest{
				Invocations: []string{"invocations/c"},
			})
			So(err, ShouldBeNil)
			actual := res.TestExonerations
			sort.Slice(actual, func(i, j int) bool {
				return actual[i].Name < actual[j].Name
			})
			So(actual, ShouldResembleProto, []*pb.TestExoneration{
				{
					Name:            "invocations/c/tests/C/exonerations/0",
					TestId:          "C",
					Variant:         pbutil.Variant("v", "c"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "c")),
					ExonerationId:   "0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_OCCURS_ON_MAINLINE,
				},
				{
					Name:            "invocations/c/tests/D/exonerations/0",
					TestId:          "D",
					Variant:         pbutil.Variant("v", "d"),
					VariantHash:     pbutil.VariantHash(pbutil.Variant("v", "d")),
					ExonerationId:   "0",
					ExplanationHtml: "explanation 0",
					Reason:          pb.ExonerationReason_UNEXPECTED_PASS,
				},
			})
		})
	})
}
