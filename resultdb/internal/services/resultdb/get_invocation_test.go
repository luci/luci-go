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
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetInvocationRequest(t *testing.T) {
	t.Parallel()
	Convey(`ValidateGetInvocationRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.GetInvocationRequest{Name: "invocations/valid_id_0"}
			So(validateGetInvocationRequest(req), ShouldBeNil)
		})

		Convey(`Invalid name`, func() {
			Convey(`, missing`, func() {
				req := &pb.GetInvocationRequest{}
				So(validateGetInvocationRequest(req), ShouldErrLike, "name missing")
			})

			Convey(`, invalid format`, func() {
				req := &pb.GetInvocationRequest{Name: "bad_name"}
				So(validateGetInvocationRequest(req), ShouldErrLike, "does not match")
			})
		})
	})
}

func TestGetInvocation(t *testing.T) {
	Convey(`GetInvocation`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInvocation},
			},
		})
		ct := testclock.TestRecentTimeUTC
		deadline := ct.Add(time.Hour)
		srv := newTestResultDBService()

		Convey(`Valid`, func() {
			// Insert some Invocations.
			testutil.MustApply(ctx,
				insert.Invocation("including", pb.Invocation_ACTIVE, map[string]any{
					"CreateTime":     ct,
					"Deadline":       deadline,
					"Realm":          "testproject:testrealm",
					"Properties":     spanutil.Compress(pbutil.MustMarshal(testutil.TestProperties())),
					"Sources":        spanutil.Compress(pbutil.MustMarshal(testutil.TestSources())),
					"InheritSources": spanner.NullBool{Valid: true, Bool: true},
					"BaselineId":     "testrealm:testbuilder",
				}),
				insert.Invocation("included0", pb.Invocation_FINALIZED, nil),
				insert.Invocation("included1", pb.Invocation_FINALIZED, nil),
				insert.Inclusion("including", "included0"),
				insert.Inclusion("including", "included1"),
			)

			// Fetch back the top-level Invocation.
			req := &pb.GetInvocationRequest{Name: "invocations/including"}
			inv, err := srv.GetInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(inv, ShouldResembleProto, &pb.Invocation{
				Name:                "invocations/including",
				State:               pb.Invocation_ACTIVE,
				CreateTime:          pbutil.MustTimestampProto(ct),
				Deadline:            pbutil.MustTimestampProto(deadline),
				IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
				Realm:               "testproject:testrealm",
				Properties:          testutil.TestProperties(),
				SourceSpec: &pb.SourceSpec{
					Sources: testutil.TestSources(),
					Inherit: true,
				},
				BaselineId: "testrealm:testbuilder",
			})
		})

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("secret", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "secretproject:testrealm",
				}),
			)
			req := &pb.GetInvocationRequest{Name: "invocations/secret"}
			_, err := srv.GetInvocation(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, "caller does not have permission resultdb.invocations.get")
		})
	})
}
